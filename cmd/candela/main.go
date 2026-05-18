// candela is a lightweight reverse proxy and CLI for LLM observability.
// It runs on a developer's machine, providing traces, costs, and budgets
// for AI-powered dev tools.
//
// Usage:
//
//	candela start                      # start proxy in background
//	candela stop                       # stop the background proxy
//	candela status                     # check if proxy is running
//	candela run                        # run in foreground (debug)
//	candela run --config ./my.yaml     # custom config
//	candela version                    # print version info
//
// Install:
//
//	brew install candelahq/tap/candela
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
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/candelahq/candela/gen/go/candela/v1/candelav1connect"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/processor"
	"github.com/candelahq/candela/pkg/proxy"
	"github.com/candelahq/candela/pkg/proxy/worker"
	"github.com/candelahq/candela/pkg/runtime"
	"github.com/candelahq/candela/pkg/session"
	"github.com/candelahq/candela/pkg/storage"

	// Register runtime backends.
	_ "github.com/candelahq/candela/pkg/runtime/lmstudio"
	_ "github.com/candelahq/candela/pkg/runtime/ollama"
	_ "github.com/candelahq/candela/pkg/runtime/vllm"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
	"gopkg.in/yaml.v3"

	"github.com/candelahq/candela/pkg/storage/otlpexporter"
	sqlitestore "github.com/candelahq/candela/pkg/storage/sqlite"
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

	// Direct cloud providers (solo mode — call Gemini/Claude without a server).
	Providers []LocalProvider `yaml:"providers"`
	VertexAI  VertexAIConfig  `yaml:"vertex_ai"`
}

// LocalProvider configures a direct cloud provider for solo mode.
type LocalProvider struct {
	Name   string   `yaml:"name"`   // "google", "anthropic"
	Models []string `yaml:"models"` // Model IDs to expose (e.g. "gemini-2.5-pro")
}

// VertexAIConfig holds GCP Vertex AI settings for direct cloud providers.
type VertexAIConfig struct {
	Project     string `yaml:"project"`      // GCP project ID (required)
	Region      string `yaml:"region"`       // GCP region (default: us-central1)
	CachingMode string `yaml:"caching_mode"` // off|auto|system-only (default: auto)
}

// version is set at build time via ldflags.
var version = "dev"

// pidFilePath returns the path to the PID file: ~/.candela/candela.pid
func pidFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".candela", "candela.pid")
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Subcommand dispatch.
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "run":
		// Strip "run" from os.Args so flag.Parse sees the flags.
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runForeground()
	case "start":
		cmdStart()
	case "stop":
		cmdStop()
	case "status":
		cmdStatus()
	case "auth":
		handleAuth(os.Args[2:])
	case "version":
		fmt.Printf("candela %s\n", version)
	default:
		// No subcommand or unknown → show help.
		fmt.Fprintf(os.Stderr, `candela — LLM observability proxy

Usage:
  candela start          Start proxy in background
  candela stop           Stop the background proxy
  candela status         Show proxy status
  candela run [flags]    Run in foreground
  candela auth login     Login via browser (Google OAuth)
  candela auth status    Show credential status
  candela auth token     Print a fresh access token
  candela version        Print version

Run flags:
  --config <path>        Config file (default: ~/.config/candela/config.yaml)
  --remote <url>         Remote Candela server URL
  --audience <id>        IAP OAuth Client ID
  --port <port>          Local port (default: 8181)
`)
		os.Exit(1)
	}
}

