package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"
)

func TestBQBufferSize(t *testing.T) {
	if got := bqBufferSize(BQConfig{}); got != defaultBQBufferSize {
		t.Errorf("default buffer size = %d, want %d", got, defaultBQBufferSize)
	}
	if got := bqBufferSize(BQConfig{BufferSize: 1024}); got != 1024 {
		t.Errorf("configured buffer size = %d, want 1024", got)
	}
	if got := bqBufferSize(BQConfig{BufferSize: -1}); got != defaultBQBufferSize {
		t.Errorf("invalid buffer size = %d, want %d", got, defaultBQBufferSize)
	}
}

// newTestBQLogger creates a BQLogger with only the events channel initialized
// (no BigQuery client). This lets us test buffer behavior without cloud deps.
func newTestBQLogger(bufSize int) *BQLogger {
	return &BQLogger{
		events: make(chan bqRow, bufSize),
		done:   make(chan struct{}),
	}
}

func TestBQLogger_Log_EnqueuesEvent(t *testing.T) {
	l := newTestBQLogger(10)

	l.Log(context.Background(), Event{
		ActorEmail: "admin@test.com",
		Service:    "UserService",
		Method:     "CreateUser",
		Procedure:  "/candela.v1.UserService/CreateUser",
		StatusCode: "ok",
		Timestamp:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	if len(l.events) != 1 {
		t.Fatalf("events channel length = %d, want 1", len(l.events))
	}

	row := <-l.events
	if row.ActorEmail != "admin@test.com" {
		t.Errorf("ActorEmail = %q, want %q", row.ActorEmail, "admin@test.com")
	}
	if row.Service != "UserService" {
		t.Errorf("Service = %q, want %q", row.Service, "UserService")
	}
	if row.Method != "CreateUser" {
		t.Errorf("Method = %q, want %q", row.Method, "CreateUser")
	}
	if row.Procedure != "/candela.v1.UserService/CreateUser" {
		t.Errorf("Procedure = %q, want %q", row.Procedure, "/candela.v1.UserService/CreateUser")
	}
	if row.StatusCode != "ok" {
		t.Errorf("StatusCode = %q, want %q", row.StatusCode, "ok")
	}
	if !row.Timestamp.Valid {
		t.Error("Timestamp.Valid = false, want true")
	}
}

func TestBQLogger_Log_BufferFullDropsEvent(t *testing.T) {
	l := newTestBQLogger(1) // buffer of 1

	// Fill the buffer.
	l.Log(context.Background(), Event{Method: "first"})

	// This should be dropped (buffer full).
	l.Log(context.Background(), Event{Method: "dropped"})

	if len(l.events) != 1 {
		t.Fatalf("events channel length = %d, want 1 (second event should be dropped)", len(l.events))
	}

	row := <-l.events
	if row.Method != "first" {
		t.Errorf("Method = %q, want %q (first event should be preserved)", row.Method, "first")
	}
}

func TestBQLogger_Log_AllFieldsMapped(t *testing.T) {
	l := newTestBQLogger(10)
	ts := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	l.Log(context.Background(), Event{
		Timestamp:  ts,
		ActorEmail: "user@example.com",
		ActorID:    "uid-456",
		Service:    "ProjectService",
		Method:     "DeleteProject",
		Procedure:  "/candela.v1.ProjectService/DeleteProject",
		StatusCode: "permission_denied",
		Error:      "not allowed",
	})

	row := <-l.events
	if row.Timestamp.Timestamp != ts {
		t.Errorf("Timestamp = %v, want %v", row.Timestamp.Timestamp, ts)
	}
	if row.ActorEmail != "user@example.com" {
		t.Errorf("ActorEmail = %q, want %q", row.ActorEmail, "user@example.com")
	}
	if row.ActorID != "uid-456" {
		t.Errorf("ActorID = %q, want %q", row.ActorID, "uid-456")
	}
	if row.Error != "not allowed" {
		t.Errorf("Error = %q, want %q", row.Error, "not allowed")
	}
}

func TestBQLogger_Log_MultipleEvents(t *testing.T) {
	l := newTestBQLogger(10)

	for i := 0; i < 5; i++ {
		l.Log(context.Background(), Event{Method: "op"})
	}

	if len(l.events) != 5 {
		t.Fatalf("events channel length = %d, want 5", len(l.events))
	}
}

func TestBQLogger_Log_ZeroTimestamp(t *testing.T) {
	l := newTestBQLogger(10)

	l.Log(context.Background(), Event{Method: "no-ts"})

	row := <-l.events
	if !row.Timestamp.Valid {
		t.Error("Timestamp.Valid = false, want true (even for zero time)")
	}
}

func TestNewBQLogger_WithClient_DoesNotOwnClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "test-proj",
		option.WithoutAuthentication(),
		option.WithEndpoint(ts.URL),
	)
	if err != nil {
		t.Fatalf("failed to create dummy bigquery client: %v", err)
	}
	defer func() { _ = client.Close() }()

	cfg := BQConfig{
		ProjectID:  "test-proj",
		Dataset:    "test-dataset",
		BufferSize: 16,
		Client:     client,
	}

	logger, err := NewBQLogger(ctx, cfg)
	if err != nil {
		t.Fatalf("NewBQLogger failed: %v", err)
	}

	if logger.ownsClient {
		t.Errorf("expected logger.ownsClient to be false, got true")
	}
	if logger.client != client {
		t.Errorf("expected logger.client to match injected client")
	}

	// Close should drain and return nil without closing the external client.
	if err := logger.Close(); err != nil {
		t.Errorf("logger.Close() failed: %v", err)
	}
}

