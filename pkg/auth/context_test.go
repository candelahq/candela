package auth

import (
	"context"
	"testing"
)

func TestNewContext_RoundTrip(t *testing.T) {
	user := &User{ID: "user-123", Email: "alice@example.com"}
	ctx := NewContext(context.Background(), user)

	got := FromContext(ctx)
	if got == nil {
		t.Fatal("expected user in context, got nil")
	}
	if got.ID != user.ID {
		t.Errorf("ID = %q, want %q", got.ID, user.ID)
	}
	if got.Email != user.Email {
		t.Errorf("Email = %q, want %q", got.Email, user.Email)
	}
}

func TestFromContext_Empty(t *testing.T) {
	got := FromContext(context.Background())
	if got != nil {
		t.Errorf("expected nil user from empty context, got %+v", got)
	}
}

func TestIDFromContext(t *testing.T) {
	tests := []struct {
		name string
		user *User
		want string
	}{
		{"with user", &User{ID: "user-456", Email: "bob@example.com"}, "user-456"},
		{"nil user", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.user != nil {
				ctx = NewContext(ctx, tt.user)
			}
			if got := IDFromContext(ctx); got != tt.want {
				t.Errorf("IDFromContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmailFromContext(t *testing.T) {
	tests := []struct {
		name string
		user *User
		want string
	}{
		{"with user", &User{ID: "user-789", Email: "carol@example.com"}, "carol@example.com"},
		{"nil user", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.user != nil {
				ctx = NewContext(ctx, tt.user)
			}
			if got := EmailFromContext(ctx); got != tt.want {
				t.Errorf("EmailFromContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewContext_Overwrite(t *testing.T) {
	user1 := &User{ID: "first", Email: "first@example.com"}
	user2 := &User{ID: "second", Email: "second@example.com"}

	ctx := NewContext(context.Background(), user1)
	ctx = NewContext(ctx, user2)

	got := FromContext(ctx)
	if got == nil {
		t.Fatal("expected user in context, got nil")
	}
	if got.ID != "second" {
		t.Errorf("expected overwritten user, got ID = %q", got.ID)
	}
}
