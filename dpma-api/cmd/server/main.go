package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	api "github.com/dpma-api/internal/api"
	"github.com/dpma-api/config"
	"github.com/dpma-api/internal/cache"
)

func main() {
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()

	cfg := config.Load()

	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	logger := log.Logger.With().Str("service", "dpma-api").Logger()

	c, err := cache.New(cache.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("redis connection failed")
	}

	app := api.NewRouter(cfg, c, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		logger.Info().Msg("shutting down...")
		app.ShutdownWithTimeout(10 * time.Second)
	}()

	logger.Info().Str("port", cfg.Port).Msg("dpma-trademark-api starting")
	if err := app.Listen(":" + cfg.Port); err != nil {
		logger.Fatal().Err(err).Msg("server error")
	}
}
