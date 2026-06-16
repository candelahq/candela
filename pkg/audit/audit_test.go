package audit

import (
	"context"
	"sync"
	"testing"

	connect "connectrpc.com/connect"
)

// memLogger captures audit events in memory for testing.
type memLogger struct {
	mu     sync.Mutex
	events []Event
}

func (m *memLogger) Log(_ context.Context, e Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
}

func (m *memLogger) Events() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Event(nil), m.events...)
}

func TestParseProcedure(t *testing.T) {
	tests := []struct {
		in      string
		service string
		method  string
	}{
		{"/candela.v1.UserService/CreateUser", "UserService", "CreateUser"},
		{"/candela.v1.ModelCatalogService/DeleteModelCatalogEntry", "ModelCatalogService", "DeleteModelCatalogEntry"},
		{"/candela.v1.ProjectService/CreateAPIKey", "ProjectService", "CreateAPIKey"},
		{"malformed", "malformed", ""},
	}
	for _, tt := range tests {
		svc, method := ParseProcedure(tt.in)
		if svc != tt.service || method != tt.method {
			t.Errorf("ParseProcedure(%q) = (%q, %q), want (%q, %q)",
				tt.in, svc, method, tt.service, tt.method)
		}
	}
}

func TestMulti(t *testing.T) {
	a := &memLogger{}
	b := &memLogger{}
	multi := Multi{a, b}

	multi.Log(context.Background(), Event{Method: "test"})

	if len(a.Events()) != 1 {
		t.Fatalf("logger a got %d events, want 1", len(a.Events()))
	}
	if len(b.Events()) != 1 {
		t.Fatalf("logger b got %d events, want 1", len(b.Events()))
	}
}

func TestSlogLogger_NoError(t *testing.T) {
	// SlogLogger should not panic.
	l := NewSlogLogger()
	l.Log(context.Background(), Event{
		ActorEmail: "admin@test.com",
		Service:    "UserService",
		Method:     "CreateUser",
		Procedure:  "/candela.v1.UserService/CreateUser",
		StatusCode: "ok",
	})
}

func TestDefaultMutationProcedures(t *testing.T) {
	// Verify key procedures are present.
	expected := []string{
		"/candela.v1.UserService/CreateUser",
		"/candela.v1.UserService/UpdateUser",
		"/candela.v1.UserService/DeleteUser",
		"/candela.v1.ModelCatalogService/UpdateModelCatalogEntry",
		"/candela.v1.ModelCatalogService/DeleteModelCatalogEntry",
		"/candela.v1.ProjectService/CreateProject",
		"/candela.v1.ProjectService/DeleteProject",
		"/candela.v1.ProjectService/CreateAPIKey",
		"/candela.v1.ProjectService/RevokeAPIKey",
	}
	for _, p := range expected {
		if !DefaultMutationProcedures[p] {
			t.Errorf("expected %q in DefaultMutationProcedures", p)
		}
	}

	// Verify read-only procedures are NOT present.
	readOnly := []string{
		"/candela.v1.UserService/ListUsers",
		"/candela.v1.UserService/GetUser",
		"/candela.v1.ModelCatalogService/ListModelCatalog",
		"/candela.v1.ProjectService/ListProjects",
	}
	for _, p := range readOnly {
		if DefaultMutationProcedures[p] {
			t.Errorf("read-only procedure %q should NOT be in DefaultMutationProcedures", p)
		}
	}
}

// Ensure connect import is used (interceptor tests reference connect.CodeOf).
var _ = connect.CodeOf
