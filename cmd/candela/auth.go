package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

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
  candela auth login                      Login (auto-detects or prompts for provider)
  candela auth login --provider gcp       Login to GCP via browser
  candela auth login --provider aws       Login to AWS (SSO or validates keys)
  candela auth status                     Show credential status (all providers)
  candela auth status --provider gcp      Show GCP credential status
  candela auth token --provider gcp       Print a fresh GCP access token
  candela auth token --provider aws       Print AWS session info

If --provider is omitted for login, it is inferred from your config file.
If no config exists, you'll be prompted to choose.

Available providers: %v
`, cloudauth.Names())
		if sub != "" {
			os.Exit(1)
		}
	}
}

// resolveProvider determines which cloud auth provider to use.
// Priority: explicit --provider flag > config file inference > interactive prompt.
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
		// No providers configured — prompt interactively.
		return promptForProvider()
	case 1:
		p, err := cloudauth.Get(inferred[0])
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return p
	default:
		// Multiple providers — prompt to choose.
		return promptForProviderFrom(inferred)
	}
}

// promptForProvider asks the user to select a cloud provider interactively.
func promptForProvider() cloudauth.Provider {
	names := cloudauth.Names()
	return promptForProviderFrom(names)
}

// promptForProviderFrom presents a numbered list and reads the user's choice.
func promptForProviderFrom(names []string) cloudauth.Provider {
	if len(names) == 0 {
		_, _ = fmt.Fprintf(os.Stderr, "error: no cloud auth providers available\n")
		os.Exit(1)
	}

	descriptions := map[string]string{
		"gcp": "Google Cloud Platform",
		"aws": "Amazon Web Services",
	}

	fmt.Println("\nSelect a cloud provider to authenticate:")
	for i, name := range names {
		desc := descriptions[name]
		if desc == "" {
			desc = name
		}
		fmt.Printf("  %d. %s — %s\n", i+1, name, desc)
	}

	fmt.Printf("\nChoice [1]: ")
	var input string
	_, _ = fmt.Scanln(&input)

	// Default to first option.
	choice := 0
	if input != "" {
		var n int
		if _, err := fmt.Sscanf(input, "%d", &n); err == nil && n >= 1 && n <= len(names) {
			choice = n - 1
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "error: invalid choice %q\n", input)
			os.Exit(1)
		}
	}

	p, err := cloudauth.Get(names[choice])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return p
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

	// Offer to save the provider to config and restart if needed.
	postLoginSetup(provider.Name())
}

// postLoginSetup persists the provider to config and restarts the proxy if running.
func postLoginSetup(providerName string) {
	changed, err := upsertConfigProvider(providerName)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not update config: %v\n", err)
		return
	}

	if changed {
		path := configFilePath()
		fmt.Printf("\n📝 Added provider to %s\n", path)
	}

	// Check if the proxy is running and offer to restart.
	if isProxyRunning() {
		if changed {
			fmt.Print("\n🔄 Candela proxy is running. Restart to apply new config? [Y/n] ")
		} else {
			fmt.Print("\n🔄 Candela proxy is running. Restart to pick up fresh credentials? [Y/n] ")
		}
		var input string
		_, _ = fmt.Scanln(&input)
		if input == "" || input == "y" || input == "Y" || input == "yes" {
			restartProxy()
		}
	}
}

// isProxyRunning checks if the candela background proxy is alive via PID file.
func isProxyRunning() bool {
	pidPath := pidFilePath()
	if pidPath == "" {
		return false
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil || pid == 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks if process exists without sending a signal.
	return process.Signal(syscall.Signal(0)) == nil
}

// restartProxy stops the running proxy and starts it again.
func restartProxy() {
	fmt.Println("Stopping proxy...")
	cmdStop()
	fmt.Println("Starting proxy...")
	cmdStart()
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
