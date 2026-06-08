package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/handelsregister-api/internal/scraper"
)

func main() {
	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()

	log.Info().Msg("starting scraper test against handelsregister.de")

	s, err := scraper.New(scraper.Options{
		Timeout:    60 * time.Second,
		Logger:     log,
		BrowserBin: `C:\Program Files\Google\Chrome\Application\chrome.exe`,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to launch browser")
	}
	defer s.Close()

	ctx := context.Background()

	// Test 1: Search by company name
	log.Info().Msg("TEST 1: searching for 'BMW' — also taking screenshot of results page")
	results, err := s.Search(ctx, "BMW")
	if err != nil {
		log.Error().Err(err).Msg("search failed")
	} else {
		fmt.Printf("\n=== Search Results for 'BMW' (%d hits) ===\n", len(results))
		for i, r := range results {
			if i >= 5 {
				fmt.Printf("... and %d more\n", len(results)-5)
				break
			}
			b, _ := json.MarshalIndent(r, "  ", "  ")
			fmt.Printf("  [%d] %s\n", i+1, b)
		}
	}

	// Test 2: Search for a known GmbH
	log.Info().Msg("TEST 2: searching for 'Bosch GmbH'")
	results2, err := s.Search(ctx, "Robert Bosch GmbH")
	if err != nil {
		log.Error().Err(err).Msg("search failed")
	} else {
		fmt.Printf("\n=== Search Results for 'Robert Bosch GmbH' (%d hits) ===\n", len(results2))
		for i, r := range results2 {
			if i >= 3 {
				break
			}
			b, _ := json.MarshalIndent(r, "  ", "  ")
			fmt.Printf("  [%d] %s\n", i+1, b)
		}
	}

	log.Info().Msg("scraper test complete")
}
