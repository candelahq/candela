package auth

import (
	"context"
	"strings"
)

type GoogleOAuthResolver struct {
	validator   AccessTokenValidator
	saAllowlist *ServiceAccountAllowlist
}

func NewGoogleOAuthResolver(validator AccessTokenValidator, saAllowlist *ServiceAccountAllowlist) *GoogleOAuthResolver {
	return &GoogleOAuthResolver{
		validator:   validator,
		saAllowlist: saAllowlist,
	}
}

func (r *GoogleOAuthResolver) Name() string { return "google-oauth" }

func (r *GoogleOAuthResolver) Resolve(ctx context.Context, token string) (*Identity, error) {
	if r.validator == nil {
		return nil, nil
	}
	user, err := r.validator.ValidateAccessToken(ctx, token)
	if err != nil {
		return nil, nil // Not an OAuth token
	}

	emailLower := strings.ToLower(user.Email)
	if strings.HasSuffix(emailLower, ".gserviceaccount.com") {
		if r.saAllowlist == nil || !r.saAllowlist.IsAllowed(emailLower) {
			return nil, &ForbiddenError{Message: errServiceAccountDenied}
		}
	}

	user.Email = emailLower
	user.Provider = "google-oauth"
	return user, nil
}
