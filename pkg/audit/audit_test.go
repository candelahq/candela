package audit

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/candelahq/candela/pkg/auth"
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

func TestMulti_NilLogger(t *testing.T) {
	a := &memLogger{}
	multi := Multi{a, nil, a} // nil logger in the middle

	multi.Log(context.Background(), Event{Method: "test"})

	if len(a.Events()) != 2 {
		t.Fatalf("logger a got %d events, want 2 (nil logger should be skipped)", len(a.Events()))
	}
}

// fakeRequest implements connect.AnyRequest for testing interceptors.
type fakeRequest struct {
	connect.AnyRequest
	procedure string
}

func (r *fakeRequest) Spec() connect.Spec {
	return connect.Spec{Procedure: r.procedure}
}

func (r *fakeRequest) Peer() connect.Peer {
	return connect.Peer{}
}

func (r *fakeRequest) Header() http.Header {
	return http.Header{}
}

func TestInterceptor_MutationLogged(t *testing.T) {
	logger := &memLogger{}
	procedures := map[string]bool{
		"/candela.v1.UserService/CreateUser": true,
	}
	interceptor := Interceptor(logger, procedures)

	// Wrap a handler that succeeds.
	handler := interceptor(func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, nil
	})

	// Inject auth context.
	ctx := auth.NewContext(context.Background(), &auth.User{
		Email: "admin@test.com",
		ID:    "uid-123",
	})

	_, _ = handler(ctx, &fakeRequest{procedure: "/candela.v1.UserService/CreateUser"})

	events := logger.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	e := events[0]
	if e.ActorEmail != "admin@test.com" {
		t.Errorf("ActorEmail = %q, want %q", e.ActorEmail, "admin@test.com")
	}
	if e.ActorID != "uid-123" {
		t.Errorf("ActorID = %q, want %q", e.ActorID, "uid-123")
	}
	if e.Service != "UserService" {
		t.Errorf("Service = %q, want %q", e.Service, "UserService")
	}
	if e.Method != "CreateUser" {
		t.Errorf("Method = %q, want %q", e.Method, "CreateUser")
	}
	if e.StatusCode != "ok" {
		t.Errorf("StatusCode = %q, want %q", e.StatusCode, "ok")
	}
	if e.Timestamp.Location() != time.UTC {
		t.Errorf("Timestamp location = %v, want UTC", e.Timestamp.Location())
	}
}

func TestInterceptor_ReadOnlyNotLogged(t *testing.T) {
	logger := &memLogger{}
	procedures := map[string]bool{
		"/candela.v1.UserService/CreateUser": true,
	}
	interceptor := Interceptor(logger, procedures)

	handler := interceptor(func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, nil
	})

	// Call a read-only procedure not in the map.
	_, _ = handler(context.Background(), &fakeRequest{procedure: "/candela.v1.UserService/ListUsers"})

	if len(logger.Events()) != 0 {
		t.Fatalf("got %d events for read-only RPC, want 0", len(logger.Events()))
	}
}

func TestInterceptor_ErrorCaptured(t *testing.T) {
	logger := &memLogger{}
	procedures := map[string]bool{
		"/candela.v1.UserService/DeleteUser": true,
	}
	interceptor := Interceptor(logger, procedures)

	handler := interceptor(func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not allowed"))
	})

	_, _ = handler(context.Background(), &fakeRequest{procedure: "/candela.v1.UserService/DeleteUser"})

	events := logger.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].StatusCode != "permission_denied" {
		t.Errorf("StatusCode = %q, want %q", events[0].StatusCode, "permission_denied")
	}
	if events[0].Error == "" {
		t.Error("Error should be non-empty for failed RPC")
	}
}

func TestInterceptor_NilAuth(t *testing.T) {
	logger := &memLogger{}
	procedures := map[string]bool{
		"/candela.v1.UserService/CreateUser": true,
	}
	interceptor := Interceptor(logger, procedures)

	handler := interceptor(func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, nil
	})

	// No auth context — should not panic.
	_, _ = handler(context.Background(), &fakeRequest{procedure: "/candela.v1.UserService/CreateUser"})

	events := logger.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].ActorEmail != "" {
		t.Errorf("ActorEmail = %q, want empty for unauthenticated", events[0].ActorEmail)
	}
}

func TestInterceptor_NilLogger(t *testing.T) {
	// Nil logger should return a no-op interceptor that doesn't panic.
	interceptor := Interceptor(nil, DefaultMutationProcedures)

	called := false
	handler := interceptor(func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return nil, nil
	})

	_, _ = handler(context.Background(), &fakeRequest{procedure: "/candela.v1.UserService/CreateUser"})
	if !called {
		t.Error("handler was not called through nil-logger interceptor")
	}
}