func TestEnsureTableWithClient_NilClient(t *testing.T) {
	err := EnsureTableWithClient(context.Background(), nil, "test-dataset")
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}
}

func TestEnsureTableWithClient_Success(t *testing.T) {
	var called atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tableReference": {"projectId": "test-proj", "datasetId": "test-ds", "tableId": "admin_audit_log"}}`))
	}))
	defer ts.Close()

	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "test-proj",
		option.WithoutAuthentication(),
		option.WithEndpoint(ts.URL),
	)
	if err != nil {
		t.Fatalf("failed to create dummy bigquery client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := EnsureTableWithClient(ctx, client, "test-ds"); err != nil {
		t.Fatalf("EnsureTableWithClient failed: %v", err)
	}
	if !called.Load() {
		t.Error("expected mock server to be called by EnsureTableWithClient")
	}
}

func TestEnsureTableWithClient_AlreadyExists409(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error": {"code": 409, "message": "Already Exists: Table test-proj:test-ds.admin_audit_log"}}`))
	}))
	defer ts.Close()

	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "test-proj",
		option.WithoutAuthentication(),
		option.WithEndpoint(ts.URL),
	)
	if err != nil {
		t.Fatalf("failed to create dummy bigquery client: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Idempotent 409 should return nil error.
	if err := EnsureTableWithClient(ctx, client, "test-ds"); err != nil {
		t.Fatalf("EnsureTableWithClient with 409 failed: %v", err)
	}
}

func TestEnsureTable_WithInjectedClient(t *testing.T) {
	var called atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tableReference": {"projectId": "test-proj", "datasetId": "test-ds", "tableId": "admin_audit_log"}}`))
	}))
	defer ts.Close()

	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "test-proj",
		option.WithoutAuthentication(),
		option.WithEndpoint(ts.URL),
	)
	if err != nil {
		t.Fatalf("failed to create dummy bigquery client: %v", err)
	}
	defer func() { _ = client.Close() }()

	cfg := BQConfig{
		ProjectID: "test-proj",
		Dataset:   "test-ds",
		Client:    client,
	}

	if err := EnsureTable(ctx, cfg); err != nil {
		t.Fatalf("EnsureTable with Client failed: %v", err)
	}
	if !called.Load() {
		t.Error("expected mock server to be called by EnsureTable")
	}
}
