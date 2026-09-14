package connecthandlers

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
)

type mockScopeUserStore struct {
	storage.UserStore
	record *storage.UserRecord
	err    error
}

func (m *mockScopeUserStore) GetUserByEmail(ctx context.Context, email string) (*storage.UserRecord, error) {
	return m.record, m.err
}

func TestScopeUserID(t *testing.T) {
	tests := []struct {
		name     string
		users    storage.UserStore
		auth     *auth.User
		devMode  bool
		want     string
		wantErr  bool
		wantCode connect.Code
	}{
		{
			name:    "nil user store (dev/unscoped mode)",
			users:   nil,
			auth:    &auth.User{Email: "dev@example.com"},
			want:    "",
			wantErr: false,
		},
		{
			name:     "no auth user in production mode fails closed",
			users:    &mockScopeUserStore{},
			auth:     nil,
			devMode:  false,
			want:     "",
			wantErr:  true,
			wantCode: connect.CodeUnauthenticated,
		},
		{
			name:    "no auth user in dev mode returns empty string",
			users:   &mockScopeUserStore{},
			auth:    nil,
			devMode: true,
			want:    "",
			wantErr: false,
		},
		{
			name: "admin user",
			users: &mockScopeUserStore{
				record: &storage.UserRecord{
					ID:   "admin@example.com",
					Role: storage.RoleAdmin,
				},
			},
			auth:    &auth.User{Email: "admin@example.com"},
			want:    "",
			wantErr: false,
		},
		{
			name: "developer user",
			users: &mockScopeUserStore{
				record: &storage.UserRecord{
					ID:   "dev@example.com",
					Role: storage.RoleDeveloper,
				},
			},
			auth:    &auth.User{Email: "dev@example.com"},
			want:    "dev@example.com",
			wantErr: false,
		},
		{
			name: "user not found - email fallback",
			users: &mockScopeUserStore{
				err: storage.ErrNotFound,
			},
			auth:    &auth.User{ID: "fallback-id", Email: "Unknown@example.com"},
			want:    "unknown@example.com",
			wantErr: false,
		},
		{
			name: "user not found - id fallback",
			users: &mockScopeUserStore{
				err: storage.ErrNotFound,
			},
			auth:    &auth.User{ID: "fallback-id"},
			want:    "fallback-id",
			wantErr: false,
		},
		{
			name: "transient lookup error - falls back to email",
			users: &mockScopeUserStore{
				err: errors.New("firestore timeout"),
			},
			auth:    &auth.User{ID: "fallback-id", Email: "user@example.com"},
			want:    "user@example.com",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.devMode {
				ctx = auth.WithDevMode(ctx, true)
			}
			if tt.auth != nil {
				ctx = auth.NewContext(ctx, tt.auth)
			}
			got, err := scopeUserID(ctx, tt.users)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("scopeUserID() expected error, got nil")
				}
				if code := connect.CodeOf(err); code != tt.wantCode {
					t.Errorf("scopeUserID() error code = %v, want %v", code, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("scopeUserID() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("scopeUserID() = %v, want %v", got, tt.want)
			}
		})
	}
}