// cmdStart launches `candela run` as a background process and writes a PID file.
func cmdStart() {
	pidPath := pidFilePath()
	if pidPath == "" {
		slog.Error("cannot determine home directory")
		os.Exit(1)
	}

	// Check if already running.
	if data, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			if process, err := os.FindProcess(pid); err == nil {
				// Signal 0 checks if process exists.
				if process.Signal(syscall.Signal(0)) == nil {
					fmt.Printf("🕯️ candela is already running (PID %d)\n", pid)
					return
				}
			}
		}
		// Stale PID file — clean up.
		_ = os.Remove(pidPath)
	}

	// Ensure ~/.candela/ exists.
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o700); err != nil {
		slog.Error("failed to create candela directory", "error", err)
		os.Exit(1)
	}

	// Find our own executable.
	exe, err := os.Executable()
	if err != nil {
		slog.Error("cannot find candela executable", "error", err)
		os.Exit(1)
	}

	// Open log file for the background process.
	// Rotate: truncate if over 10 MB to prevent unbounded growth.
	logPath := filepath.Join(filepath.Dir(pidPath), "candela.log")
	const maxLogSize = 10 << 20 // 10 MB
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogSize {
		_ = os.Truncate(logPath, 0)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		slog.Warn("cannot open log file — background output will be lost", "path", logPath, "error", err)
	}

	// Forward any extra args after "start" to "run".
	args := append([]string{"run"}, os.Args[2:]...)
	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		slog.Error("failed to start candela", "error", err)
		os.Exit(1)
	}

	// Write PID file.
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		slog.Warn("failed to write PID file", "path", pidPath, "error", err)
	}

	// Resolve port from args or config for display.
	port := resolvePort(os.Args[2:])

	fmt.Printf("🕯️ candela started (PID %d)\n", cmd.Process.Pid)
	fmt.Printf("   proxy: http://127.0.0.1:%d\n", port)
	fmt.Printf("   UI:    http://127.0.0.1:%d/_local/\n", port)
	fmt.Printf("   logs:  %s\n", logPath)
}

