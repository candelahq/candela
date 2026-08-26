package bigquery

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	bq "cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"github.com/candelahq/candela/pkg/storage"
)

// testStore creates a Store backed by the given mock client.
func testStore(client *mockBQClient) *Store {
	return NewWithClient(client, Config{
		ProjectID: "test-project",
		Dataset:   "test_dataset",
		Table:     "spans",
	})
}

// assertContains checks that the SQL string contains the expected substring.
func assertContains(t *testing.T, sql, substr string) {
	t.Helper()
	if !strings.Contains(sql, substr) {
		t.Errorf("SQL does not contain %q:\n%s", substr, sql)
	}
}

// assertParam checks that the given parameter name exists in the list.
func assertParam(t *testing.T, params []bq.QueryParameter, name string) {
	t.Helper()
	for _, p := range params {
		if p.Name == name {
			return
		}
	}
	t.Errorf("missing parameter %q in %v", name, paramNames(params))
}

func paramNames(params []bq.QueryParameter) []string {
	var names []string
	for _, p := range params {
		names = append(names, p.Name)
	}
	return names
}

// ── QueryTraces ──────────────────────────────────────────────────────

func TestQueryTraces_BuildsCorrectSQL(t *testing.T) {
	client := newMockBQClient()

	// QueryTraces makes two queries:
	// 1. Get trace IDs (GROUP BY trace_id)
	// 2. Batch-fetch spans for those IDs
	traceIDQuery := &mockBQQuery{
		iter: &mockBQRowIterator{
			nextFunc: func() func(dst interface{}) error {
				called := false
				return func(dst interface{}) error {
					if called {
						return iterator.Done
					}
					called = true
					// Production scans into an anonymous struct; use reflect
					// to set fields without depending on the exact type.
					v := reflect.ValueOf(dst).Elem()
					if f := v.FieldByName("TraceID"); f.IsValid() {
						f.SetString("trace-001")
					}
					if f := v.FieldByName("Earliest"); f.IsValid() {
						f.Set(reflect.ValueOf(time.Now()))
					}
					return nil
				}
			}(),
		},
	}

	batchSpanQuery := &mockBQQuery{
		iter: &mockBQRowIterator{
			nextFunc: func(dst interface{}) error {
				// Return Done immediately — we're testing SQL, not row scanning.
				return iterator.Done
			},
		},
	}

	client.enqueueQuery(traceIDQuery)
	client.enqueueQuery(batchSpanQuery)

	s := testStore(client)
	tq := storage.TraceQuery{
		ProjectID: "proj-1",
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
		UserID:    "user-1",
		PageSize:  10,
	}

	_, _ = s.QueryTraces(context.Background(), tq)

	// Verify first query (trace ID lookup).
	assertContains(t, traceIDQuery.sql, "GROUP BY trace_id")
	assertContains(t, traceIDQuery.sql, "LIMIT @pageSize")
	assertParam(t, traceIDQuery.params, "projectID")
	assertParam(t, traceIDQuery.params, "startTime")
	assertParam(t, traceIDQuery.params, "endTime")
	assertParam(t, traceIDQuery.params, "userID")
	assertParam(t, traceIDQuery.params, "pageSize")

	// Verify second query (batch span fetch).
	assertContains(t, batchSpanQuery.sql, "trace_id IN UNNEST(@traceIDs)")
	assertParam(t, batchSpanQuery.params, "traceIDs")

	// Verify that trace-001 from the first query is passed to the second.
	for _, p := range batchSpanQuery.params {
		if p.Name == "traceIDs" {
			if ids, ok := p.Value.([]string); !ok || len(ids) != 1 || ids[0] != "trace-001" {
				t.Errorf("traceIDs parameter = %v, want [trace-001]", p.Value)
			}
			break
		}
	}
}

