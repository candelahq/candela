// Candela sidecar — minimal production LLM proxy for container environments.
//
// Routes LLM traffic through configurable providers with ADC credential
// injection and exports observability spans to Pub/Sub and/or OTLP sinks.
// No Firebase, no local storage, no UI — pure proxy + telemetry.
//
// Configuration is entirely via environment variables:
//
//	PORT               — HTTP listen port (default: 8080)
//	GCP_PROJECT        — GCP project for Vertex AI + Pub/Sub
//	VERTEX_REGION      — Vertex AI region (default: us-central1)
//	PROVIDERS          — comma-separated provider list (default: all)
//	CANDELA_PROJECT_ID — project ID for span tagging
//	PUBSUB_TOPIC       — Pub/Sub topic for span export (optional)
//	SPAN_FORMAT        — "proto" (default) or "json" for Pub/Sub messages
//	OTLP_ENDPOINT      — OTLP/HTTP endpoint for span export (optional)
//	OTLP_HEADERS       — comma-separated key=value OTLP auth headers
//	CORS_ORIGINS       — comma-separated allowed origins (default: *)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/oauth2/google"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/processor"
	"github.com/candelahq/candela/pkg/proxy"
	"github.com/candelahq/candela/pkg/storage"
	otlpexporter "github.com/candelahq/candela/pkg/storage/otlpexporter"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// ── Configuration ──
	port := envOr("PORT", "8080")
	gcpProject := os.Getenv("GCP_PROJECT")
	vertexRegion := envOr("VERTEX_REGION", "us-central1")
	providerFilter := os.Getenv("PROVIDERS")
	projectID := envOr("CANDELA_PROJECT_ID", gcpProject)
	pubsubTopic := os.Getenv("PUBSUB_TOPIC")
	spanFormat := envOr("SPAN_FORMAT", "proto")
	otlpEndpoint := os.Getenv("OTLP_ENDPOINT")
	otlpHeaders := os.Getenv("OTLP_HEADERS")
	corsOrigins := envOr("CORS_ORIGINS", "*")

	slog.Info("🕯️ candela-sidecar starting",
		"port", port,
		"gcp_project", gcpProject,
		"vertex_region", vertexRegion,
		"project_id", projectID,
		"pubsub_topic", pubsubTopic,
		"span_format", spanFormat,
		"otlp_endpoint", otlpEndpoint,
	)

	// ── Span writers (fan-out sinks) ──
	var writers []storage.SpanWriter
	var closers []func()

	// Pub/Sub sink.
	if pubsubTopic != "" {
		if gcpProject == "" {
			slog.Error("PUBSUB_TOPIC requires GCP_PROJECT")
			os.Exit(1)
		}
		psWriter, err := NewPubSubWriter(context.Background(), gcpProject, pubsubTopic, spanFormat)
		if err != nil {
			slog.Error("failed to initialize Pub/Sub writer", "error", err)
			os.Exit(1)
		}
		writers = append(writers, psWriter)
		closers = append(closers, func() { _ = psWriter.Close() })
		slog.Info("📡 Pub/Sub span sink enabled", "topic", pubsubTopic, "format", spanFormat)
	}

	// OTLP sink.
	if otlpEndpoint != "" {
		headers := parseHeaders(otlpHeaders)
		otlpW, err := otlpexporter.New(context.Background(), otlpexporter.Config{
			Endpoint:    otlpEndpoint,
			Headers:     headers,
			Compression: "gzip",
			TimeoutSec:  10,
		})
		if err != nil {
			slog.Error("failed to initialize OTLP exporter", "error", err)
			os.Exit(1)
		}
		writers = append(writers, otlpW)
		closers = append(closers, func() { _ = otlpW.Close() })
		slog.Info("📡 OTLP span sink enabled", "endpoint", otlpEndpoint)
	}

	if len(writers) == 0 {
		slog.Warn("⚠️  No span sinks configured (set PUBSUB_TOPIC and/or OTLP_ENDPOINT)")
	}

	// ── Cost calculator ──
	calc := costcalc.New()

	// ── Span processor ──
	proc := processor.New(writers, calc, 100)
	go proc.Run(context.Background())
	defer proc.Stop()

	// ── LLM proxy ──
	allProviders := proxy.DefaultProviders()

	// Attach ADC + Vertex AI path rewriting to Anthropic provider.
	if gcpProject != "" {
		tokenSource, err := google.DefaultTokenSource(context.Background(),
			"https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			slog.Warn("failed to get ADC — Vertex AI providers will require manual auth", "error", err)
		}

		for i, p := range allProviders {
			if p.Name == "anthropic" {
				allProviders[i].UpstreamURL = fmt.Sprintf(
					"https://%s-aiplatform.googleapis.com", vertexRegion)
				allProviders[i].FormatTranslator = &proxy.AnthropicFormatTranslator{}
				allProviders[i].PathRewriter = &proxy.VertexAIPathRewriter{
					ProjectID: gcpProject,
					Region:    vertexRegion,
				}
				if tokenSource != nil {
					allProviders[i].TokenSource = tokenSource
				}
				slog.Info("🔐 Anthropic via Vertex AI configured",
					"project", gcpProject, "region", vertexRegion)
				break
			}
		}
	}

	// Filter to requested providers.
	var activeProviders []proxy.Provider
	if providerFilter != "" {
		enabled := make(map[string]bool)
		for _, name := range strings.Split(providerFilter, ",") {
			enabled[strings.TrimSpace(name)] = true
		}
		for _, p := range allProviders {
			if enabled[p.Name] {
				activeProviders = append(activeProviders, p)
			}
		}
	} else {
		activeProviders = allProviders
	}

	if len(activeProviders) == 0 {
		slog.Error("no valid providers configured")
		os.Exit(1)
	}

	llmProxy := proxy.New(proxy.Config{
		Providers: activeProviders,
		ProjectID: projectID,
	}, proc, calc)

	// ── HTTP server ──
	mux := http.NewServeMux()

	// Health check.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status":"ok","binary":"candela-sidecar"}`)
	})

	// Readiness check (could be enhanced with sink connectivity checks).
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status":"ready"}`)
	})

	// Register proxy routes.
	llmProxy.RegisterRoutes(mux)

	var names []string
	for _, p := range activeProviders {
		names = append(names, "/proxy/"+p.Name+"/")
	}
	slog.Info("🔀 LLM proxy enabled", "routes", names)

	// Wrap with CORS.
	origins := strings.Split(corsOrigins, ",")
	handler := corsMiddleware(mux, origins)

	addr := ":" + port
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
	}

	// ── Graceful shutdown ──
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("🕯️ candela-sidecar listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Close sinks first (flush pending spans), then server.
	proc.Stop()
	for i := len(closers) - 1; i >= 0; i-- {
		closers[i]()
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("sidecar stopped")
}

// envOr returns the value of the environment variable or the default.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseHeaders parses "key1=val1,key2=val2" into a map.
func parseHeaders(s string) map[string]string {
	if s == "" {
		return nil
	}
	headers := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 {
			headers[kv[0]] = kv[1]
		}
	}
	return headers
}

// corsMiddleware wraps an http.Handler with permissive CORS for sidecar use.
func corsMiddleware(next http.Handler, origins []string) http.Handler {
	allowed := make(map[string]bool, len(origins))
	allowAll := false
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Traceparent, Tracestate, X-Api-Key, X-Request-ID, X-Session-Id")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
