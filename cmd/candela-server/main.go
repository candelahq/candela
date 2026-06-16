// Candela server — single-binary backend serving ConnectRPC (for the UI) and
// handling span ingestion. DuckDB by default for local dev, BigQuery
// for production.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "go.uber.org/automaxprocs" // automatically sets GOMAXPROCS from container CPU quota
	_ "time/tzdata"              // Embed timezone database for scratch/distroless containers

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/oauth2/google"
	"gopkg.in/yaml.v3"

	connect "connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/candelahq/candela/gen/go/candela/v1/candelav1connect"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/catalog"
	"github.com/candelahq/candela/pkg/connecthandlers"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/notify"
	"github.com/candelahq/candela/pkg/processor"
	"github.com/candelahq/candela/pkg/proxy"
	"github.com/candelahq/candela/pkg/proxy/spendoutbox"
	"github.com/candelahq/candela/pkg/storage"
	bqstore "github.com/candelahq/candela/pkg/storage/bigquery"
	duckdbstore "github.com/candelahq/candela/pkg/storage/duckdb"
	firestorestore "github.com/candelahq/candela/pkg/storage/firestoredb"
	otlpexporter "github.com/candelahq/candela/pkg/storage/otlpexporter"
	"github.com/candelahq/candela/pkg/storage/projectdb"
	sqlitestore "github.com/candelahq/candela/pkg/storage/sqlite"
)

// ProviderConfig defines a custom LLM provider added via YAML config.
// This allows adding new providers without code changes.
type ProviderConfig struct {
	Name        string `yaml:"name"`
	UpstreamURL string `yaml:"upstream_url"`
	AuthHeader  string `yaml:"auth_header"`  // e.g., "Authorization", "x-api-key"
	AuthEnvVar  string `yaml:"auth_env_var"` // env var for API key
	Enabled     *bool  `yaml:"enabled"`      // nil = default (enabled)
}

// Config holds the server configuration.
type Config struct {
	CustomProviders   []ProviderConfig `yaml:"custom_providers"`
	DisabledProviders []string         `yaml:"disabled_providers"`
	Server            struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	Storage struct {
		Backend string `yaml:"backend"`
		DuckDB  struct {
			Path string `yaml:"path"` // e.g. "candela.duckdb"
		} `yaml:"duckdb"`
		SQLite struct {
			Path string `yaml:"path"` // e.g. "candela.db" or ":memory:"
		} `yaml:"sqlite"`
		BigQuery struct {
			ProjectID string `yaml:"project_id"`
			Dataset   string `yaml:"dataset"`
			Table     string `yaml:"table"`
			Location  string `yaml:"location"`
		} `yaml:"bigquery"`
	} `yaml:"storage"`
	Proxy struct {
		Enabled        bool                     `yaml:"enabled"`
		ProjectID      string                   `yaml:"project_id"`
		Providers      []string                 `yaml:"providers"`            // e.g. ["openai", "google", "anthropic", "anthropic-direct", "gemini-oai"]
		MaxRequestCost float64                  `yaml:"max_request_cost_usd"` // Per-request cost cap (0 = disabled)
		DailyLimits    []proxy.SpendLimitConfig `yaml:"daily_limits"`         // Per-model daily spend limits
		Policy         *proxy.PolicyConfig      `yaml:"policy"`               // Model allowlist policy (#207)
		VertexAI       struct {
			ProjectID     string `yaml:"project_id"`     // GCP project for Vertex AI
			Region        string `yaml:"region"`         // default region (e.g. "us-central1")
			CachingMode   string `yaml:"caching_mode"`   // off|auto|system-only (default: auto)
			PromptCaching bool   `yaml:"prompt_caching"` // enable prompt caching (maps to CachingMode: auto)
			CacheTTL      string `yaml:"cache_ttl"`      // Vertex AI cache TTL ("5m" or "1h")
			// ProviderOverrides allows per-provider region and endpoint overrides.
			// MaaS models (Mistral, DeepSeek, Qwen) have limited regional availability;
			// this lets each provider target the correct region independently.
			ProviderOverrides map[string]ProviderOverride `yaml:"provider_overrides"`
		} `yaml:"vertex_ai"`
	} `yaml:"proxy"`
	Catalog CatalogConfig `yaml:"catalog"`
	CORS    struct {
		AllowedOrigins []string `yaml:"allowed_origins"` // e.g. ["http://localhost:3000"]
	} `yaml:"cors"`
	Worker struct {
		BatchSize     int    `yaml:"batch_size"`
		FlushInterval string `yaml:"flush_interval"`
	} `yaml:"worker"`
	Auth struct {
		DevMode                bool     `yaml:"dev_mode"`                 // If true, skip auth validation
		AllowedServiceAccounts []string `yaml:"allowed_service_accounts"` // SAs allowed to proxy — deny all if empty
	} `yaml:"auth"`
	Firestore struct {
		Enabled    bool   `yaml:"enabled"`
		ProjectID  string `yaml:"project_id"`
		DatabaseID string `yaml:"database_id"` // e.g. "candela" or "(default)"
	} `yaml:"firestore"`
	Pricing costcalc.PricingConfig `yaml:"pricing"`
	Users   struct {
		DefaultDailyBudgetUSD float64 `yaml:"default_daily_budget_usd"` // auto-assigned to new users (0 = no default)
	} `yaml:"users"`
	Sinks struct {
		OTLP struct {
			Enabled     bool              `yaml:"enabled"`
			Required    bool              `yaml:"required"`    // if true, fail startup on init error
			Endpoint    string            `yaml:"endpoint"`    // e.g. "http://localhost:4318"
			Protocol    string            `yaml:"protocol"`    // "http" (default) or "grpc"
			Headers     map[string]string `yaml:"headers"`     // optional auth headers
			Insecure    bool              `yaml:"insecure"`    // skip TLS verification
			Compression string            `yaml:"compression"` // "gzip" (default) or "none"
			TimeoutSec  int               `yaml:"timeout_sec"` // per-export timeout (default: 30)
		} `yaml:"otlp"`
	} `yaml:"sinks"`
	Budget struct {
		Timezone string `yaml:"timezone"` // IANA timezone for budget period reset (default: UTC)
	} `yaml:"budget"`
}

