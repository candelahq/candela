package connecthandlers

import (
	"context"
	"testing"

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
		name  string
		users storage.UserStore
		auth  *auth.User
		want  string
	}{
		{
			name:  "nil user store",
			users: nil,
			auth:  &auth.User{Email: "dev@example.com"},
			want:  "",
		},
		{
			name:  "no auth user",
			users: &mockScopeUserStore{},
			auth:  nil,
			want:  "",
		},
		{
			name: "admin user",
			users: &mockScopeUserStore{
				record: &storage.UserRecord{
					ID:   "admin@example.com",
					Role: storage.RoleAdmin,
				},
			},
			auth: &auth.User{Email: "admin@example.com"},
			want: "",
		},
		{
			name: "developer user",
			users: &mockScopeUserStore{
				record: &storage.UserRecord{
					ID:   "dev@example.com",
					Role: storage.RoleDeveloper,
				},
			},
			auth: &auth.User{Email: "dev@example.com"},
			want: "dev@example.com",
		},
		{
			name: "user not found - email fallback",
			users: &mockScopeUserStore{
				err: storage.ErrNotFound,
			},
			auth: &auth.User{ID: "fallback-id", Email: "Unknown@example.com"},
			want: "unknown@example.com",
		},
		{
			name: "user not found - id fallback",
			users: &mockScopeUserStore{
				err: storage.ErrNotFound,
			},
			auth: &auth.User{ID: "fallback-id"},
			want: "fallback-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.auth != nil {
				ctx = auth.NewContext(ctx, tt.auth)
			}
			got := scopeUserID(ctx, tt.users)
			if got != tt.want {
				t.Errorf("scopeUserID() = %v, want %v", got, tt.want)
			}
		})
	}
}
