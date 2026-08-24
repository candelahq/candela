package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ResolverEntry describes a single resolver in the config-driven chain.
// The Type field selects which resolver to instantiate; remaining fields
// are type-specific.
type ResolverEntry struct {
	Type         string         `yaml:"type"`          // "oidc", "firebase", "google_oauth", "google_oidc"
	Issuer       string         `yaml:"issuer"`        // OIDC: issuer URL (required)
	Audience     string         `yaml:"audience"`      // OIDC: expected "aud" claim (required)
	ClaimMapping *ClaimMapEntry `yaml:"claim_mapping"` // OIDC: optional claim overrides
}

// ClaimMapEntry configures how OIDC token claims map to Identity fields.
// All fields are optional — defaults are used when empty.
type ClaimMapEntry struct {
	Subject string `yaml:"subject"` // default: "sub"
	Email   string `yaml:"email"`   // default: "email"
	Roles   string `yaml:"roles"`   // default: "roles"
	Tenants string `yaml:"tenants"` // default: "candela.tenants"
}

// BuildOptions provides dependencies needed by individual resolvers.
type BuildOptions struct {
	// Firebase
	FirebaseVerifier TokenVerifier

	// Google OIDC (Cloud Run audience for candela-local tokens)
	CloudRunAudience string

	// Service account allowlist (shared across Google resolvers)
	SAAllowlist *ServiceAccountAllowlist

	// Dev mode: prepends a DevResolver to the chain
	DevMode bool

	// Cache settings
	CacheMaxSize int
	CacheTTL     time.Duration
}

// BuildResolverChain constructs a ResolverChain from config entries.
// Each entry's Type determines which resolver is instantiated.
// Returns an error if a required field is missing or OIDC discovery fails.
func BuildResolverChain(ctx context.Context, configs []ResolverEntry, opts BuildOptions) (*ResolverChain, error) {
	var resolvers []IdentityResolver

	if opts.DevMode {
		resolvers = append(resolvers, NewDevResolver())
		slog.Info("🔐 resolver chain: dev resolver added")
	}

	for i, cfg := range configs {
		r, err := buildResolver(ctx, cfg, opts)
		if err != nil {
			return nil, fmt.Errorf("resolver[%d] (%s): %w", i, cfg.Type, err)
		}
		resolvers = append(resolvers, r)
		slog.Info("🔐 resolver chain: added resolver", "type", cfg.Type, "name", r.Name())
	}

	if len(resolvers) == 0 {
		return nil, fmt.Errorf("no resolvers configured (and dev_mode is off)")
	}

	maxSize := opts.CacheMaxSize
	if maxSize <= 0 {
		maxSize = 1000
	}
	ttl := opts.CacheTTL
	if ttl <= 0 {
		ttl = 120 * time.Second
	}
	cache := NewIdentityCache(maxSize, ttl)

	return NewResolverChain(cache, resolvers...), nil
}

func buildResolver(ctx context.Context, cfg ResolverEntry, opts BuildOptions) (IdentityResolver, error) {
	switch cfg.Type {
	case "oidc":
		if cfg.Issuer == "" {
			return nil, fmt.Errorf("issuer is required for OIDC resolver")
		}
		if cfg.Audience == "" {
			return nil, fmt.Errorf("audience is required for OIDC resolver")
		}
		claimMap := DefaultOIDCClaimMapping()
		if cfg.ClaimMapping != nil {
			if cfg.ClaimMapping.Subject != "" {
				claimMap.Subject = cfg.ClaimMapping.Subject
			}
			if cfg.ClaimMapping.Email != "" {
				claimMap.Email = cfg.ClaimMapping.Email
			}
			if cfg.ClaimMapping.Roles != "" {
				claimMap.Roles = cfg.ClaimMapping.Roles
			}
			if cfg.ClaimMapping.Tenants != "" {
				claimMap.Tenants = cfg.ClaimMapping.Tenants
			}
		}
		return NewOIDCResolver(ctx, cfg.Issuer, cfg.Audience, claimMap)

	case "firebase":
		if opts.FirebaseVerifier == nil {
			return nil, fmt.Errorf("firebase resolver requires Firebase Admin SDK (is dev_mode on?)")
		}
		return NewFirebaseResolver(opts.FirebaseVerifier), nil

	case "google_oidc":
		return NewGoogleOIDCResolver(opts.CloudRunAudience, opts.SAAllowlist), nil

	case "google_oauth":
		return NewGoogleOAuthResolver(DefaultAccessTokenValidator(), opts.SAAllowlist), nil

	default:
		return nil, fmt.Errorf("unknown resolver type: %q (supported: oidc, firebase, google_oidc, google_oauth)", cfg.Type)
	}
}
