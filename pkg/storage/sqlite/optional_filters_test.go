package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// seedMultiProjectSpans inserts spans across multiple projects, users, tenants,
// and jobs for comprehensive filter testing. Returns the time anchor used.
func seedMultiProjectSpans(t *testing.T, s *Store) time.Time {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)

	spans := []storage.Span{
		{
			SpanID: "mp-s1", TraceID: "mp-t1", Name: "openai.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(-1 * time.Hour), EndTime: now.Add(-59 * time.Minute),
			Duration: time.Minute, ProjectID: "default", UserID: "alice@example.com",
			TenantID: "acme", JobID: "job-1",
			GenAI: &storage.GenAIAttributes{
				Model: "gpt-4o", Provider: "openai",
				InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.05,
			},
		},
		{
			SpanID: "mp-s2", TraceID: "mp-t2", Name: "claude.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(-30 * time.Minute), EndTime: now.Add(-29 * time.Minute),
			Duration: time.Minute, ProjectID: "default", UserID: "bob@example.com",
			TenantID: "acme", JobID: "job-2",
			GenAI: &storage.GenAIAttributes{
				Model: "claude-sonnet-4-20250514", Provider: "anthropic",
				InputTokens: 200, OutputTokens: 100, TotalTokens: 300, CostUSD: 0.10,
			},
		},
		{
			SpanID: "mp-s3", TraceID: "mp-t3", Name: "gemini.chat",
			Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now.Add(-15 * time.Minute), EndTime: now.Add(-14 * time.Minute),
			Duration: time.Minute, ProjectID: "other-project", UserID: "alice@example.com",
			TenantID: "beta-corp", JobID: "job-3",
			GenAI: &storage.GenAIAttributes{
				Model: "gemini-2.5-pro", Provider: "google",
				InputTokens: 300, OutputTokens: 150, TotalTokens: 450, CostUSD: 0.15,
			},
		},
	}
	if err := s.IngestSpans(context.Background(), spans); err != nil {
		t.Fatalf("seeding spans: %v", err)
	}
	return now
}

func newFilterTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("creating test store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// ── Issue 1: Empty project_id must return all projects ──────────────────────

func TestGetUsageSummary_EmptyProjectID_ReturnsAll(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	summary, err := s.GetUsageSummary(context.Background(), storage.UsageQuery{
		ProjectID: "", // ← the bug: this used to match only project_id=''
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetUsageSummary: %v", err)
	}
	if summary.TotalSpans != 3 {
		t.Errorf("TotalSpans = %d, want 3 (all projects)", summary.TotalSpans)
	}
	if summary.TotalCostUSD != 0.30 {
		t.Errorf("TotalCostUSD = %f, want 0.30", summary.TotalCostUSD)
	}
}

func TestGetUsageSummary_SpecificProjectID_FiltersCorrectly(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	summary, err := s.GetUsageSummary(context.Background(), storage.UsageQuery{
		ProjectID: "default",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetUsageSummary: %v", err)
	}
	if summary.TotalSpans != 2 {
		t.Errorf("TotalSpans = %d, want 2 (default project only)", summary.TotalSpans)
	}
}

func TestGetModelBreakdown_EmptyProjectID_ReturnsAll(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	models, err := s.GetModelBreakdown(context.Background(), storage.UsageQuery{
		ProjectID: "",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetModelBreakdown: %v", err)
	}
	if len(models) != 3 {
		t.Errorf("model count = %d, want 3", len(models))
	}
}

func TestGetModelBreakdown_SpecificProjectID(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	models, err := s.GetModelBreakdown(context.Background(), storage.UsageQuery{
		ProjectID: "other-project",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetModelBreakdown: %v", err)
	}
	if len(models) != 1 {
		t.Errorf("model count = %d, want 1", len(models))
	}
	if len(models) > 0 && models[0].Model != "gemini-2.5-pro" {
		t.Errorf("model = %q, want gemini-2.5-pro", models[0].Model)
	}
}

func TestQueryTraces_EmptyProjectID_ReturnsAll(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	result, err := s.QueryTraces(context.Background(), storage.TraceQuery{
		ProjectID: "",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("QueryTraces: %v", err)
	}
	if result.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3 (all projects)", result.TotalCount)
	}
}

func TestSearchSpans_EmptyProjectID_ReturnsAll(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	result, err := s.SearchSpans(context.Background(), storage.SpanQuery{
		ProjectID: "",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("SearchSpans: %v", err)
	}
	if result.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3 (all projects)", result.TotalCount)
	}
}

func TestGetUserLeaderboard_EmptyProjectID_ReturnsAll(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	users, err := s.GetUserLeaderboard(context.Background(), storage.UsageQuery{
		ProjectID: "",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	}, 10)
	if err != nil {
		t.Fatalf("GetUserLeaderboard: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("user count = %d, want 2 (alice + bob across all projects)", len(users))
	}
}

func TestGetTenantLeaderboard_EmptyProjectID_ReturnsAll(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	tenants, err := s.GetTenantLeaderboard(context.Background(), storage.UsageQuery{
		ProjectID: "",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	}, 10)
	if err != nil {
		t.Fatalf("GetTenantLeaderboard: %v", err)
	}
	if len(tenants) != 2 {
		t.Errorf("tenant count = %d, want 2 (acme + beta-corp)", len(tenants))
	}
}

func TestGetJobLeaderboard_EmptyProjectID_ReturnsAll(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	jobs, err := s.GetJobLeaderboard(context.Background(), storage.UsageQuery{
		ProjectID: "",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	}, 10)
	if err != nil {
		t.Fatalf("GetJobLeaderboard: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("job count = %d, want 3", len(jobs))
	}
}

// ── Issue 2: UserID scoping still works with optional project_id ────────────

func TestGetUsageSummary_UserAndProjectScoping(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	t.Run("empty_project_alice_sees_both_projects", func(t *testing.T) {
		summary, err := s.GetUsageSummary(context.Background(), storage.UsageQuery{
			ProjectID: "",
			UserID:    "alice@example.com",
			StartTime: now.Add(-2 * time.Hour),
			EndTime:   now.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("GetUsageSummary: %v", err)
		}
		if summary.TotalSpans != 2 {
			t.Errorf("TotalSpans = %d, want 2 (alice across all projects)", summary.TotalSpans)
		}
	})

	t.Run("specific_project_alice_sees_one", func(t *testing.T) {
		summary, err := s.GetUsageSummary(context.Background(), storage.UsageQuery{
			ProjectID: "default",
			UserID:    "alice@example.com",
			StartTime: now.Add(-2 * time.Hour),
			EndTime:   now.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("GetUsageSummary: %v", err)
		}
		if summary.TotalSpans != 1 {
			t.Errorf("TotalSpans = %d, want 1 (alice in default only)", summary.TotalSpans)
		}
	})
}

// ── Issue 3: TenantID scoping with optional project_id ──────────────────────

func TestGetUsageSummary_TenantScoping_WithEmptyProject(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	summary, err := s.GetUsageSummary(context.Background(), storage.UsageQuery{
		ProjectID: "",
		TenantID:  "acme",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetUsageSummary: %v", err)
	}
	if summary.TotalSpans != 2 {
		t.Errorf("TotalSpans = %d, want 2 (acme tenant across all projects)", summary.TotalSpans)
	}
}

// ── Regression guard: nonexistent project returns empty ─────────────────────

func TestGetUsageSummary_NonexistentProject_ReturnsZero(t *testing.T) {
	s := newFilterTestStore(t)
	now := seedMultiProjectSpans(t, s)

	summary, err := s.GetUsageSummary(context.Background(), storage.UsageQuery{
		ProjectID: "does-not-exist",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetUsageSummary: %v", err)
	}
	if summary.TotalSpans != 0 {
		t.Errorf("TotalSpans = %d, want 0 for nonexistent project", summary.TotalSpans)
	}
}
