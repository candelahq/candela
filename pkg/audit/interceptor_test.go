package audit

import (
	"context"
	"errors"
	"sync"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/candelahq/candela/pkg/auth"
)

// captureLogger records all audit events for inspection.
type captureLogger struct {
	mu     sync.Mutex
	events []Event
}

func (c *captureLogger) Log(_ context.Context, e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *captureLogger) get() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event{}, c.events...)
}

func TestInterceptor_NilLogger_NoOp(t *testing.T) {
	interceptor := Interceptor(nil, DefaultMutationProcedures)

	called := false
	handler := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return nil, nil
	})

	_, _ = handler(context.Background(), connect.NewRequest(&struct{}{}))
	if !called {
		t.Fatal("handler was not called with nil logger")
	}
}

func TestInterceptor_UnmatchedProcedure_SkipsLogging(t *testing.T) {
	logger := &captureLogger{}
	procs := map[string]bool{"/candela.v1.UserService/CreateUser": true}
	interceptor := Interceptor(logger, procs)

	called := false
	handler := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return nil, nil
	})

	// The request goes through connect.NewRequest which doesn't set Spec.Procedure.
	// Since empty string is not in procs, it should skip logging.
	_, _ = handler(context.Background(), connect.NewRequest(&struct{}{}))

	if !called {
		t.Fatal("handler was not called")
	}
	if len(logger.get()) != 0 {
		t.Errorf("expected no audit events for unmatched procedure, got %d", len(logger.get()))
	}
}

func TestInterceptor_HandlerError_LogsErrorStatus(t *testing.T) {
	logger := &captureLogger{}
	// Use empty string as the key since connect.NewRequest yields empty procedure.
	procs := map[string]bool{"": true}
	interceptor := Interceptor(logger, procs)

	handler := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	})

	_, _ = handler(context.Background(), connect.NewRequest(&struct{}{}))

	events := logger.get()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].StatusCode != "permission_denied" {
		t.Errorf("status = %q, want permission_denied", events[0].StatusCode)
	}
	if events[0].Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestInterceptor_WithAuthContext_CapturesActor(t *testing.T) {
	logger := &captureLogger{}
	procs := map[string]bool{"": true}
	interceptor := Interceptor(logger, procs)

	handler := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, nil
	})

	ctx := auth.NewContext(context.Background(), &auth.Identity{
		Email: "admin@test.com",
		ID:    "uid-123",
	})
	_, _ = handler(ctx, connect.NewRequest(&struct{}{}))

	events := logger.get()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ActorEmail != "admin@test.com" {
		t.Errorf("actor = %q, want admin@test.com", events[0].ActorEmail)
	}
	if events[0].ActorID != "uid-123" {
		t.Errorf("actorID = %q, want uid-123", events[0].ActorID)
	}
	if events[0].StatusCode != "ok" {
		t.Errorf("status = %q, want ok", events[0].StatusCode)
	}
}

func TestMulti_FansOutToAll(t *testing.T) {
	l1 := &captureLogger{}
	l2 := &captureLogger{}
	multi := Multi{l1, nil, l2} // nil should be skipped

	multi.Log(context.Background(), Event{Method: "test"})

	if len(l1.get()) != 1 {
		t.Errorf("l1 got %d events, want 1", len(l1.get()))
	}
	if len(l2.get()) != 1 {
		t.Errorf("l2 got %d events, want 1", len(l2.get()))
	}
}

func TestParseProcedure_EdgeCases(t *testing.T) {
	tests := []struct {
		input   string
		service string
		method  string
	}{
		{"/candela.v1.UserService/CreateUser", "UserService", "CreateUser"},
		{"/a.b.c.Svc/Method", "Svc", "Method"},
		{"/Svc/M", "Svc", "M"},
		{"noslash", "noslash", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		s, m := ParseProcedure(tt.input)
		if s != tt.service || m != tt.method {
			t.Errorf("ParseProcedure(%q) = (%q, %q), want (%q, %q)", tt.input, s, m, tt.service, tt.method)
		}
	}
}
