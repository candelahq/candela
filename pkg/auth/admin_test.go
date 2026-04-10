package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/candelahq/candela/pkg/storage"
)

// mockUserStoreLookup implements only the GetUserByEmail method needed by AdminInterceptor.
type mockUserStoreLookup struct {
	storage.UserStore // embed to satisfy interface (panics on unimplemented calls)
	users             map[string]*storage.UserRecord
}

func (m *mockUserStoreLookup) GetUserByEmail(_ context.Context, email string) (*storage.UserRecord, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, fmt.Errorf("user %s: %w", email, storage.ErrNotFound)
	}
	return u, nil
}

func TestAdminInterceptor_AllowsAdmin(t *testing.T) {
	store := &mockUserStoreLookup{
		users: map[string]*storage.UserRecord{
			"admin@test.com": {ID: "u1", Email: "admin@test.com", Role: "admin"},
		},
	}
	interceptor := AdminInterceptor(store)

	ctx := NewContext(context.Background(), &User{ID: "u1", Email: "admin@test.com"})

	called := false
	wrappedFn := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return nil, nil
	})

	req := &fakeRequest{procedure: "/candela.v1.UserService/CreateUser"}
	_, err := wrappedFn(ctx, req)
	if err != nil {
		t.Fatalf("expected admin to be allowed, got: %v", err)
	}
	if !called {
		t.Error("expected next handler to be called")
	}
}

func TestAdminInterceptor_BlocksDeveloper(t *testing.T) {
	store := &mockUserStoreLookup{
		users: map[string]*storage.UserRecord{
			"dev@test.com": {ID: "u2", Email: "dev@test.com", Role: "developer"},
		},
	}
	interceptor := AdminInterceptor(store)

	ctx := NewContext(context.Background(), &User{ID: "u2", Email: "dev@test.com"})

	wrappedFn := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t.Error("next handler should not be called for non-admin")
		return nil, nil
	})

	req := &fakeRequest{procedure: "/candela.v1.UserService/CreateUser"}
	_, err := wrappedFn(ctx, req)
	if err == nil {
		t.Fatal("expected error for non-admin user")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("code = %v, want PermissionDenied", connectErr.Code())
	}
}

func TestAdminInterceptor_BlocksUnauthenticated(t *testing.T) {
	store := &mockUserStoreLookup{
		users: map[string]*storage.UserRecord{},
	}
	interceptor := AdminInterceptor(store)

	wrappedFn := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t.Error("next handler should not be called for unauthenticated")
		return nil, nil
	})

	req := &fakeRequest{procedure: "/candela.v1.UserService/CreateUser"}
	_, err := wrappedFn(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for unauthenticated request")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want Unauthenticated", connectErr.Code())
	}
}

func TestAdminInterceptor_AllowsSelfServiceForAnyone(t *testing.T) {
	store := &mockUserStoreLookup{
		users: map[string]*storage.UserRecord{
			"dev@test.com": {ID: "u2", Email: "dev@test.com", Role: "developer"},
		},
	}
	interceptor := AdminInterceptor(store)

	ctx := NewContext(context.Background(), &User{ID: "u2", Email: "dev@test.com"})

	for _, procedure := range []string{
		"/candela.v1.UserService/GetCurrentUser",
		"/candela.v1.UserService/GetMyBudget",
		"/candela.v1.TraceService/ListTraces",
	} {
		called := false
		wrappedFn := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			called = true
			return nil, nil
		})

		req := &fakeRequest{procedure: procedure}
		_, err := wrappedFn(ctx, req)
		if err != nil {
			t.Errorf("%s: expected to be allowed for developer, got: %v", procedure, err)
		}
		if !called {
			t.Errorf("%s: expected next handler to be called", procedure)
		}
	}
}

func TestAdminInterceptor_UnknownUserDenied(t *testing.T) {
	store := &mockUserStoreLookup{
		users: map[string]*storage.UserRecord{},
	}
	interceptor := AdminInterceptor(store)

	ctx := NewContext(context.Background(), &User{ID: "u3", Email: "unknown@test.com"})

	wrappedFn := interceptor(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t.Error("next handler should not be called for unknown user")
		return nil, nil
	})

	req := &fakeRequest{procedure: "/candela.v1.UserService/SetBudget"}
	_, err := wrappedFn(ctx, req)
	if err == nil {
		t.Fatal("expected error for unknown user on admin endpoint")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("code = %v, want PermissionDenied", connectErr.Code())
	}
}

// fakeRequest implements connect.AnyRequest for testing interceptors.
type fakeRequest struct {
	connect.AnyRequest
	procedure string
}

func (r *fakeRequest) Spec() connect.Spec {
	return connect.Spec{
		Procedure: r.procedure,
	}
}

func (r *fakeRequest) Peer() connect.Peer {
	return connect.Peer{}
}

func (r *fakeRequest) Header() http.Header {
	return http.Header{}
}