// cmdStop reads the PID file and sends SIGTERM.
func cmdStop() {
	pidPath := pidFilePath()
	if pidPath == "" {
		slog.Error("cannot determine home directory")
		os.Exit(1)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Println("candela is not running (no PID file)")
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fmt.Println("candela is not running (invalid PID file)")
		_ = os.Remove(pidPath)
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil || process.Signal(syscall.Signal(0)) != nil {
		fmt.Println("candela is not running (stale PID file)")
		_ = os.Remove(pidPath)
		return
	}

	// Send SIGTERM for graceful shutdown.
	if err := process.Signal(syscall.SIGTERM); err != nil {
		slog.Error("failed to stop candela", "pid", pid, "error", err)
		os.Exit(1)
	}

	// Wait for process to exit before removing PID file.
	for i := 0; i < 10; i++ {
		if process.Signal(syscall.Signal(0)) != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	_ = os.Remove(pidPath)
	fmt.Printf("🛑 candela stopped (PID %d)\n", pid)
}

// cmdStatus checks the PID file and health endpoint.
func cmdStatus() {
	pidPath := pidFilePath()
	if pidPath == "" {
		fmt.Println("● candela: stopped")
		return
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Println("● candela: stopped")
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fmt.Println("● candela: stopped (invalid PID file)")
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil || process.Signal(syscall.Signal(0)) != nil {
		fmt.Println("● candela: stopped (stale PID file)")
		_ = os.Remove(pidPath)
		return
	}

	// Check health endpoint — /_local/ is always served regardless of mode.
	port := resolvePort(nil)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/_local/", port))
	status := "running (not responding)"
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			status = "running"
		}
	}

	fmt.Printf("● candela: %s\n", status)
	fmt.Printf("  PID:   %d\n", pid)
	fmt.Printf("  proxy: http://127.0.0.1:%d\n", port)
	fmt.Printf("  UI:    http://127.0.0.1:%d/_local/\n", port)
}

// resolvePort returns the configured port by checking args then config file.
func resolvePort(args []string) int {
	// Check --port in args.
	for i, arg := range args {
		if arg == "--port" && i+1 < len(args) {
			if p, err := strconv.Atoi(args[i+1]); err == nil {
				return p
			}
		}
	}
	// Check config file.
	cfg := loadConfig("")
	if cfg.Port != 0 {
		return cfg.Port
	}
	return 8181
}

// runForeground runs the proxy in the foreground (the original main behavior).
func runForeground() {
	// ── Flags ──
	var (
		configPath string
		remote     string
		audience   string
		port       int
	)
	flag.StringVar(&configPath, "config", "", "path to config file (default: ~/.config/candela/config.yaml)")
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

	// ── Validate & mode detection ──
	ctx := context.Background()
	soloMode := cfg.Remote == ""

	var remoteProxy *httputil.ReverseProxy
	var spanProc *processor.SpanProcessor
	var traceReader storage.SpanReader

	if soloMode {
		slog.Info("🏠 solo mode — no remote server configured, local-only operation")
		if cfg.Audience != "" {
			slog.Warn("audience is set but remote is empty — audience will be ignored")
		}

		// Initialize local traces store for observability.
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			slog.Warn("could not resolve home directory — observability disabled", "error", homeErr)
		} else {
			tracesDir := filepath.Join(home, ".candela")
			if err := os.MkdirAll(tracesDir, 0o700); err != nil {
				slog.Warn("failed to create traces directory", "path", tracesDir, "error", err)
			}
			tracesPath := filepath.Join(tracesDir, "traces.db")
			traceStore, err := sqlitestore.New(sqlitestore.Config{Path: tracesPath})
			if err != nil {
				slog.Warn("failed to initialize traces store — observability disabled", "error", err)
			} else {
				defer func() { _ = traceStore.Close() }()
				calc := costcalc.New()
				spanProc = processor.New([]storage.SpanWriter{traceStore}, calc, 50)
				go spanProc.Run(ctx)
				defer spanProc.Stop()
				traceReader = traceStore
				slog.Info("📊 local traces enabled", "path", tracesPath)
			}
		}
	} else {
		if cfg.Audience == "" {
			slog.Error("audience is required when remote is set (set in ~/.config/candela/config.yaml or --audience)")
			os.Exit(1)
		}

		remoteURL, err := url.Parse(cfg.Remote)
		if err != nil {
			slog.Error("invalid remote URL", "url", cfg.Remote, "error", err)
			os.Exit(1)
		}

		// ── Get auth token source via ADC ──
		// Strategy 1: idtoken.NewTokenSource (works for service accounts).
		//   → AccessToken IS the audience-scoped OIDC ID token.
		// Strategy 2: google.DefaultTokenSource (works for user credentials).
		//   → AccessToken is an OAuth2 access token. The server validates it
		//     via the userinfo endpoint (Strategy 3 in FirebaseAuthMiddleware).
		//
		// In both cases, token.AccessToken contains the correct bearer token.
		// Do NOT use token.Extra("id_token") — for user credentials it returns
		// a generic OIDC token without the required audience claim, which the
		// server rejects.
		var tokenSource oauth2.TokenSource

		ts, err := idtoken.NewTokenSource(ctx, cfg.Audience)
		if err == nil {
			slog.Info("using service account ID token source")
			tokenSource = ts
		} else {
			slog.Debug("idtoken.NewTokenSource unavailable (user credentials fallback)", "reason", err)
			ts2, err2 := google.DefaultTokenSource(ctx, "openid", "email")
			if err2 != nil {
				slog.Error("failed to get credentials — run 'candela auth login' first",
					"error", err2)
				os.Exit(1)
			}
			slog.Info("using user ADC credentials (OAuth2 access token)")
			tokenSource = ts2
		}

		// ── Build reverse proxy ──
		remoteProxy = &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				// Rewrite the request to point at the remote server.
				req.URL.Scheme = remoteURL.Scheme
				req.URL.Host = remoteURL.Host
				req.Host = remoteURL.Host

				// Inject auth token for the remote server.
				// For service accounts, AccessToken is the audience-scoped OIDC ID token.
				// For user credentials, AccessToken is the OAuth2 access token which
				// the server validates via Google's userinfo endpoint.
				token, err := tokenSource.Token()
				if err != nil {
					slog.Error("failed to get auth token", "error", err)
					return
				}
				req.Header.Set("Authorization", "Bearer "+token.AccessToken)

				// Preserve the original path.
				if _, ok := req.Header["User-Agent"]; !ok {
					req.Header.Set("User-Agent", "candela/1.0")
				}
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				slog.Error("proxy error", "path", r.URL.Path, "error", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "remote server unavailable"})
			},
		}

		// Initialize local traces store for offline sync.
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			slog.Warn("could not resolve home directory — offline sync disabled", "error", homeErr)
		} else {
			tracesDir := filepath.Join(home, ".candela")
			if err := os.MkdirAll(tracesDir, 0o700); err != nil {
				slog.Warn("failed to create traces directory", "path", tracesDir, "error", err)
			}
			tracesPath := filepath.Join(tracesDir, "traces.db")
			traceStore, err := sqlitestore.New(sqlitestore.Config{Path: tracesPath})
			if err != nil {
				slog.Warn("failed to initialize traces store — offline sync disabled", "error", err)
			} else {
				defer func() { _ = traceStore.Close() }()

				// OTLP exporter pointing to Candela server
				// For HTTP, standard path is /v1/traces
				endpoint := singleJoiningSlash(cfg.Remote, "/v1/traces")

				// Re-use tokenSource for the exporter's Auth header if needed
				var headers map[string]string
				if token, err := tokenSource.Token(); err == nil {
					headers = map[string]string{
						"Authorization": "Bearer " + token.AccessToken,
					}
				}

				otlpCfg := otlpexporter.Config{
					Endpoint:    endpoint,
					Protocol:    "http",
					Headers:     headers,
					Compression: "gzip",
				}

				upstream, err := otlpexporter.New(ctx, otlpCfg)
				if err != nil {
					slog.Warn("failed to initialize OTLP exporter for sync worker", "error", err)
				} else {
					defer func() { _ = upstream.Close() }()

					// Start the Sync Worker
					syncWorker := worker.NewSyncWorker(traceStore, upstream, 5*time.Second)
					syncWorker.Start()
					defer syncWorker.Stop()

					// Also initialize spanProc to capture local runtime traces
					calc := costcalc.New()
					spanProc = processor.New([]storage.SpanWriter{traceStore}, calc, 50)
					go spanProc.Run(ctx)
					defer spanProc.Stop()
					traceReader = traceStore

					slog.Info("🔄 offline sync worker enabled", "endpoint", endpoint)
				}
			}
		}
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
					req.Header.Set("User-Agent", "candela/1.0")
				}
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				slog.Error("local proxy error", "path", r.URL.Path, "error", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "local runtime unavailable"})
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
	stateDB, err := openStateDB(statePath)
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

	// Register traces API endpoint (solo mode observability).
	mux.Handle("/_local/api/traces", newTracesHandler(traceReader))
	mux.Handle("/_local/api/leaderboard", newLeaderboardHandler(traceReader))

	// Register runtime config API — cloudProxy is nil until built below.
	configAPI := &localAPI{}
	mux.HandleFunc("GET /_local/api/config", configAPI.handleGetConfig)
	mux.HandleFunc("POST /_local/api/config/caching", configAPI.handleSetCaching)

	// Everything else → remote Candela server (if configured).
	if remoteProxy != nil {
		mux.Handle("/", remoteProxy)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "solo mode — no remote server configured"})
		})
	}

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Minute, // generous for streaming LLM responses
		IdleTimeout:       120 * time.Second,
	}

	// ── LM Studio compat listener ──
	// Starts a secondary listener on a separate port (default 1234) so IntelliJ's
	// "Enable LM Studio" checkbox works with zero URL changes.
	// Uses a smart handler that merges local runtime models with remote cloud
	// models and routes chat/completions to the right backend.
	lmPort := cfg.LMStudioPort
	if lmPort == 0 {
		lmPort = 1234
	}
	var lmSrv *http.Server

	// Build a local proxy for the runtime (use explicit local_upstream or runtime endpoint).
	var runtimeLocalProxy *httputil.ReverseProxy
	if cfg.LocalUpstream != "" {
		// Reuse the already-configured local upstream proxy target.
		runtimeLocalProxy = buildLocalProxy(cfg.LocalUpstream)
	} else if mgr != nil {
		// Auto-create a local proxy pointing at the runtime endpoint.
		runtimeLocalProxy = buildLocalProxy(mgr.Runtime().Endpoint())
	}

	// Create the session resolver chain for conversation tracking.
	sessionResolver := session.NewChainResolver(
		session.NewHeaderResolver(""),              // Explicit X-Session-Id header wins.
		session.NewUserMsgResolver(30*time.Minute), // Heuristic: first user message fingerprint.
	)

	// Build the smart LM handler.
	// In solo mode with traces enabled, wrap local proxy with span capture.
	var localHandler http.Handler
	if runtimeLocalProxy != nil {
		localHandler = newSpanCapture(runtimeLocalProxy, spanProc, sessionResolver)
	}

	// Build direct cloud proxy if providers are configured (solo + cloud mode).
	var cloudProxy *proxy.Proxy
	var cloudCalc *costcalc.Calculator
	cloudModels := make(map[string]string)
	if soloMode && len(cfg.Providers) > 0 {
		cloudProxy, cloudModels = buildCloudProxy(*cfg, spanProc)
	}
	// Wire the cloud proxy into the config API for runtime caching control.
	configAPI.cloudProxy = cloudProxy
	// Create a calc for pricing-based model filtering.
	// Uses the same defaults as the cloud proxy's embedded calc.
	if len(cloudModels) > 0 {
		cloudCalc = costcalc.New()
	}

	lmH := newLMHandler(mgr, remoteProxy, runtimeLocalProxy, localHandler, cloudProxy, cloudModels, cloudCalc)
	lmAddr := fmt.Sprintf("127.0.0.1:%d", lmPort)

	// ── Graceful shutdown ──
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if soloMode {
			slog.Info("🕯️ candela started (solo mode)",
				"local", fmt.Sprintf("http://%s", addr))
		} else {
			slog.Info("🕯️ candela proxy started",
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
		}

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Start LM Studio compat listener on separate port.
	lmSrv = &http.Server{
		Addr:              lmAddr,
		Handler:           lmH,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
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
	slog.Info("candela stopped")
}

// loadConfig reads the candela-local config file.
// Search order:
//  1. --config flag
//  2. $CANDELA_CONFIG env var
//  3. ~/.config/candela/config.yaml  (preferred — works on all platforms)
//  4. os.UserConfigDir()/candela/config.yaml  (~/Library/Application Support on macOS)
//  5. ~/.candela.yaml  (legacy)
func loadConfig(configPath string) *Config {
	if configPath == "" {
		configPath = os.Getenv("CANDELA_CONFIG")
	}
	if configPath == "" {
		// Check ~/.config/candela/config.yaml explicitly — this is the
		// documented preferred location and works cross-platform regardless
		// of what os.UserConfigDir() returns (which is ~/Library/Application
		// Support on macOS, not ~/.config).
		if home, err := os.UserHomeDir(); err == nil {
			dotConfigPath := filepath.Join(home, ".config", "candela", "config.yaml")
			if _, err := os.Stat(dotConfigPath); err == nil {
				configPath = dotConfigPath
			}
		}
	}
	if configPath == "" {
		// Platform-native config dir (e.g. ~/Library/Application Support on macOS).
		if configDir, err := os.UserConfigDir(); err == nil {
			nativePath := filepath.Join(configDir, "candela", "config.yaml")
			if _, err := os.Stat(nativePath); err == nil {
				configPath = nativePath
			}
		}
		// Fallback to legacy location.
		if configPath == "" {
			if home, err := os.UserHomeDir(); err == nil {
				configPath = filepath.Join(home, ".candela.yaml")
			}
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

// buildLocalProxy creates a reverse proxy to a local runtime endpoint.
func buildLocalProxy(upstream string) *httputil.ReverseProxy {
	u, err := url.Parse(upstream)
	if err != nil {
		slog.Warn("invalid local proxy URL", "url", upstream, "error", err)
		return nil
	}
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = u.Scheme
			req.URL.Host = u.Host
			req.Host = u.Host
			if _, ok := req.Header["User-Agent"]; !ok {
				req.Header.Set("User-Agent", "candela/1.0")
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("local proxy error", "path", r.URL.Path, "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "local runtime unavailable"})
		},
	}
}

// buildCloudProxy creates a proxy.Proxy for direct cloud model access in solo mode.
// Uses Google ADC for authentication to Vertex AI endpoints (Gemini + Anthropic).
func buildCloudProxy(cfg Config, submitter *processor.SpanProcessor) (*proxy.Proxy, map[string]string) {
	region := cfg.VertexAI.Region
	if region == "" {
		region = "us-central1"
	}

	// Resolve GCP project: config > gcloud fallback.
	project := cfg.VertexAI.Project
	if project == "" {
		// Try gcloud config as fallback.
		gcloudCtx, gcloudCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer gcloudCancel()
		if out, err := exec.CommandContext(gcloudCtx, "gcloud", "config", "get", "project").Output(); err == nil {
			project = strings.TrimSpace(string(out))
			if project != "" {
				slog.Warn("vertex_ai.project not set in config, using gcloud default", "project", project)
			}
		}
	}
	if project == "" {
		slog.Error("vertex_ai.project is required for direct cloud providers — set it in ~/.config/candela/config.yaml")
		return nil, nil
	}

	// Get ADC token source.
	tokenSource, adcErr := google.DefaultTokenSource(context.Background(),
		"https://www.googleapis.com/auth/cloud-platform")
	if adcErr != nil {
		slog.Error("failed to get Google ADC — run 'candela auth login'", "error", adcErr)
		return nil, nil
	}

	// Build providers from config.
	var providers []proxy.Provider
	cloudModels := make(map[string]string)

	for _, lp := range cfg.Providers {
		p := proxy.Provider{Name: lp.Name, TokenSource: tokenSource}

		switch lp.Name {
		case "google", "gemini":
			p.Name = "gemini-oai"
			p.UpstreamURL = "https://generativelanguage.googleapis.com/v1beta/openai"
		case "anthropic":
			p.UpstreamURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com", region)
			ft := &proxy.AnthropicFormatTranslator{}
			if cfg.VertexAI.CachingMode != "" {
				ft.SetCachingMode(proxy.ParseCachingMode(cfg.VertexAI.CachingMode))
			}
			p.FormatTranslator = ft
			p.PathRewriter = &proxy.VertexAIPathRewriter{
				ProjectID: project,
				Region:    region,
			}
		case "anthropic-direct":
			// Native Anthropic Messages API passthrough — client provides its own
			// API key via x-api-key or Authorization header. No ADC, no Vertex AI.
			// This is the Claude Code LLM gateway mode.
			p.UpstreamURL = "https://api.anthropic.com"
			p.TokenSource = nil // Client manages auth, not ADC.
		case "anthropic-vertex":
			// Native Anthropic Messages API routed via Vertex AI rawPredict.
			// Candela injects GCP ADC auth — no client API key needed.
			// For Claude Code: ANTHROPIC_BASE_URL=http://localhost:8181/proxy/anthropic-vertex
			p.UpstreamURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com", region)
			p.PathRewriter = &proxy.VertexAIPathRewriter{
				ProjectID: project,
				Region:    region,
			}
		default:
			slog.Warn("unknown provider — skipping", "name", lp.Name)
			continue
		}

		providers = append(providers, p)
		for _, m := range lp.Models {
			cloudModels[m] = p.Name
		}
	}

	if len(providers) == 0 {
		return nil, nil
	}

	calc := costcalc.New()
	cloudProxy := proxy.New(proxy.Config{
		Providers: providers,
		ProjectID: "local",
	}, submitter, calc)

	var names []string
	for _, p := range providers {
		names = append(names, p.Name)
	}
	slog.Info("☁️ direct cloud providers enabled", "providers", names, "models", len(cloudModels))
	return cloudProxy, cloudModels
}
