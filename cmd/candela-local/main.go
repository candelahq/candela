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
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
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

	"github.com/candelahq/candela/gen/go/candela/v1/candelav1connect"
	"github.com/candelahq/candela/pkg/runtime"

	// Register runtime backends.
	_ "github.com/candelahq/candela/pkg/runtime/lmstudio"
	_ "github.com/candelahq/candela/pkg/runtime/ollama"
	_ "github.com/candelahq/candela/pkg/runtime/vllm"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
	"gopkg.in/yaml.v3"
)

//go:embed ui
var uiFS embed.FS

// Config holds the candela-local configuration.
type Config struct {
	Remote        string `yaml:"remote"`         // Remote Candela server URL
	Audience      string `yaml:"audience"`       // IAP OAuth Client ID (OIDC audience)
	Port          int    `yaml:"port"`           // Local port to listen on
	LMStudioPort  int    `yaml:"lmstudio_port"`  // LM Studio compat listener port (default: 1234)
	LocalUpstream string `yaml:"local_upstream"` // Local runtime URL (e.g. http://127.0.0.1:11434)
	StateDBPath   string `yaml:"state_db_path"`  // Path to SQLite state DB (default: ~/.candela/state.db)

	// Runtime management configuration.
	RuntimeBackend string                `yaml:"runtime_backend"` // "ollama", "vllm", "lmstudio"
	RuntimeConfig  runtime.Config        `yaml:"runtime_config"`  // Host, port, args for the backend
	RuntimeManage  runtime.ManagerConfig `yaml:"runtime_manage"`  // Auto-start, auto-pull, models
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

	// ── Get OIDC ID token source via ADC ──
	// Strategy 1: idtoken.NewTokenSource (works for service accounts).
	// Strategy 2: google.DefaultTokenSource with openid scope (works for user credentials).
	ctx := context.Background()
	var tokenSource oauth2.TokenSource
	useIDToken := false

	ts, err := idtoken.NewTokenSource(ctx, cfg.Audience)
	if err == nil {
		slog.Info("using service account ID token source")
		tokenSource = ts
		useIDToken = true
	} else {
		slog.Info("idtoken unavailable (user credentials), using OAuth2 with openid scope", "reason", err)
		ts2, err2 := google.DefaultTokenSource(ctx, "openid", "email")
		if err2 != nil {
			slog.Error("failed to get credentials — run 'gcloud auth application-default login' first",
				"error", err2)
			os.Exit(1)
		}
		tokenSource = ts2
	}

	// ── Build reverse proxy ──
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Rewrite the request to point at the remote server.
			req.URL.Scheme = remoteURL.Scheme
			req.URL.Host = remoteURL.Host
			req.Host = remoteURL.Host

			// Inject OIDC identity token for IAP.
			// For user credentials (OAuth2), the ID token is in token.Extra("id_token").
			// For service account credentials (idtoken pkg), AccessToken IS the ID token.
			token, err := tokenSource.Token()
			if err != nil {
				slog.Error("failed to get identity token", "error", err)
				return
			}
			bearerToken := token.AccessToken
			if useIDToken {
				if idToken, ok := token.Extra("id_token").(string); ok && idToken != "" {
					bearerToken = idToken
				}
			}
			req.Header.Set("Authorization", "Bearer "+bearerToken)

			// Preserve the original path.
			if _, ok := req.Header["User-Agent"]; !ok {
				req.Header.Set("User-Agent", "candela-local/1.0")
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("proxy error", "path", r.URL.Path, "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("proxy error: %v", err)})
		},
	}

	// ── Local runtime proxy ──
	// If a local upstream is configured, route /proxy/local/ directly to the
	// local runtime (Ollama, vLLM, LM Studio) without a cloud round-trip.
	mux := http.NewServeMux()

	if cfg.LocalUpstream != "" {
		localURL, err := url.Parse(cfg.LocalUpstream)
		if err != nil {
			slog.Error("invalid local_upstream URL", "url", cfg.LocalUpstream, "error", err)
			os.Exit(1)
		}

		localProxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				// Strip the /proxy/local prefix and prepend the upstream path.
				req.URL.Scheme = localURL.Scheme
				req.URL.Host = localURL.Host
				req.Host = localURL.Host
				stripped := strings.TrimPrefix(req.URL.Path, "/proxy/local")
				if stripped == "" {
					stripped = "/"
				}
				req.URL.Path = singleJoiningSlash(localURL.Path, stripped)
				if _, ok := req.Header["User-Agent"]; !ok {
					req.Header.Set("User-Agent", "candela-local/1.0")
				}
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				slog.Error("local proxy error", "path", r.URL.Path, "error", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("local runtime unavailable: %v", err)})
			},
		}
		mux.Handle("/proxy/local/", localProxy)
		slog.Info("🏠 local runtime proxy enabled", "upstream", cfg.LocalUpstream)
	}

	// ── Runtime management (ConnectRPC + embedded UI) ──
	var mgr *runtime.Manager
	var stateDB *StateDB

	// Open state DB (best-effort — not required to run).
	statePath := cfg.StateDBPath
	if statePath == "" {
		statePath = "~/.candela/state.db"
	}
	stateDB, err = openStateDB(statePath)
	if err != nil {
		slog.Warn("state DB unavailable (running without persistence)", "error", err)
		stateDB = nil
	} else {
		slog.Info("📦 state DB opened", "path", statePath)
	}

	if cfg.RuntimeBackend != "" {
		rt, err := runtime.New(cfg.RuntimeBackend, cfg.RuntimeConfig)
		if err != nil {
			slog.Error("failed to create runtime", "backend", cfg.RuntimeBackend, "error", err)
			os.Exit(1)
		}
		mgr = runtime.NewManager(rt, cfg.RuntimeManage)
		if err := mgr.Start(ctx); err != nil {
			slog.Error("failed to start runtime manager", "error", err)
			os.Exit(1)
		}
		// Record in state DB.
		if stateDB != nil {
			_ = stateDB.SetRuntimeState(cfg.RuntimeBackend, "")
		}
	}

	// Mount ConnectRPC RuntimeService.
	handler := newRuntimeHandler(mgr, stateDB, ctx)
	rpcPath, rpcHandler := candelav1connect.NewRuntimeServiceHandler(handler)
	mux.Handle(rpcPath, rpcHandler)

	// Mount active pulls REST endpoint for the embedded UI.
	mux.HandleFunc("/_local/api/pulls", handler.ServeActivePulls)

	// Mount embedded UI at /_local/.
	uiContent, err := fs.Sub(uiFS, "ui")
	if err != nil {
		slog.Error("failed to load embedded UI", "error", err)
		os.Exit(1)
	}
	mux.Handle("/_local/", http.StripPrefix("/_local/", http.FileServer(http.FS(uiContent))))

	slog.Info("🔧 management UI enabled",
		"ui", fmt.Sprintf("http://127.0.0.1:%d/_local/", cfg.Port),
		"rpc", rpcPath)

	// Everything else → remote Candela server.
	mux.Handle("/", proxy)

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// ── LM Studio compat listener ──
	// Starts a secondary proxy on a separate port (default 1234) so IntelliJ's
	// "Enable LM Studio" checkbox works with zero URL changes. The remote
	// Candela server serves /v1/models, /v1/chat/completions, and /api/v0/models.
	lmPort := cfg.LMStudioPort
	if lmPort == 0 {
		lmPort = 1234
	}
	var lmSrv *http.Server

	// ── Graceful shutdown ──
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("🕯️ candela-local proxy started",
			"local", fmt.Sprintf("http://%s", addr),
			"remote", cfg.Remote,
			"audience", cfg.Audience[:min(20, len(cfg.Audience))]+"...")
		logFields := []any{
			"openai", fmt.Sprintf("http://%s/proxy/openai/v1", addr),
			"anthropic", fmt.Sprintf("http://%s/proxy/anthropic/", addr),
			"google", fmt.Sprintf("http://%s/proxy/google/", addr),
		}
		if cfg.LocalUpstream != "" {
			logFields = append(logFields, "local", fmt.Sprintf("http://%s/proxy/local/v1", addr))
		}
		slog.Info("Point your tools at:", logFields...)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Start LM Studio compat listener on separate port.
	lmAddr := fmt.Sprintf("127.0.0.1:%d", lmPort)
	lmSrv = &http.Server{Addr: lmAddr, Handler: proxy}
	go func() {
		slog.Info("🖥️ LM Studio compat listener started",
			"addr", fmt.Sprintf("http://%s", lmAddr),
			"models", fmt.Sprintf("http://%s/v1/models", lmAddr))
		if err := lmSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Warn("LM Studio listener failed (port may be in use)", "addr", lmAddr, "error", err)
		}
	}()

	<-sigCtx.Done()
	slog.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if lmSrv != nil {
		if err := lmSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("LM Studio shutdown error", "error", err)
		}
	}
	if mgr != nil {
		if err := mgr.Stop(shutdownCtx); err != nil {
			slog.Error("runtime manager shutdown error", "error", err)
		}
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	if stateDB != nil {
		_ = stateDB.Close()
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

// singleJoiningSlash joins two URL path segments with exactly one slash.
// This is the same logic used by net/http/httputil.NewSingleHostReverseProxy.
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}
