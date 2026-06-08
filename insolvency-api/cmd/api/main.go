package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/insolvency-api/config"
	"github.com/insolvency-api/internal/api"
	"github.com/insolvency-api/internal/cache"
	"github.com/insolvency-api/internal/worker"
)

func main() {
	cfg := config.Load()

	logger := newLogger(cfg.LogLevel)
	zerolog.DefaultContextLogger = &logger

	logger.Info().Str("env", cfg.Environment).Str("port", cfg.Port).Msg("starting insolvency-api")

	c := cache.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := c.Ping(ctx); err != nil {
		logger.Warn().Err(err).Msg("redis unreachable at startup; running without cache acceleration")
	}
	cancel()

	// Start the asynq background worker (best-effort; logs and continues on error).
	startWorker(cfg, c, logger)

	app := api.NewRouter(cfg, c, logger)

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			logger.Fatal().Err(err).Msg("server stopped")
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("graceful shutdown failed")
	}
}

func startWorker(cfg *config.Config, c *cache.Cache, logger zerolog.Logger) {
	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 5,
		Logger:      &asynqLogger{logger},
	})

	mux := asynq.NewServeMux()
	mux.Handle(worker.TaskMonitorCompany, worker.NewMonitorHandler(c, logger))

	go func() {
		if err := srv.Run(mux); err != nil {
			logger.Warn().Err(err).Msg("asynq worker stopped")
		}
	}()
}

func newLogger(level string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)
	return log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()
}

// asynqLogger adapts zerolog to the asynq.Logger interface.
type asynqLogger struct{ l zerolog.Logger }

func (a *asynqLogger) Debug(args ...any) { a.l.Debug().Msgf("%v", args) }
func (a *asynqLogger) Info(args ...any)  { a.l.Info().Msgf("%v", args) }
func (a *asynqLogger) Warn(args ...any)  { a.l.Warn().Msgf("%v", args) }
func (a *asynqLogger) Error(args ...any) { a.l.Error().Msgf("%v", args) }
func (a *asynqLogger) Fatal(args ...any) { a.l.Fatal().Msgf("%v", args) }