// CatalogConfig holds model catalog configuration.
type CatalogConfig struct {
	Backend   string                 `yaml:"backend"` // "config" (default), "firestore"
	Firestore FirestoreCatalogConfig `yaml:"firestore"`
}

// FirestoreCatalogConfig holds Firestore-specific catalog settings.
type FirestoreCatalogConfig struct {
	Collection string `yaml:"collection"`  // Firestore collection name (default: "model_catalog")
	ProjectID  string `yaml:"project_id"`  // GCP project (defaults to server's project_id)
	DatabaseID string `yaml:"database_id"` // Firestore database ID (default: "(default)")
}

// ProviderOverride holds per-provider Vertex AI configuration overrides.
type ProviderOverride struct {
	Region   string `yaml:"region"`   // Regional override (e.g. "us-central1" for Mistral)
	Endpoint string `yaml:"endpoint"` // Full endpoint URL override (e.g. "https://us-central1-aiplatform.googleapis.com")
}

// getProviderOverride extracts region and endpoint from provider overrides.
// Returns the defaultRegion if no override is set or region is empty.
func getProviderOverride(overrides map[string]ProviderOverride, provider, defaultRegion string) (region, endpoint string) {
	region = defaultRegion
	if ov, ok := overrides[provider]; ok {
		if ov.Region != "" {
			region = ov.Region
		}
		endpoint = ov.Endpoint
	}
	return
}

// buildCustomProviders converts ProviderConfig entries into proxy.Provider
// values, applying validation (skip empty name/upstream_url) and wiring
// AuthEnvVar / AuthHeader into the Provider's APIKey / AuthHeader fields.
// envLookup is typically os.Getenv but can be swapped for testing.
func buildCustomProviders(cfgs []ProviderConfig, envLookup func(string) string) []proxy.Provider {
	var out []proxy.Provider
	for _, cp := range cfgs {
		if cp.Enabled != nil && !*cp.Enabled {
			continue
		}
		if cp.Name == "" {
			slog.Warn("skipping custom provider with empty name")
			continue
		}
		if cp.UpstreamURL == "" {
			slog.Warn("skipping custom provider with empty upstream_url", "name", cp.Name)
			continue
		}
		p := proxy.Provider{
			Name:        cp.Name,
			UpstreamURL: cp.UpstreamURL,
		}
		if cp.AuthEnvVar != "" {
			if key := envLookup(cp.AuthEnvVar); key != "" {
				p.APIKey = key
				if cp.AuthHeader != "" {
					p.AuthHeader = cp.AuthHeader
				}
			}
		}
		out = append(out, p)
	}
	return out
}

