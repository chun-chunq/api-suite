package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/handelsregister-api/config"
	"github.com/handelsregister-api/internal/api"
	"github.com/handelsregister-api/internal/cache"
	"github.com/handelsregister-api/internal/scraper"
	"github.com/handelsregister-api/internal/worker"
)

// version is overridable at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("load config")
	}

	logger := newLogger(cfg.Env)

	rootCtx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Redis-backed cache ---
	c, err := cache.New(rootCtx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.CacheTTL)
	if err != nil {
		logger.Fatal().Err(err).Msg("init cache")
	}
	defer c.Close()

	// --- Headless-Chrome scraper ---
	scr, err := scraper.New(scraper.Options{
		Timeout:    cfg.ScrapeTimeout,
		BrowserBin: cfg.BrowserBin,
		Logger:     logger.With().Str("component", "scraper").Logger(),
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("init scraper")
	}
	defer scr.Close()

	// --- Asynq client (enqueue) ---
	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}
	queue := asynq.NewClient(redisOpt)
	defer queue.Close()

	// --- Asynq worker (process) ---
	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.WorkerConcurrency,
		Logger:      &asynqLogger{logger.With().Str("component", "asynq").Logger()},
		Queues:      map[string]int{"default": 10},
	})
	mux := asynq.NewServeMux()
	worker.NewHandlers(scr, c, logger.With().Str("component", "worker").Logger()).Register(mux)

	// --- HTTP app ---
	app := api.NewRouter(api.Deps{
		Config:  cfg,
		Cache:   c,
		Scraper: scr,
		Queue:   queue,
		Logger:  logger,
		Version: version,
	})

	// Run worker in background.
	workerErr := make(chan error, 1)
	go func() {
		logger.Info().Int("concurrency", cfg.WorkerConcurrency).Msg("starting asynq worker")
		workerErr <- srv.Run(mux)
	}()

	// Run HTTP server in background.
	httpErr := make(chan error, 1)
	go func() {
		logger.Info().Str("port", cfg.Port).Str("version", version).Msg("starting HTTP server")
		httpErr <- app.Listen(":" + cfg.Port)
	}()

	// --- Wait for shutdown signal or fatal error ---
	select {
	case <-rootCtx.Done():
		logger.Info().Msg("shutdown signal received")
	case err := <-httpErr:
		if err != nil {
			logger.Error().Err(err).Msg("http server stopped")
		}
	case err := <-workerErr:
		if err != nil {
			logger.Error().Err(err).Msg("worker stopped")
		}
	}

	// --- Graceful shutdown ---
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	srv.Shutdown()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error().Err(err).Msg("http graceful shutdown")
	}

	logger.Info().Msg("bye")
}

func newLogger(env string) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339
	if env == "development" {
		return zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.Kitchen}).
			With().Timestamp().Logger()
	}
	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}

// asynqLogger adapts zerolog to asynq's Logger interface.
type asynqLogger struct{ l zerolog.Logger }

func (a *asynqLogger) Debug(args ...any) { a.l.Debug().Msg(sprint(args...)) }
func (a *asynqLogger) Info(args ...any)  { a.l.Info().Msg(sprint(args...)) }
func (a *asynqLogger) Warn(args ...any)  { a.l.Warn().Msg(sprint(args...)) }
func (a *asynqLogger) Error(args ...any) { a.l.Error().Msg(sprint(args...)) }
func (a *asynqLogger) Fatal(args ...any) { a.l.Fatal().Msg(sprint(args...)) }

func sprint(args ...any) string {
	return fmt.Sprint(args...)
}
