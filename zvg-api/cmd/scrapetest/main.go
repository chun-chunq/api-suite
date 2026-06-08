package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/zvg-api/internal/scraper"
)

func main() {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen}).
		With().Timestamp().Logger()

	sc, err := scraper.New(scraper.Options{
		Timeout: 90 * time.Second,
		Logger:  logger,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("create scraper")
	}
	defer sc.Close()

	fmt.Fprintln(os.Stderr, "=== Search: Bayern, Einfamilienhaus ===")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	res, err := sc.Search(ctx, scraper.SearchQuery{
		State:       "by",
		ObjectTypes: []string{"3"}, // Einfamilienhaus
		SortBy:      "2",          // by Termin
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("search failed")
	}

	fmt.Fprintf(os.Stderr, "Found: %d auctions\n", res.TotalFound)
	b, _ := json.MarshalIndent(res.Auctions[:min(3, len(res.Auctions))], "", "  ")
	fmt.Println(string(b))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
