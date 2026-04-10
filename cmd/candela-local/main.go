// candela-local is a lightweight reverse proxy that runs on a developer's
// machine. It injects an IAP-compatible OIDC identity token (via Application
// Default Credentials) into every outbound request to the remote Candela
// server, making authentication seamless for tools like Zed, OpenCode, etc.
//
// Usage:
//
//	candela-local                      # reads ~/.candela.yaml
//	candela-local --config ./my.yaml   # custom config
//	candela-local --remote https://candela-xxx.run.app --audience 12345 --port 8181
//
// Install:
//
//	go install github.com/candelahq/candela/cmd/candela-local@latest
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/oauth2/google"
	"gopkg.in/yaml.v3"
)

// Config holds the candela-local configuration.
type Config struct {
	Remote   string `yaml:"remote"`   // Remote Candela server URL
	Audience string `yaml:"audience"` // IAP OAuth Client ID (OIDC audience)
	Port     int    `yaml:"port"`     // Local port to listen on
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// ── Flags ──
	var (
		configPath string
		remote     string
		audience   string
		port       int
	)
	flag.StringVar(&configPath, "config", "", "path to config file (default: ~/.candela.yaml)")
	flag.StringVar(&remote, "remote", "", "remote Candela server URL")
	flag.StringVar(&audience, "audience", "", "IAP OAuth Client ID")
	flag.IntVar(&port, "port", 0, "local port (default: 8181)")
	flag.Parse()

	// ── Load config ──
	cfg := loadConfig(configPath)

	// CLI flags override config file.
	if remote != "" {
		cfg.Remote = remote
	}
	if audience != "" {
		cfg.Audience = audience
	}
	if port != 0 {
		cfg.Port = port
	}
	if cfg.Port == 0 {
		cfg.Port = 8181
	}

	// ── Validate ──
	if cfg.Remote == "" {
		slog.Error("remote URL is required (set in ~/.candela.yaml or --remote)")
		os.Exit(1)
	}
	if cfg.Audience == "" {
		slog.Error("audience is required (set in ~/.candela.yaml or --audience)")
		os.Exit(1)
	}

	remoteURL, err := url.Parse(cfg.Remote)
	if err != nil {
		slog.Error("invalid remote URL", "url", cfg.Remote, "error", err)
		os.Exit(1)
	}

	// ── Get OIDC token source via ADC ──
	ctx := context.Background()
	tokenSource, err := google.DefaultTokenSource(ctx, cfg.Audience)
	if err != nil {
		slog.Error("failed to get Application Default Credentials — run 'gcloud auth application-default login' first",
			"error", err)
		os.Exit(1)
	}

	// ── Build reverse proxy ──
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Rewrite the request to point at the remote server.
			req.URL.Scheme = remoteURL.Scheme
			req.URL.Host = remoteURL.Host
			req.Host = remoteURL.Host

			// Inject OIDC identity token.
			token, err := tokenSource.Token()
			if err != nil {
				slog.Error("failed to get identity token", "error", err)
				return
			}

			// For IAP, the identity token goes in the Authorization header
			// as a Bearer token. The ID token is in token.Extra("id_token")
			// when using google.DefaultTokenSource. If not available,
			// fall back to the access token.
			if idToken, ok := token.Extra("id_token").(string); ok && idToken != "" {
				req.Header.Set("Authorization", "Bearer "+idToken)
			} else {
				req.Header.Set("Authorization", "Bearer "+token.AccessToken)
			}

			// Preserve the original path.
			if _, ok := req.Header["User-Agent"]; !ok {
				req.Header.Set("User-Agent", "candela-local/1.0")
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("proxy error", "path", r.URL.Path, "error", err)
			http.Error(w, fmt.Sprintf(`{"error": "proxy error: %s"}`, err), http.StatusBadGateway)
		},
	}

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: proxy,
	}

	// ── Graceful shutdown ──
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("🕯️ candela-local proxy started",
			"local", fmt.Sprintf("http://%s", addr),
			"remote", cfg.Remote,
			"audience", cfg.Audience[:min(20, len(cfg.Audience))]+"...")
		slog.Info("Point your tools at:",
			"openai", fmt.Sprintf("http://%s/proxy/openai/v1", addr),
			"anthropic", fmt.Sprintf("http://%s/proxy/anthropic/", addr),
			"google", fmt.Sprintf("http://%s/proxy/google/", addr))

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-sigCtx.Done()
	slog.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("candela-local stopped")
}

// loadConfig reads the candela-local config file.
// Search order: --config flag → $CANDELA_CONFIG → ~/.candela.yaml
func loadConfig(configPath string) *Config {
	if configPath == "" {
		configPath = os.Getenv("CANDELA_CONFIG")
	}
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			configPath = filepath.Join(home, ".candela.yaml")
		}
	}

	cfg := &Config{}
	if configPath == "" {
		return cfg
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read config file", "path", configPath, "error", err)
		}
		return cfg
	}

	// Strip common leading indent (handles terraform output which indents
	// the entire block uniformly). We detect the indent of the first non-empty,
	// non-comment line and strip that prefix from all lines. This preserves
	// nested YAML structure unlike TrimSpace.
	lines := strings.Split(string(data), "\n")
	indent := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent = line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		break
	}
	if indent != "" {
		for i, line := range lines {
			lines[i] = strings.TrimPrefix(line, indent)
		}
	}
	cleaned := strings.Join(lines, "\n")

	if err := yaml.Unmarshal([]byte(cleaned), cfg); err != nil {
		slog.Warn("failed to parse config file", "path", configPath, "error", err)
		return &Config{}
	}

	slog.Info("loaded config", "path", configPath)
	return cfg
}