func TestQueryTraces_EmptyResult(t *testing.T) {
	client := newMockBQClient()

	// Return no trace IDs.
	emptyQuery := &mockBQQuery{
		iter: &mockBQRowIterator{
			nextFunc: func(dst interface{}) error {
				return iterator.Done
			},
		},
	}
	client.enqueueQuery(emptyQuery)

	s := testStore(client)
	result, err := s.QueryTraces(context.Background(), storage.TraceQuery{
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Traces) != 0 {
		t.Errorf("expected 0 traces, got %d", len(result.Traces))
	}
	if result.TotalCount != 0 {
		t.Errorf("expected TotalCount=0, got %d", result.TotalCount)
	}
}

func TestQueryTraces_DefaultPageSize(t *testing.T) {
	client := newMockBQClient()

	capturedQuery := &mockBQQuery{
		iter: &mockBQRowIterator{
			nextFunc: func(dst interface{}) error {
				return iterator.Done
			},
		},
	}
	client.enqueueQuery(capturedQuery)

	s := testStore(client)
	_, _ = s.QueryTraces(context.Background(), storage.TraceQuery{
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
		PageSize:  0, // should default to 20
	})

	// Check that pageSize parameter was set to 21 (default 20 + 1 for has-more-pages detection).
	for _, p := range capturedQuery.params {
		if p.Name == "pageSize" {
			if v, ok := p.Value.(int); ok && v == 21 {
				return // correct: default 20 + 1
			}
			t.Errorf("pageSize = %v, want 21", p.Value)
			return
		}
	}
	t.Error("pageSize parameter not found")
}

// ── SearchSpans ──────────────────────────────────────────────────────

func TestSearchSpans_BuildsCorrectSQL(t *testing.T) {
	client := newMockBQClient()

	capturedQuery := &mockBQQuery{
		iter: &mockBQRowIterator{
			nextFunc: func(dst interface{}) error {
				return iterator.Done
			},
		},
	}
	client.enqueueQuery(capturedQuery)

	s := testStore(client)
	_, _ = s.SearchSpans(context.Background(), storage.SpanQuery{
		ProjectID:    "proj-1",
		StartTime:    time.Now().Add(-24 * time.Hour),
		EndTime:      time.Now(),
		Kind:         storage.SpanKindLLM,
		Model:        "gpt-4",
		NameContains: "chat",
		UserID:       "user-1",
		TenantID:     "tenant-1",
		PageSize:     25,
	})

	assertContains(t, capturedQuery.sql, "SELECT * FROM")
	assertContains(t, capturedQuery.sql, "LIMIT @pageSize")
	assertContains(t, capturedQuery.sql, "@projectID")
	assertContains(t, capturedQuery.sql, "@kind")
	assertContains(t, capturedQuery.sql, "@model")
	assertParam(t, capturedQuery.params, "projectID")
	assertParam(t, capturedQuery.params, "kind")
	assertParam(t, capturedQuery.params, "model")
	assertParam(t, capturedQuery.params, "nameContains")
	assertParam(t, capturedQuery.params, "userID")
	assertParam(t, capturedQuery.params, "tenantID")
	assertParam(t, capturedQuery.params, "pageSize")
}

// ── GetUsageSummary ──────────────────────────────────────────────────

func TestGetUsageSummary_BuildsCorrectSQL(t *testing.T) {
	client := newMockBQClient()

	capturedQuery := &mockBQQuery{
		iter: &mockBQRowIterator{
			nextFunc: func(dst interface{}) error {
				// Return one empty row then done.
				return iterator.Done
			},
		},
	}
	client.enqueueQuery(capturedQuery)

	s := testStore(client)
	_, _ = s.GetUsageSummary(context.Background(), storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
		UserID:    "user-1",
		TenantID:  "tenant-1",
	})

	assertContains(t, capturedQuery.sql, "COUNT(DISTINCT trace_id)")
	assertContains(t, capturedQuery.sql, "SUM(gen_ai_input_tokens)")
	assertContains(t, capturedQuery.sql, "SUM(gen_ai_cost_usd)")
	assertParam(t, capturedQuery.params, "projectID")
	assertParam(t, capturedQuery.params, "llmKind")
	assertParam(t, capturedQuery.params, "userID")
	assertParam(t, capturedQuery.params, "tenantID")
}

// ── GetModelBreakdown ────────────────────────────────────────────────

