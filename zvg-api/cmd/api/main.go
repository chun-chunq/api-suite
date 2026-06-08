package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/zvg-api/config"
	"github.com/zvg-api/internal/api"
	"github.com/zvg-api/internal/cache"
)

func main() {
	cfg := config.Load()

	lvl, _ := zerolog.ParseLevel(cfg.LogLevel)
	zerolog.SetGlobalLevel(lvl)
	logger := log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()

	logger.Info().Str("port", cfg.Port).Str("env", cfg.Environment).Msg("starting zvg-api")

	c := cache.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := c.Ping(ctx); err != nil {
		logger.Warn().Err(err).Msg("redis unreachable; running without cache")
	}
	cancel()

	app := api.NewRouter(cfg, c, logger)

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			logger.Fatal().Err(err).Msg("server stopped")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	app.ShutdownWithContext(shutCtx)
}
