package main

import (
	"context"
	"fmt"
	"os"

	"github.com/candelahq/candela/pkg/cloudauth"
)

// handleAuth dispatches auth subcommands.
func handleAuth(args []string) {
	sub := ""
	providerName := ""

	// Parse args: extract --provider flag and subcommand.
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--provider" && i+1 < len(args):
			providerName = args[i+1]
			i++ // skip the value
		default:
			remaining = append(remaining, args[i])
		}
	}
	if len(remaining) > 0 {
		sub = remaining[0]
	}

	switch sub {
	case "login":
		cmdAuthLogin(providerName)
	case "status":
		cmdAuthStatus(providerName)
	case "token":
		cmdAuthToken(providerName)
	default:
		_, _ = fmt.Fprintf(os.Stderr, `candela auth — manage cloud credentials

Usage:
  candela auth login --provider gcp       Login to GCP via browser
  candela auth login --provider aws       Login to AWS (SSO or validates keys)
  candela auth status                     Show credential status (all providers)
  candela auth status --provider gcp      Show GCP credential status
  candela auth token --provider gcp       Print a fresh GCP access token
  candela auth token --provider aws       Print AWS session info

If --provider is omitted for login/token, it is inferred from your config file.
If your config has only one cloud provider type, that provider is used automatically.

Available providers: %v
`, cloudauth.Names())
		if sub != "" {
			os.Exit(1)
		}
	}
}

// resolveProvider determines which cloud auth provider to use.
// Priority: explicit --provider flag > config file inference > error.
func resolveProvider(providerName string) cloudauth.Provider {
	if providerName != "" {
		p, err := cloudauth.Get(providerName)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			_, _ = fmt.Fprintf(os.Stderr, "Available providers: %v\n", cloudauth.Names())
			os.Exit(1)
		}
		return p
	}

	// Try to infer from config file.
	cfg := loadConfig("")
	inferred := inferAuthProviders(cfg)

	switch len(inferred) {
	case 0:
		_, _ = fmt.Fprintf(os.Stderr, "error: --provider is required (no cloud providers configured)\n")
		_, _ = fmt.Fprintf(os.Stderr, "  candela auth login --provider gcp\n")
		_, _ = fmt.Fprintf(os.Stderr, "  candela auth login --provider aws\n")
		os.Exit(1)
	case 1:
		p, err := cloudauth.Get(inferred[0])
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return p
	default:
		_, _ = fmt.Fprintf(os.Stderr, "error: --provider is required (multiple cloud providers in config: %v)\n", inferred)
		_, _ = fmt.Fprintf(os.Stderr, "  candela auth login --provider gcp\n")
		_, _ = fmt.Fprintf(os.Stderr, "  candela auth login --provider aws\n")
		os.Exit(1)
	}
	return nil // unreachable
}

// inferAuthProviders reads the config and returns which cloud auth providers
// are needed based on the configured proxy providers.
func inferAuthProviders(cfg *Config) []string {
	needsGCP := false
	needsAWS := false

	for _, lp := range cfg.Providers {
		switch lp.Name {
		case "google", "gemini", "anthropic", "anthropic-vertex":
			needsGCP = true
		case "anthropic-bedrock":
			needsAWS = true
		}
	}

	var result []string
	if needsGCP {
		result = append(result, "gcp")
	}
	if needsAWS {
		result = append(result, "aws")
	}
	return result
}

// cmdAuthLogin performs the interactive login flow via the provider.
func cmdAuthLogin(providerName string) {
	provider := resolveProvider(providerName)
	ctx := context.Background()
	if err := provider.Login(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// cmdAuthStatus displays credential status. If providerName is empty,
// shows status for all registered providers.
func cmdAuthStatus(providerName string) {
	ctx := context.Background()

	var providers []cloudauth.Provider
	if providerName != "" {
		p, err := cloudauth.Get(providerName)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		providers = []cloudauth.Provider{p}
	} else {
		providers = cloudauth.All()
	}

	for _, p := range providers {
		status, err := p.Status(ctx)
		if err != nil {
			fmt.Printf("● %s: error: %v\n", p.Name(), err)
			continue
		}

		if !status.Valid {
			fmt.Printf("● %s: ❌ not configured\n", status.Provider)
			fmt.Printf("  Run: candela auth login --provider %s\n", status.Provider)
		} else {
			expiryStr := ""
			if status.ExpiresIn > 0 {
				expiryStr = fmt.Sprintf(" (expires in %s)", cloudauth.FormatDuration(status.ExpiresIn))
			}
			fmt.Printf("● %s: ✅ %s%s\n", status.Provider, status.Account, expiryStr)
			fmt.Printf("  path: %s\n", status.FilePath)
		}
	}
}

// cmdAuthToken refreshes and prints a fresh access token (for piping).
func cmdAuthToken(providerName string) {
	provider := resolveProvider(providerName)

	if provider.Name() == "aws" {
		// AWS doesn't use Bearer tokens — show session info instead.
		ctx := context.Background()
		status, err := provider.Status(ctx)
		if err != nil || !status.Valid {
			_, _ = fmt.Fprintf(os.Stderr, "error: AWS credentials not valid — run: candela auth login --provider aws\n")
			os.Exit(1)
		}
		fmt.Printf("# AWS uses SigV4 request signing, not Bearer tokens.\n")
		fmt.Printf("# Identity: %s\n", status.Account)
		if status.ExpiresIn > 0 {
			fmt.Printf("# Expires in: %s\n", cloudauth.FormatDuration(status.ExpiresIn))
		}
		return
	}

	// GCP: refresh and print access token (for piping to curl, etc.)
	path, err := cloudauth.ADCPath()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	creds, err := cloudauth.ReadADC(path)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: no credentials found — run: candela auth login --provider gcp\n")
		os.Exit(1)
	}

	token, err := cloudauth.RefreshFromADC(creds)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: failed to refresh token: %v\n", err)
		os.Exit(1)
	}

	// Print just the token (suitable for piping to curl, etc.).
	fmt.Print(token.AccessToken)
}
