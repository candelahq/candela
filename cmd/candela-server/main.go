// Candela server — single-binary backend serving ConnectRPC (for the UI) and
// handling span ingestion. SQLite by default for local dev, BigQuery/ClickHouse
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

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"gopkg.in/yaml.v3"

	"github.com/candelahq/candela/gen/go/candela/v1/candelav1connect"
	"github.com/candelahq/candela/pkg/connecthandlers"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
	chstore "github.com/candelahq/candela/pkg/storage/clickhouse"
	sqlitestore "github.com/candelahq/candela/pkg/storage/sqlite"
)

// Config holds the server configuration.
type Config struct {
	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	Storage struct {
		Backend string `yaml:"backend"` // "sqlite" (default), "clickhouse", "bigquery"
		SQLite  struct {
			Path string `yaml:"path"` // e.g. "candela.db" or ":memory:"
		} `yaml:"sqlite"`
		ClickHouse struct {
			Addr     string `yaml:"addr"`
			Database string `yaml:"database"`
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"clickhouse"`
	} `yaml:"storage"`
	Worker struct {
		BatchSize    int `yaml:"batch_size"`
		FlushInterval string `yaml:"flush_interval"`
	} `yaml:"worker"`
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// Initialize storage backend.
	store, err := initStorage(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize storage")
	}
	defer store.Close()
	log.Info().Str("backend", cfg.Storage.Backend).Msg("storage initialized")

	// Initialize cost calculator.
	calc := costcalc.New()

	// Start the in-process span processor (replaces the separate worker).
	processor := NewSpanProcessor(store, calc, cfg.Worker.BatchSize)
	go processor.Run(context.Background())
	defer processor.Stop()

	// Build the HTTP mux for ConnectRPC handlers.
	mux := http.NewServeMux()

	// Health check.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := store.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status": "error", "detail": %q}`, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status": "ok"}`)
	})

	// Register ConnectRPC service handlers.
	tracePath, traceH := candelav1connect.NewTraceServiceHandler(
		connecthandlers.NewTraceHandler(store))
	mux.Handle(tracePath, traceH)

	ingestionPath, ingestionH := candelav1connect.NewIngestionServiceHandler(
		connecthandlers.NewIngestionHandlerDirect(processor))
	mux.Handle(ingestionPath, ingestionH)

	dashboardPath, dashboardH := candelav1connect.NewDashboardServiceHandler(
		connecthandlers.NewDashboardHandler(store))
	mux.Handle(dashboardPath, dashboardH)

	log.Info().
		Str("trace", tracePath).
		Str("ingestion", ingestionPath).
		Str("dashboard", dashboardPath).
		Msg("ConnectRPC services registered")

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info().Str("addr", addr).Msg("🕯️ Candela server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	processor.Stop()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("shutdown error")
	}
	log.Info().Msg("server stopped")
}

func initStorage(cfg *Config) (storage.TraceStore, error) {
	switch cfg.Storage.Backend {
	case "sqlite", "":
		return sqlitestore.New(sqlitestore.Config{
			Path: cfg.Storage.SQLite.Path,
		})
	case "clickhouse":
		s, err := chstore.New(chstore.Config{
			Addr:     cfg.Storage.ClickHouse.Addr,
			Database: cfg.Storage.ClickHouse.Database,
			Username: cfg.Storage.ClickHouse.Username,
			Password: cfg.Storage.ClickHouse.Password,
		})
		if err != nil {
			return nil, err
		}
		return s, s.Migrate(context.Background())
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", cfg.Storage.Backend)
	}
}

func loadConfig() (*Config, error) {
	cfgPath := os.Getenv("CANDELA_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		// No config file — use defaults (SQLite, port 8080).
		log.Warn().Str("path", cfgPath).Msg("config file not found, using defaults")
		cfg := &Config{}
		cfg.Server.Port = 8080
		cfg.Storage.Backend = "sqlite"
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
		cfg.Storage.Backend = "sqlite"
	}

	return &cfg, nil
}
