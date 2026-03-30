// Candela worker — pulls spans from the Redis queue, enriches them with cost
// data, and writes them to the storage backend.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"github.com/candelahq/candela/pkg/costcalc"
	redisqueue "github.com/candelahq/candela/pkg/ingestion/redis"
	chstore "github.com/candelahq/candela/pkg/storage/clickhouse"
)

// Config holds the worker configuration.
type Config struct {
	Storage struct {
		ClickHouse struct {
			Addr     string `yaml:"addr"`
			Database string `yaml:"database"`
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"clickhouse"`
	} `yaml:"storage"`
	Queue struct {
		Redis struct {
			Addr string `yaml:"addr"`
		} `yaml:"redis"`
	} `yaml:"queue"`
	Worker struct {
		BatchSize   int `yaml:"batch_size"`
		Concurrency int `yaml:"concurrency"`
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
	store, err := chstore.New(chstore.Config{
		Addr:     cfg.Storage.ClickHouse.Addr,
		Database: cfg.Storage.ClickHouse.Database,
		Username: cfg.Storage.ClickHouse.Username,
		Password: cfg.Storage.ClickHouse.Password,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to clickhouse")
	}
	defer store.Close()

	// Run migrations.
	if err := store.Migrate(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}

	// Initialize queue.
	queue, err := redisqueue.New(cfg.Queue.Redis.Addr, "worker-1")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer queue.Close()

	// Initialize cost calculator.
	calc := costcalc.New()

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	batchSize := cfg.Worker.BatchSize
	if batchSize == 0 {
		batchSize = 100
	}

	log.Info().Int("batch_size", batchSize).Msg("🕯️ Candela worker starting")

	// Main processing loop.
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("worker shutting down")
			return
		default:
		}

		spans, err := queue.Pull(ctx, batchSize)
		if err != nil {
			if ctx.Err() != nil {
				return // Shutting down
			}
			log.Error().Err(err).Msg("failed to pull spans from queue")
			continue
		}

		if len(spans) == 0 {
			continue
		}

		// Enrich spans with cost data.
		for i := range spans {
			if spans[i].GenAI != nil && spans[i].GenAI.CostUSD == 0 {
				spans[i].GenAI.CostUSD = calc.Calculate(
					spans[i].GenAI.Provider,
					spans[i].GenAI.Model,
					spans[i].GenAI.InputTokens,
					spans[i].GenAI.OutputTokens,
				)
			}
		}

		// Write to storage.
		if err := store.IngestSpans(ctx, spans); err != nil {
			log.Error().Err(err).Int("count", len(spans)).Msg("failed to write spans")
			continue
		}

		log.Debug().Int("count", len(spans)).Msg("processed spans")
	}
}

func loadConfig() (*Config, error) {
	cfgPath := os.Getenv("CANDELA_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", cfgPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}
