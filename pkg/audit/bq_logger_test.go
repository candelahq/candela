package audit

import (
	"context"
	"testing"
	"time"
)

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