func main() {
	// Set up structured logging to stderr.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize storage backend.
	reader, writers, closeFn, err := initStorage(cfg)
	if err != nil {
		slog.Error("failed to initialize storage", "error", err)
		os.Exit(1)
	}
	defer closeFn()
	slog.Info("storage initialized", "backend", cfg.Storage.Backend, "sinks", len(writers))

	// Initialize cost calculator with built-in defaults + config overrides.
	calc := costcalc.New()
	if cfg.Pricing.DiscountPercent > 0 || len(cfg.Pricing.Models) > 0 {
		calc.LoadFromConfig(cfg.Pricing)
	}

	// Initialize model catalog store.
	var catalogStore catalog.ModelCatalogStore
	var catalogClosers []func()
	switch cfg.Catalog.Backend {
	case "firestore":
		collection := cfg.Catalog.Firestore.Collection
		if collection == "" {
			collection = "model_catalog"
		}
		projectID := cfg.Catalog.Firestore.ProjectID
		if projectID == "" {
			projectID = cfg.Firestore.ProjectID
		}
		databaseID := cfg.Catalog.Firestore.DatabaseID
		if databaseID == "" {
			databaseID = cfg.Firestore.DatabaseID
		}
		var fsClient *firestore.Client
		var err error
		if databaseID != "" && databaseID != "(default)" {
			fsClient, err = firestore.NewClientWithDatabase(context.Background(), projectID, databaseID)
		} else {
			fsClient, err = firestore.NewClient(context.Background(), projectID)
		}
		if err != nil {
			slog.Error("failed to create Firestore client for catalog", "error", err)
			slog.Warn("falling back to config-based catalog")
			catalogStore = catalog.NewConfigStore(nil) // falls back to built-in defaults
		} else {
			catalogStore = catalog.NewFirestoreStore(fsClient, collection)
			catalogClosers = append(catalogClosers, func() { _ = fsClient.Close() })
		}
	case "config", "":
		catalogStore = catalog.NewConfigStore(nil) // uses Calculator's built-in defaults
	default:
		slog.Error("unknown catalog backend", "backend", cfg.Catalog.Backend)
		os.Exit(1)
	}
	defer func() {
		for _, fn := range catalogClosers {
			fn()
		}
	}()
	slog.Info("catalog store initialized", "backend", catalogStore.Source(), "writable", catalogStore.Writable())

	// If the catalog is backed by a real store (not config), load entries into Calculator.
	if catalogStore.Source() != "config" {
		ctx := context.Background()
		entries, err := catalogStore.List(ctx, false) // only enabled models needed for pricing
		if err != nil {
			slog.Error("failed to load catalog entries", "error", err)
			slog.Warn("using built-in default pricing (catalog unavailable)")
		} else {
			calc.LoadFromCatalog(entries)
		}
	}

	// Start the in-process span processor (fan-out to all writers).
	proc := processor.New(writers, calc, cfg.Worker.BatchSize)
	go proc.Run(context.Background())
	defer proc.Stop()

	// Build the HTTP mux for ConnectRPC handlers.
	mux := http.NewServeMux()

	// Liveness probe: returns 200 if the process is alive.
	// No external dependency checks — a failing DB should not cause restarts.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status": "ok"}`)
	})

	// Readiness probe: checks that the server can serve traffic.
	// Used by Cloud Run / Kubernetes for traffic routing decisions.
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := reader.Ping(r.Context()); err != nil {
			slog.Error("readyz: storage ping failed", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintln(w, `{"status": "not_ready", "reason": "storage_unavailable"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status": "ready"}`)
	})

	// Catalog health check: returns backend, model count, latency, writable flag.
	// 503 when catalog is unhealthy (e.g. Firestore unavailable).
	mux.HandleFunc("/catalog/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if catalogStore == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(catalog.HealthStatus{Error: "catalog store is nil"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		status := catalog.CheckHealth(ctx, catalogStore)

		if !status.Healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(status)
	})

	var spendOB *spendoutbox.Outbox
	var llmProxy *proxy.Proxy

	// HIGH-5: Debug metrics endpoint (JSON, no Prometheus dependency).
	mux.HandleFunc("/debug/metrics", func(w http.ResponseWriter, r *http.Request) {
		type sinkMetric struct {
			Name          string `json:"name"`
			State         string `json:"state"`
			TotalWrites   int64  `json:"total_writes"`
			TotalFailures int64  `json:"total_failures"`
			TotalDropped  int64  `json:"total_dropped"`
		}
		type metrics struct {
			Proxy struct {
				DroppedSpans       int64   `json:"dropped_spans"`
				SASpendUSD         float64 `json:"sa_spend_usd"`
				SpendOutboxPending int64   `json:"spend_outbox_pending"`
			} `json:"proxy"`
			Processor struct {
				DroppedSpans int64        `json:"dropped_spans"`
				Sinks        []sinkMetric `json:"sinks"`
			} `json:"processor"`
		}
		var m metrics
		if llmProxy != nil {
			m.Proxy.DroppedSpans = llmProxy.DroppedSpans()
			m.Proxy.SASpendUSD = float64(llmProxy.SASpendMicroUSD()) / 1_000_000
		}
		if spendOB != nil {
			if pending, err := spendOB.Pending(r.Context()); err == nil {
				m.Proxy.SpendOutboxPending = pending
			}
		}
		m.Processor.DroppedSpans = proc.DroppedSpans()
		for _, sh := range proc.SinkHealth() {
			m.Processor.Sinks = append(m.Processor.Sinks, sinkMetric{
				Name:          sh.Name,
				State:         sh.State,
				TotalWrites:   sh.TotalWrites,
				TotalFailures: sh.TotalFailures,
				TotalDropped:  sh.TotalDropped,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	})

	// Register ConnectRPC service handlers.

	// Initialize Firestore-backed UserStore (if enabled).
	// Needed by trace/dashboard handlers for user-scoped access control,
	// and by UserService for user management.
	var userStore storage.UserStore
	if cfg.Firestore.Enabled {
		fStore, err := firestorestore.New(context.Background(),
			cfg.Firestore.ProjectID, cfg.Firestore.DatabaseID)
		if err != nil {
			slog.Error("failed to initialize Firestore", "error", err)
			os.Exit(1)
		}

		// Apply budget timezone if configured (#136).
		if cfg.Budget.Timezone != "" {
			loc, err := time.LoadLocation(cfg.Budget.Timezone)
			if err != nil {
				slog.Error("invalid budget timezone", "timezone", cfg.Budget.Timezone, "error", err)
				os.Exit(1)
			}
			fStore.SetBudgetLocation(loc)
			slog.Info("budget timezone configured", "timezone", cfg.Budget.Timezone)
		}

		defer func() { _ = fStore.Close() }()
		userStore = fStore

		// Create protovalidate interceptor (validates request fields before handler).
		validateInterceptor := validate.NewInterceptor()

		userPath, userH := candelav1connect.NewUserServiceHandler(
			connecthandlers.NewUserHandler(fStore, cfg.Users.DefaultDailyBudgetUSD),
			connect.WithInterceptors(validateInterceptor, auth.AdminInterceptor(fStore)))
		mux.Handle(userPath, userH)
		slog.Info("UserService registered", "path", userPath,
			"admin_guard", true, "validation", true,
			"default_daily_budget", cfg.Users.DefaultDailyBudgetUSD)
	} else {
		slog.Info("Firestore disabled — UserService not available, all users see all traces")
	}

	tracePath, traceH := candelav1connect.NewTraceServiceHandler(
		connecthandlers.NewTraceHandler(reader, userStore))
	mux.Handle(tracePath, traceH)

	ingestionPath, ingestionH := candelav1connect.NewIngestionServiceHandler(
		connecthandlers.NewIngestionHandlerDirect(proc))
	mux.Handle(ingestionPath, ingestionH)

	dashboardPath, dashboardH := candelav1connect.NewDashboardServiceHandler(
		connecthandlers.NewDashboardHandler(reader, userStore))
	mux.Handle(dashboardPath, dashboardH)

	// Initialize project store (separate SQLite DB for relational metadata).
	projectStore, err := projectdb.New("candela-projects.db")
	if err != nil {
		slog.Error("failed to initialize project store", "error", err)
		os.Exit(1)
	}
	defer func() { _ = projectStore.Close() }()

	projectPath, projectH := candelav1connect.NewProjectServiceHandler(
		connecthandlers.NewProjectHandler(projectStore))
	mux.Handle(projectPath, projectH)

	catalogValidateInterceptor := validate.NewInterceptor()
	catalogPath, catalogH := candelav1connect.NewModelCatalogServiceHandler(
		connecthandlers.NewCatalogHandler(catalogStore, userStore),
		connect.WithInterceptors(catalogValidateInterceptor))
	mux.Handle(catalogPath, catalogH)

	slog.Info("ConnectRPC services registered",
		"trace", tracePath,
		"ingestion", ingestionPath,
		"dashboard", dashboardPath,
		"project", projectPath,
		"catalog", catalogPath)

	// Register LLM proxy routes (selective activation).
	if cfg.Proxy.Enabled {
		allProviders := proxy.DefaultProviders()

		// Remove disabled providers (from config YAML).
		if len(cfg.DisabledProviders) > 0 {
			disabled := make(map[string]bool, len(cfg.DisabledProviders))
			for _, name := range cfg.DisabledProviders {
				disabled[strings.ToLower(name)] = true
			}
			filtered := allProviders[:0]
			for _, p := range allProviders {
				if !disabled[strings.ToLower(p.Name)] {
					filtered = append(filtered, p)
				}
			}
			allProviders = filtered
			slog.Info("disabled providers from config", "count", len(cfg.DisabledProviders), "names", cfg.DisabledProviders)
		}

		// Add custom providers (from config YAML).
		customProviders := buildCustomProviders(cfg.CustomProviders, os.Getenv)
		for _, p := range customProviders {
			slog.Info("added custom provider", "name", p.Name, "upstream", p.UpstreamURL,
				"has_api_key", p.APIKey != "", "auth_header", p.AuthHeader)
		}
		allProviders = append(allProviders, customProviders...)

		// Get ADC token source for automatic GCP auth.
		// Used by Anthropic (Vertex AI), Gemini, and Google providers so the
		// Cloud Run service account authenticates to upstream APIs on behalf
		// of users — individual user tokens never reach the upstream provider.
		tokenSource, adcErr := google.DefaultTokenSource(context.Background(),
			"https://www.googleapis.com/auth/cloud-platform")
		if adcErr != nil {
			slog.Warn("failed to get GCP ADC — GCP-backed providers will require manual auth",
				"error", adcErr)
		}

		// NOTE: Gemini/Google providers are configured below to use the Vertex AI
		// global endpoint. This avoids regional model availability issues
		// and natively accepts ADC OAuth2 tokens.

		// Attach FormatTranslator + PathRewriter + ADC to Anthropic providers
		// when Vertex AI is configured. Anthropic routes through regional
		// Vertex AI publisher endpoints (rawPredict/streamRawPredict).
		if cfg.Proxy.VertexAI.ProjectID != "" {
			region := cfg.Proxy.VertexAI.Region
			if region == "" {
				region = "us-central1"
			}

			for i, p := range allProviders {
				switch p.Name {
				case "anthropic", "anthropic-vertex":
					allProviders[i].UpstreamURL = proxy.VertexAIUpstreamURL(region)
					allProviders[i].PathRewriter = &proxy.VertexAIPathRewriter{
						ProjectID: cfg.Proxy.VertexAI.ProjectID,
						Region:    region,
						ModelResolver: func(model string) (string, string) {
							entry, err := catalogStore.Get(context.Background(), "anthropic", model)
							if err != nil || entry == nil {
								return "", ""
							}
							return entry.ProviderModelID, entry.Region
						},
					}
					if tokenSource != nil {
						allProviders[i].TokenSource = tokenSource
					}
					// Only add format translation for the OpenAI-compat "anthropic" provider.
					// anthropic-vertex is a native Messages API passthrough (for Claude Code).
					if p.Name == "anthropic" {
						ft := &proxy.AnthropicFormatTranslator{}
						if cfg.Proxy.VertexAI.CachingMode != "" {
							ft.SetCachingMode(proxy.ParseCachingMode(cfg.Proxy.VertexAI.CachingMode))
						}
						// prompt_caching: true is a shorthand for caching_mode: auto
						if cfg.Proxy.VertexAI.CachingMode == "" && cfg.Proxy.VertexAI.PromptCaching {
							ft.SetCachingMode(proxy.CachingAuto)
						}
						if cfg.Proxy.VertexAI.CacheTTL != "" {
							ft.SetCacheTTL(proxy.ParseCacheTTL(cfg.Proxy.VertexAI.CacheTTL))
						}
						allProviders[i].FormatTranslator = ft
					}
					slog.Info("🔐 Anthropic via Vertex AI configured",
						"provider", p.Name,
						"project", cfg.Proxy.VertexAI.ProjectID,
						"region", region,
						"adc", tokenSource != nil,
						"format_translation", p.Name == "anthropic",
						"caching_mode", cfg.Proxy.VertexAI.CachingMode)
				}
			}
		}

		// Configure Gemini providers — these route through Vertex AI's
		// global OpenAI-compatible endpoint (aiplatform.googleapis.com with
		// locations/global). This accepts ADC OAuth2 tokens and avoids
		// regional model availability issues.
		// Requires a ProjectID for the endpoint path.
		geminiProjectID := cfg.Proxy.VertexAI.ProjectID
		if geminiProjectID != "" {
			for i, p := range allProviders {
				switch p.Name {
				case "gemini-oai":
					// OpenAI-compatible endpoint via Vertex AI global.
					// Path: /v1/projects/{project}/locations/global/endpoints/openapi/chat/completions
					// Model names require "google/" prefix (injected by handleProxy).
					allProviders[i].UpstreamURL = proxy.VertexAIUpstreamURL("global")
					allProviders[i].PathRewriter = &proxy.VertexAIGeminiOAIPathRewriter{
						ProjectID: geminiProjectID,
						Region:    "global",
					}
					if tokenSource != nil {
						allProviders[i].TokenSource = tokenSource
					}
					slog.Info("🔐 Gemini-OAI via Vertex AI global endpoint configured",
						"provider", p.Name,
						"project", geminiProjectID,
						"adc", tokenSource != nil)

				case "google":
					// Native Gemini endpoint via Vertex AI global.
					allProviders[i].UpstreamURL = proxy.VertexAIUpstreamURL("global")
					allProviders[i].PathRewriter = &proxy.VertexAIGooglePathRewriter{
						ProjectID: geminiProjectID,
						Region:    "global",
					}
					if tokenSource != nil {
						allProviders[i].TokenSource = tokenSource
					}
					slog.Info("🔐 Google native via Vertex AI global endpoint configured",
						"provider", p.Name,
						"project", geminiProjectID,
						"adc", tokenSource != nil)

				case "gemini-vertex":
					// Native Gemini API via Vertex AI publisher endpoint.
					// Same as "google" but with an explicit name for clarity.
					allProviders[i].UpstreamURL = proxy.VertexAIUpstreamURL("global")
					allProviders[i].PathRewriter = &proxy.VertexAIGooglePathRewriter{
						ProjectID: geminiProjectID,
						Region:    "global",
					}
					if tokenSource != nil {
						allProviders[i].TokenSource = tokenSource
					}
					slog.Info("🔐 Gemini-Vertex native via Vertex AI configured",
						"provider", p.Name,
						"project", geminiProjectID,
						"adc", tokenSource != nil)
				}
			}
		} else {
			// Without a project ID, Vertex AI providers can't route.
			// Remove them from allProviders to prevent broken defaults.
			slog.Warn("⚠️ Vertex AI providers require vertex_ai.project_id — gemini-oai, google, gemini-vertex, mistral, deepseek, deepseek-v3, qwen providers will be disabled")
			var filtered []proxy.Provider
			for _, p := range allProviders {
				switch p.Name {
				case "gemini-oai", "google", "gemini-vertex", "mistral", "deepseek", "deepseek-v3", "qwen":
					// Skip — these need Vertex AI project ID
				default:
					filtered = append(filtered, p)
				}
			}
			allProviders = filtered
		}

		// Configure Mistral — routes through regional Vertex AI rawPredict
		// endpoint with publisher "mistralai". Same auth pattern as Anthropic.
		// Default to us-central1 since Mistral is only available in limited regions.
		if cfg.Proxy.VertexAI.ProjectID != "" {
			mistralRegion, mistralEndpoint := getProviderOverride(cfg.Proxy.VertexAI.ProviderOverrides, "mistral", "us-central1")
			for i, p := range allProviders {
				switch p.Name {
				case "mistral":
					if mistralEndpoint != "" {
						allProviders[i].UpstreamURL = mistralEndpoint
					} else {
						allProviders[i].UpstreamURL = proxy.VertexAIUpstreamURL(mistralRegion)
					}
					allProviders[i].PathRewriter = &proxy.VertexAIMaaSPathRewriter{
						ProjectID: cfg.Proxy.VertexAI.ProjectID,
						Region:    mistralRegion,
						Publisher: "mistralai",
					}
					if tokenSource != nil {
						allProviders[i].TokenSource = tokenSource
					}
					slog.Info("🔐 Mistral via Vertex AI configured",
						"provider", p.Name,
						"project", cfg.Proxy.VertexAI.ProjectID,
						"region", mistralRegion,
						"endpoint_override", mistralEndpoint != "",
						"adc", tokenSource != nil)
				}
			}
		}

		// Configure DeepSeek & Qwen — route through Vertex AI's
		// OpenAI-compatible endpoint (same pattern as gemini-oai).
		// DeepSeek R1 requires us-central1; V3.2 is global-only
		// (override via provider_overrides if using V3.2).
		// Qwen MaaS models (235B, Coder 480B) require us-south1.
		// Supports per-provider region/endpoint overrides.
		if geminiProjectID != "" {
			for i, p := range allProviders {
				var defaultRegion string
				switch p.Name {
				case "deepseek":
					defaultRegion = "us-central1"
				case "deepseek-v3":
					defaultRegion = "global"
				case "qwen":
					defaultRegion = "us-south1"
				default:
					continue
				}
				provRegion, provEndpoint := getProviderOverride(cfg.Proxy.VertexAI.ProviderOverrides, p.Name, defaultRegion)
				if provEndpoint != "" {
					allProviders[i].UpstreamURL = provEndpoint
				} else {
					allProviders[i].UpstreamURL = proxy.VertexAIUpstreamURL(provRegion)
				}
				allProviders[i].PathRewriter = &proxy.VertexAIGeminiOAIPathRewriter{
					ProjectID: geminiProjectID,
					Region:    provRegion,
				}
				if tokenSource != nil {
					allProviders[i].TokenSource = tokenSource
				}
				slog.Info("🔐 "+p.Name+" via Vertex AI configured",
					"provider", p.Name,
					"project", geminiProjectID,
					"region", provRegion,
					"endpoint_override", provEndpoint != "",
					"adc", tokenSource != nil)
			}
		}

		// Validate provider_overrides keys — warn on unknown providers.
		if len(cfg.Proxy.VertexAI.ProviderOverrides) > 0 {
			validOverrideProviders := map[string]bool{
				"mistral": true, "deepseek": true, "deepseek-v3": true, "qwen": true,
				"anthropic": true, "anthropic-vertex": true,
				"gemini-oai": true, "gemini-vertex": true, "google": true,
			}
			for name := range cfg.Proxy.VertexAI.ProviderOverrides {
				if !validOverrideProviders[name] {
					slog.Warn("⚠️ unknown provider in provider_overrides — will be ignored",
						"provider", name)
				}
			}
		}

		var activeProviders []proxy.Provider

		if len(cfg.Proxy.Providers) > 0 {
			// Filter to only the configured providers.
			enabled := make(map[string]bool)
			for _, name := range cfg.Proxy.Providers {
				enabled[name] = true
			}
			for _, p := range allProviders {
				if enabled[p.Name] {
					activeProviders = append(activeProviders, p)
				}
			}
		} else {
			// No filter — enable all providers.
			activeProviders = allProviders
		}

		if len(activeProviders) > 0 {
			llmProxy, err = proxy.New(proxy.Config{
				Providers:      activeProviders,
				ProjectID:      cfg.Proxy.ProjectID,
				MaxRequestCost: cfg.Proxy.MaxRequestCost,
				DailyLimits:    cfg.Proxy.DailyLimits,
				Policy:         cfg.Proxy.Policy,
			}, proc, calc)
			if err != nil {
				slog.Error("invalid proxy configuration", "error", err)
				os.Exit(1)
			}

			// Wire team-mode features if Firestore is available.
			if userStore != nil {
				llmProxy.SetUserStore(userStore)
				llmProxy.SetBudgetChecker(notify.NewBudgetChecker(&notify.LogNotifier{}))

				// CRIT-3: Wire durable spend outbox for DeductSpend retries.
				// Prefer $HOME/.candela/ for local dev; fall back to /etc/candela/
				// in containers where the non-root user has no real home directory.
				obDir := ""
				if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" {
					obDir = filepath.Join(home, ".candela")
				} else {
					obDir = "/etc/candela" // Created and chown'd in Dockerfile
				}
				obPath := filepath.Join(obDir, "spend-outbox.db")
				obErr := os.MkdirAll(filepath.Dir(obPath), 0o700)
				if obErr != nil && obDir != "/etc/candela" {
					// $HOME/.candela/ not writable (common in non-root containers
					// where $HOME is still /root) — fall back to /etc/candela/.
					slog.Warn("failed to create spend outbox in home dir, falling back to /etc/candela",
						"path", filepath.Dir(obPath), "error", obErr)
					obDir = "/etc/candela"
					obPath = filepath.Join(obDir, "spend-outbox.db")
					obErr = os.MkdirAll(filepath.Dir(obPath), 0o700)
				}
				if obErr != nil {
					slog.Warn("spend outbox disabled: failed to create directory", "path", filepath.Dir(obPath), "error", obErr)
				} else {
					ob, obErr := spendoutbox.New(obPath)
					if obErr != nil {
						slog.Warn("spend outbox unavailable — failed DeductSpend calls will not be retried",
							"error", obErr)
					} else {
						spendOB = ob
						llmProxy.SetSpendOutbox(ob)
						// Start background retry worker.
						spendWorker := spendoutbox.NewSpendSyncWorker(ob, userStore, 10*time.Second)
						spendWorker.Start()
						// IMPORTANT: defer order is LIFO — close DB AFTER stopping worker.
						defer func() { _ = ob.Close() }()
						defer spendWorker.Stop()
						slog.Info("💾 Spend outbox + retry worker enabled", "path", obPath)
					}
				}

				slog.Info("🔔 Budget deduction + notifications wired into proxy")
			}

			llmProxy.RegisterRoutes(mux)

			// Build compat model list from the cost calculator's pricing table.
			// Map pricing provider names → OpenAI-compatible proxy route names.
			// Only include models whose compat provider is active.
			pricingToCompat := map[string]string{
				"google":      "gemini-oai",  // Gemini via OpenAI-compat endpoint
				"anthropic":   "anthropic",   // Anthropic with FormatTranslator (OpenAI→Messages)
				"openai":      "openai",      // Native OpenAI passthrough
				"mistral":     "mistral",     // Mistral via Vertex AI rawPredict
				"deepseek":    "deepseek",    // DeepSeek R1 via Vertex AI OpenAI-compat
				"deepseek-v3": "deepseek-v3", // DeepSeek V3 via Vertex AI OpenAI-compat
				"qwen":        "qwen",        // Qwen via Vertex AI OpenAI-compat
			}

			// Build set of active provider names for quick lookup.
			activeSet := make(map[string]bool, len(activeProviders))
			for _, p := range activeProviders {
				activeSet[p.Name] = true
			}

			// Deduplicate models: the pricing table may have both short names
			// (e.g. "claude-sonnet-4") and dated variants ("claude-sonnet-4-20250514")
			// that resolve to the same underlying model. Keep all of them — the
			// compat layer's prefix-based alias resolution handles short→long mapping.
			buildCompatModels := func() []proxy.CompatModel {
				seen := make(map[string]bool)
				var models []proxy.CompatModel
				for _, m := range calc.Models() {
					compatProvider, ok := pricingToCompat[m.Provider]
					if !ok || !activeSet[compatProvider] {
						continue
					}
					key := compatProvider + "/" + m.Model
					if seen[key] {
						continue
					}
					seen[key] = true
					models = append(models, proxy.CompatModel{
						ID:       m.Model,
						Provider: compatProvider,
					})
				}
				return models
			}

			compatModels := buildCompatModels()

			if len(compatModels) > 0 {
				llmProxy.RegisterCompatRoutes(mux, compatModels)
				slog.Info("📋 /v1/models endpoint registered", "models", len(compatModels))
			}

			// Periodic catalog refresh: when backed by Firestore (or any
			// non-config store), reload the catalog every 60s and push
			// updated models to the proxy.  This lets admins add/remove
			// models in Firestore without restarting the server.
			if catalogStore != nil && catalogStore.Source() != "config" {
				catalogDone := make(chan struct{})
				go func() {
					ticker := time.NewTicker(60 * time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-ticker.C:
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							entries, err := catalogStore.List(ctx, false)
							cancel()
							if err != nil {
								slog.Warn("catalog refresh failed", "error", err)
								continue
							}
							calc.LoadFromCatalog(entries)
							refreshed := buildCompatModels()
							llmProxy.RefreshModels(refreshed)
						case <-catalogDone:
							return
						}
					}
				}()
				// Ensure the goroutine is stopped when the server exits.
				defer close(catalogDone)
				slog.Info("🔄 catalog refresh goroutine started", "interval", "60s")
			}

			var names []string
			for _, p := range activeProviders {
				names = append(names, "/proxy/"+p.Name+"/")
			}
			slog.Info("🔀 LLM proxy enabled", "routes", names)

		} else {
			slog.Warn("proxy enabled but no valid providers configured")
		}
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	// Wrap the mux with Firebase Auth middleware.
	devMode := cfg.Auth.DevMode
	// Cloud Run service URL is the audience for Google ID tokens (candela-local).
	cloudRunURL := os.Getenv("CLOUD_RUN_URL")

	// Guard: never allow dev mode on Cloud Run. Fail-closed.
	if err := validateAuthConfig(devMode, os.Getenv("K_SERVICE"), cloudRunURL); err != nil {
		slog.Error("FATAL: " + err.Error())
		os.Exit(1)
	}

	// Initialize Firebase Admin SDK for token verification.
	var fbAuthClient *fbauth.Client
	if !devMode {
		fbApp, err := firebase.NewApp(context.Background(), nil)
		if err != nil {
			slog.Error("failed to initialize Firebase Admin SDK", "error", err)
			os.Exit(1)
		}
		fbAuthClient, err = fbApp.Auth(context.Background())
		if err != nil {
			slog.Error("failed to get Firebase Auth client", "error", err)
			os.Exit(1)
		}
		slog.Info("🔐 Firebase Auth initialized")
	}

	// Build a UserAuthorizer from the Firestore UserStore (if available).
	// This restricts access to only registered users.
	var userAuth auth.UserAuthorizer
	if userStore != nil {
		userAuth = func(ctx context.Context, email string) error {
			_, err := userStore.GetUserByEmail(ctx, email)
			if err != nil {
				// Distinguish "not found" from transient errors so the
				// middleware can return 403 vs 500 appropriately.
				if errors.Is(err, storage.ErrNotFound) {
					return fmt.Errorf("%w: %s", auth.ErrNotRegistered, email)
				}
				return err // transient — will trigger 500
			}
			return nil
		}
	}

	authedMux := auth.FirebaseAuthMiddleware(
		corsMiddleware(mux, cfg.CORS.AllowedOrigins),
		fbAuthClient,
		cloudRunURL,
		userAuth,
		devMode,
		cfg.Auth.AllowedServiceAccounts,
	)
	if devMode {
		slog.Info("🔓 Running in dev mode — auth disabled")
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           h2c.NewHandler(authedMux, &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Minute, // generous for streaming LLM responses
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("🕯️ Candela server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("server stopped")
}

// initStorage creates the storage backend and returns a reader (for queries)
// and a slice of writers (for the processor fan-out). The closeFn handles cleanup.
func initStorage(cfg *Config) (storage.SpanReader, []storage.SpanWriter, func(), error) {
	var reader storage.SpanReader
	var writers []storage.SpanWriter
	var closers []func()

	switch cfg.Storage.Backend {
	case "duckdb":
		store, err := duckdbstore.New(duckdbstore.Config{
			Path: cfg.Storage.DuckDB.Path,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		reader = store
		writers = append(writers, store)
		closers = append(closers, func() { _ = store.Close() })
	case "sqlite":
		store, err := sqlitestore.New(sqlitestore.Config{
			Path: cfg.Storage.SQLite.Path,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		reader = store
		writers = append(writers, store)
		closers = append(closers, func() { _ = store.Close() })
	case "bigquery":
		store, err := bqstore.New(context.Background(), bqstore.Config{
			ProjectID: cfg.Storage.BigQuery.ProjectID,
			Dataset:   cfg.Storage.BigQuery.Dataset,
			Table:     cfg.Storage.BigQuery.Table,
			Location:  cfg.Storage.BigQuery.Location,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		reader = store
		writers = append(writers, store)
		closers = append(closers, func() { _ = store.Close() })
	default:
		return nil, nil, nil, fmt.Errorf("unknown storage backend: %s", cfg.Storage.Backend)
	}

	// Append optional OTLP export sink.
	if cfg.Sinks.OTLP.Enabled {
		otlpW, err := otlpexporter.New(context.Background(), otlpexporter.Config{
			Endpoint:    cfg.Sinks.OTLP.Endpoint,
			Protocol:    cfg.Sinks.OTLP.Protocol,
			Headers:     cfg.Sinks.OTLP.Headers,
			Insecure:    cfg.Sinks.OTLP.Insecure,
			Compression: cfg.Sinks.OTLP.Compression,
			TimeoutSec:  cfg.Sinks.OTLP.TimeoutSec,
		})
		if err != nil {
			if cfg.Sinks.OTLP.Required {
				return nil, nil, nil, fmt.Errorf("otlp exporter required but failed to initialize: %w", err)
			}
			slog.Error("failed to initialize OTLP exporter (non-fatal)", "error", err)
		} else {
			writers = append(writers, otlpW)
			closers = append(closers, func() { _ = otlpW.Close() })
			slog.Info("📡 OTLP export sink added to fan-out")
		}
	}

	closeFn := func() {
		// Reverse order: close sinks before primary storage.
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	return reader, writers, closeFn, nil
}

func loadConfig() (*Config, error) {
	cfgPath := os.Getenv("CANDELA_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		// No config file — use defaults (DuckDB, port 8181).
		slog.Warn("config file not found, using defaults", "path", cfgPath)
		cfg := &Config{}
		cfg.Server.Port = 8181
		cfg.Storage.Backend = "duckdb"
		return cfg, nil
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8181
	}
	if cfg.Storage.Backend == "" {
		cfg.Storage.Backend = "duckdb"
	}

	// Catalog backend: env var override, then default to "config".
	if env := os.Getenv("CANDELA_CATALOG_BACKEND"); env != "" {
		cfg.Catalog.Backend = env
	}
	if cfg.Catalog.Backend == "" {
		cfg.Catalog.Backend = "config"
	}

	return &cfg, nil
}

// corsMiddleware wraps an http.Handler with CORS headers.
// Origins are configurable; defaults to localhost dev servers if none specified.
func corsMiddleware(next http.Handler, origins []string) http.Handler {
	// Build allowed set. Default to localhost dev servers.
	if len(origins) == 0 {
		origins = []string{"http://localhost:3000", "http://localhost:8080"}
	}
	allowed := make(map[string]bool, len(origins))
	allowAll := false
	for _, o := range origins {
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
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms, Traceparent, Tracestate, X-Request-ID, X-Session-Id, X-Candela-Tenant-Id, X-Candela-Job-Id")
		w.Header().Set("Access-Control-Expose-Headers", "Connect-Content-Encoding")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Handle preflight.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// validateAuthConfig checks that the auth configuration is safe for the runtime environment.
// Returns an error if the configuration would be insecure.
func validateAuthConfig(devMode bool, kService, cloudRunURL string) error {
	if devMode && kService != "" {
		return fmt.Errorf("auth.dev_mode=true is not allowed on Cloud Run (K_SERVICE=%s)", kService)
	}
	if kService != "" && cloudRunURL == "" {
		slog.Warn("CLOUD_RUN_URL not set on Cloud Run — Google ID token validation (Strategy 2) will be skipped; tokens will fall through to OAuth2 userinfo (slower)")
	}
	return nil
}