func TestGetModelBreakdown_BuildsCorrectSQL(t *testing.T) {
	client := newMockBQClient()

	capturedQuery := &mockBQQuery{
		iter: &mockBQRowIterator{
			nextFunc: func(dst interface{}) error {
				return iterator.Done
			},
		},
	}
	client.enqueueQuery(capturedQuery)

	s := testStore(client)
	_, _ = s.GetModelBreakdown(context.Background(), storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
	})

	assertContains(t, capturedQuery.sql, "GROUP BY gen_ai_model, gen_ai_provider")
	assertContains(t, capturedQuery.sql, "gen_ai_model AS model")
	assertContains(t, capturedQuery.sql, "ORDER BY total_cost_usd DESC")
	assertParam(t, capturedQuery.params, "projectID")
	assertParam(t, capturedQuery.params, "userID")
	assertParam(t, capturedQuery.params, "tenantID")
}

// ── GetUserLeaderboard ───────────────────────────────────────────────

func TestGetUserLeaderboard_BuildsCorrectSQL(t *testing.T) {
	client := newMockBQClient()

	capturedQuery := &mockBQQuery{
		iter: &mockBQRowIterator{
			nextFunc: func(dst interface{}) error {
				return iterator.Done
			},
		},
	}
	client.enqueueQuery(capturedQuery)

	s := testStore(client)
	_, _ = s.GetUserLeaderboard(context.Background(), storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
	}, 10)

	assertContains(t, capturedQuery.sql, "GROUP BY user_id")
	assertContains(t, capturedQuery.sql, "ORDER BY total_cost_usd DESC")
	assertContains(t, capturedQuery.sql, "LIMIT @limit")
	assertParam(t, capturedQuery.params, "projectID")
	assertParam(t, capturedQuery.params, "llmKind")
	assertParam(t, capturedQuery.params, "limit")
}

// ── GetTenantLeaderboard ─────────────────────────────────────────────

func TestGetTenantLeaderboard_BuildsCorrectSQL(t *testing.T) {
	client := newMockBQClient()

	capturedQuery := &mockBQQuery{
		iter: &mockBQRowIterator{
			nextFunc: func(dst interface{}) error {
				return iterator.Done
			},
		},
	}
	client.enqueueQuery(capturedQuery)

	s := testStore(client)
	_, _ = s.GetTenantLeaderboard(context.Background(), storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
	}, 10)

	assertContains(t, capturedQuery.sql, "GROUP BY tenant_id")
	assertContains(t, capturedQuery.sql, "ORDER BY total_cost_usd DESC")
	assertContains(t, capturedQuery.sql, "LIMIT @limit")
	assertParam(t, capturedQuery.params, "projectID")
	assertParam(t, capturedQuery.params, "llmKind")
	assertParam(t, capturedQuery.params, "limit")
}

// ── GetJobLeaderboard ────────────────────────────────────────────────

func TestGetJobLeaderboard_BuildsCorrectSQL(t *testing.T) {
	client := newMockBQClient()

	capturedQuery := &mockBQQuery{
		iter: &mockBQRowIterator{
			nextFunc: func(dst interface{}) error {
				return iterator.Done
			},
		},
	}
	client.enqueueQuery(capturedQuery)

	s := testStore(client)
	_, _ = s.GetJobLeaderboard(context.Background(), storage.UsageQuery{
		ProjectID: "proj-1",
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
	}, 10)

	assertContains(t, capturedQuery.sql, "GROUP BY job_id")
	assertContains(t, capturedQuery.sql, "LIMIT @limit")
	assertParam(t, capturedQuery.params, "projectID")
	assertParam(t, capturedQuery.params, "llmKind")
	assertParam(t, capturedQuery.params, "limit")
}

// ── Ping ─────────────────────────────────────────────────────────────

func TestPing_CallsTableMetadata(t *testing.T) {
	client := newMockBQClient()
	s := testStore(client)

	err := s.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	table, ok := client.dataset.tables["spans"]
	if !ok {
		t.Fatal("Ping did not access the expected table")
	}
	if !table.metadataCalled {
		t.Error("Ping did not call Table.Metadata()")
	}
}

func TestPing_ReturnsError(t *testing.T) {
	client := newMockBQClient()
	client.dataset.tables["spans"] = &mockBQTable{
		id:          "spans",
		metadataErr: fmt.Errorf("connection refused"),
	}

	s := testStore(client)
	err := s.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error from Ping")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %v, want to contain 'connection refused'", err)
	}
}
