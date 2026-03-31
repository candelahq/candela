// Candela server — single-binary backend serving ConnectRPC (for the UI) and
// handling span ingestion. DuckDB by default for local dev, BigQuery
// for production.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"gopkg.in/yaml.v3"
	"log/slog"

	"github.com/candelahq/candela/gen/go/candela/v1/candelav1connect"
	"github.com/candelahq/candela/pkg/connecthandlers"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/proxy"
	"github.com/candelahq/candela/pkg/storage"
	bqstore "github.com/candelahq/candela/pkg/storage/bigquery"
	duckdbstore "github.com/candelahq/candela/pkg/storage/duckdb"
	"github.com/candelahq/candela/pkg/storage/projectdb"
	sqlitestore "github.com/candelahq/candela/pkg/storage/sqlite"
)

// Config holds the server configuration.
type Config struct {
	Server struct {
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
		Enabled   bool     `yaml:"enabled"`
		ProjectID string   `yaml:"project_id"`
		Providers []string `yaml:"providers"` // e.g. ["openai", "google", "anthropic"]
	} `yaml:"proxy"`
	CORS struct {
		AllowedOrigins []string `yaml:"allowed_origins"` // e.g. ["http://localhost:3000"]
	} `yaml:"cors"`
	Worker struct {
		BatchSize     int    `yaml:"batch_size"`
		FlushInterval string `yaml:"flush_interval"`
	} `yaml:"worker"`
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

	// Initialize cost calculator.
	calc := costcalc.New()

	// Start the in-process span processor (fan-out to all writers).
	processor := NewSpanProcessor(writers, calc, cfg.Worker.BatchSize)
	go processor.Run(context.Background())
	defer processor.Stop()

	// Build the HTTP mux for ConnectRPC handlers.
	mux := http.NewServeMux()

	// Health check.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := reader.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status": "error", "detail": %q}`, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status": "ok"}`)
	})

	// Register ConnectRPC service handlers.
	tracePath, traceH := candelav1connect.NewTraceServiceHandler(
		connecthandlers.NewTraceHandler(reader))
	mux.Handle(tracePath, traceH)

	ingestionPath, ingestionH := candelav1connect.NewIngestionServiceHandler(
		connecthandlers.NewIngestionHandlerDirect(processor))
	mux.Handle(ingestionPath, ingestionH)

	dashboardPath, dashboardH := candelav1connect.NewDashboardServiceHandler(
		connecthandlers.NewDashboardHandler(reader))
	mux.Handle(dashboardPath, dashboardH)

	// Initialize project store (separate SQLite DB for relational metadata).
	projectStore, err := projectdb.New("candela-projects.db")
	if err != nil {
		slog.Error("failed to initialize project store", "error", err)
		os.Exit(1)
	}
	defer projectStore.Close()

	projectPath, projectH := candelav1connect.NewProjectServiceHandler(
		connecthandlers.NewProjectHandler(projectStore))
	mux.Handle(projectPath, projectH)

	slog.Info("ConnectRPC services registered",
		"trace", tracePath,
		"ingestion", ingestionPath,
		"dashboard", dashboardPath,
		"project", projectPath)

	// Register LLM proxy routes (selective activation).
	if cfg.Proxy.Enabled {
		allProviders := proxy.DefaultProviders()
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
			llmProxy := proxy.New(proxy.Config{
				Providers: activeProviders,
				ProjectID: cfg.Proxy.ProjectID,
			}, processor, calc)
			llmProxy.RegisterRoutes(mux)

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
	srv := &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(corsMiddleware(mux, cfg.CORS.AllowedOrigins), &http2.Server{}),
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

	processor.Stop()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("server stopped")
}

// initStorage creates the storage backend and returns a reader (for queries)
// and a slice of writers (for the processor fan-out). The closeFn handles cleanup.
func initStorage(cfg *Config) (storage.SpanReader, []storage.SpanWriter, func(), error) {
	switch cfg.Storage.Backend {
	case "duckdb":
		store, err := duckdbstore.New(duckdbstore.Config{
			Path: cfg.Storage.DuckDB.Path,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		// DuckDB implements both SpanReader and SpanWriter.
		return store, []storage.SpanWriter{store}, func() { store.Close() }, nil
	case "sqlite":
		store, err := sqlitestore.New(sqlitestore.Config{
			Path: cfg.Storage.SQLite.Path,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		return store, []storage.SpanWriter{store}, func() { store.Close() }, nil
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
		// BigQuery implements both SpanReader and SpanWriter.
		return store, []storage.SpanWriter{store}, func() { store.Close() }, nil
	default:
		return nil, nil, nil, fmt.Errorf("unknown storage backend: %s", cfg.Storage.Backend)
	}
}

func loadConfig() (*Config, error) {
	cfgPath := os.Getenv("CANDELA_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		// No config file — use defaults (DuckDB, port 8080).
		slog.Warn("config file not found, using defaults", "path", cfgPath)
		cfg := &Config{}
		cfg.Server.Port = 8080
		cfg.Storage.Backend = "duckdb"
		return cfg, nil
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Storage.Backend == "" {
		cfg.Storage.Backend = "duckdb"
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
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms")
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
