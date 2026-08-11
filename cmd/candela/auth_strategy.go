package main

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// authStrategy identifies which authentication strategy was selected.
type authStrategy int

const (
	strategyDirectIDToken    authStrategy = iota // Strategy 1: SA credentials → direct OIDC ID token
	strategyIAPImpersonation                     // Strategy 1.5: IAP impersonation via iap_service_account
	strategyUserADC                              // Strategy 2: User credentials → OAuth2 access token
)

func (s authStrategy) String() string {
	switch s {
	case strategyDirectIDToken:
		return "direct-id-token"
	case strategyIAPImpersonation:
		return "iap-impersonation"
	case strategyUserADC:
		return "user-adc"
	default:
		return "unknown"
	}
}

// authResult holds the resolved token sources and strategy metadata.
type authResult struct {
	// tokenSource provides tokens for Proxy-Authorization (IAP) and
	// Authorization (server) headers.
	tokenSource oauth2.TokenSource

	// userTokenSource (optional) provides a separate user identity token
	// when using dual-token mode (Strategy 1.5). When set, the Director
	// sends tokenSource → Proxy-Authorization + Authorization, and
	// userTokenSource → X-Candela-Auth.
	userTokenSource oauth2.TokenSource

	// strategy records which strategy was selected for diagnostics/testing.
	strategy authStrategy
}

// idTokenFactory abstracts idtoken.NewTokenSource for testing.
// In production, this is idtoken.NewTokenSource.
type idTokenFactory func(ctx context.Context, audience string, opts ...idtoken.ClientOption) (oauth2.TokenSource, error)

// defaultTokenFactory abstracts google.DefaultTokenSource for testing.
type defaultTokenFactory func(ctx context.Context, scope ...string) (oauth2.TokenSource, error)

// resolveAuthStrategy selects the authentication strategy based on the
// available credentials and configuration.
//
// When iap_service_account is configured, Strategy 1 (direct ID token) is
// skipped entirely. This prevents WIF external_account credentials from
// calling generateIdToken on the caller's own SA — which fails when the
// SA only has getOpenIdToken on the target IAP SA, not on itself.
func resolveAuthStrategy(
	ctx context.Context,
	cfg *Config,
	newIDToken idTokenFactory,
	newDefaultToken defaultTokenFactory,
) (*authResult, error) {

	// When iap_service_account is configured, skip the direct ID token
	// strategy. idtoken.NewTokenSource succeeds for WIF external_account
	// credentials but then calls generateIdToken on the caller's own SA,
	// which fails if that SA only has getOpenIdToken on the *target* IAP SA
	// (the typical CI setup). Jump straight to IAP impersonation instead.
	var ts oauth2.TokenSource
	var err error
	if cfg.IAPServiceAccount == "" {
		ts, err = newIDToken(ctx, cfg.Audience)
	} else {
		err = fmt.Errorf("iap_service_account is set, using IAP impersonation")
	}

	if err == nil {
		// Strategy 1: Service account credentials → audience-scoped OIDC ID token.
		result := &authResult{
			tokenSource: ts,
			strategy:    strategyDirectIDToken,
		}
		slog.Info("resolved auth strategy", "strategy", result.strategy)
		return result, nil
	}

	if cfg.IAPServiceAccount != "" {
		// Strategy 1.5: IAP impersonation → dual token.
		slog.Debug("using IAP impersonation", "reason", err)
		baseTSr, err2 := newDefaultToken(ctx, "https://www.googleapis.com/auth/cloud-platform")
		if err2 != nil {
			return nil, fmt.Errorf("failed to get credentials: %w", err2)
		}

		result := &authResult{
			tokenSource: oauth2.ReuseTokenSource(nil, &iapImpersonatingTokenSource{
				base:           baseTSr,
				serviceAccount: cfg.IAPServiceAccount,
				audience:       cfg.Audience,
			}),
			strategy: strategyIAPImpersonation,
		}

		// Keep the user's ADC token source for server-side identity validation.
		userTSr, err3 := newDefaultToken(ctx, "openid", "email")
		if err3 != nil {
			slog.Warn("failed to get user ADC token source — server auth may fail", "error", err3)
		} else {
			result.userTokenSource = userTSr
		}

		slog.Info("resolved auth strategy", "strategy", result.strategy)
		return result, nil
	}

	// Strategy 2: User credentials → OAuth2 access token.
	slog.Debug("idtoken.NewTokenSource unavailable (user credentials fallback)", "reason", err)
	ts2, err2 := newDefaultToken(ctx, "openid", "email")
	if err2 != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err2)
	}

	result := &authResult{
		tokenSource: ts2,
		strategy:    strategyUserADC,
	}
	slog.Info("resolved auth strategy", "strategy", result.strategy)
	return result, nil
}
