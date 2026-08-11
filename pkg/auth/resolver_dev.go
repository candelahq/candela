package auth

import "context"

type DevResolver struct{}

func NewDevResolver() *DevResolver {
	return &DevResolver{}
}

func (r *DevResolver) Name() string { return "dev" }

func (r *DevResolver) Resolve(ctx context.Context, token string) (*Identity, error) {
	return &Identity{
		ID:       "dev-admin",
		Email:    "admin@localhost",
		Provider: "dev",
	}, nil
}
