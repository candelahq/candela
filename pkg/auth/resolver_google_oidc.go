package auth

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/idtoken"
)

type GoogleOIDCResolver struct {
	audience    string
	saAllowlist *ServiceAccountAllowlist
}

func NewGoogleOIDCResolver(audience string, saAllowlist *ServiceAccountAllowlist) *GoogleOIDCResolver {
	return &GoogleOIDCResolver{
		audience:    audience,
		saAllowlist: saAllowlist,
	}
}

func (r *GoogleOIDCResolver) Name() string { return "google-oidc" }

func (r *GoogleOIDCResolver) Resolve(ctx context.Context, token string) (*Identity, error) {
	if r.audience == "" {
		return nil, nil
	}
	payload, err := idtoken.Validate(ctx, token, r.audience)
	if err != nil {
		return nil, nil // Not a Google ID token
	}
	email, ok := payload.Claims["email"].(string)
	if !ok || email == "" {
		return nil, fmt.Errorf("token missing email claim")
	}
	if payload.Subject == "" {
		return nil, fmt.Errorf("token missing subject claim")
	}

	emailLower := strings.ToLower(email)
	if strings.HasSuffix(emailLower, ".gserviceaccount.com") {
		if r.saAllowlist == nil || !r.saAllowlist.IsAllowed(emailLower) {
			return nil, &ForbiddenError{Message: errServiceAccountDenied}
		}
	}

	return &Identity{
		ID:       payload.Subject,
		Email:    emailLower,
		Provider: "google-oidc",
		Claims:   payload.Claims,
	}, nil
}
