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
	if len(args) > 0 {
		sub = args[0]
	}

	// Resolve the target provider. Currently defaults to GCP;
	// future: parse --provider flag from args.
	provider := cloudauth.Default()

	switch sub {
	case "login":
		cmdAuthLogin(provider)
	case "status":
		cmdAuthStatus(provider)
	case "token":
		cmdAuthToken()
	default:
		_, _ = fmt.Fprintf(os.Stderr, `candela auth — manage cloud credentials

Usage:
  candela auth login      Login via browser (writes credentials)
  candela auth status     Show current credential status
  candela auth token      Print a fresh access token
`)
		if sub != "" {
			os.Exit(1)
		}
	}
}

// cmdAuthLogin performs the interactive login flow via the provider.
func cmdAuthLogin(provider cloudauth.Provider) {
	ctx := context.Background()
	if err := provider.Login(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// cmdAuthStatus reads credentials and displays status via the provider.
func cmdAuthStatus(provider cloudauth.Provider) {
	ctx := context.Background()
	status, err := provider.Status(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if !status.Valid {
		fmt.Println("● credentials: not configured")
		fmt.Printf("  Run: candela auth login\n")
		return
	}

	fmt.Printf("● credentials: %s\n", status.FilePath)
	fmt.Printf("  provider: %s\n", status.Provider)
	fmt.Printf("  account:  %s\n", status.Account)
	fmt.Printf("  status:   ✅ valid (expires in %s)\n", cloudauth.FormatDuration(status.ExpiresIn))
}

// cmdAuthToken refreshes and prints a fresh access token (for piping).
// This still uses the GCP-specific ADC path directly since the token
// command needs to work without any provider resolution complexity.
func cmdAuthToken() {
	path, err := cloudauth.ADCPath()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	creds, err := cloudauth.ReadADC(path)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: no credentials found — run: candela auth login\n")
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
