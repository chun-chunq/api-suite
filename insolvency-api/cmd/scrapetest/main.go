// Command scrapetest is a standalone runnable that exercises the live
// insolvenzbekanntmachungen.de scraper without Redis or the HTTP API.
//
//	go run ./cmd/scrapetest
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/insolvency-api/internal/scraper"
)

func main() {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen}).
		With().Timestamp().Logger()

	sc, err := scraper.New(scraper.Options{
		Timeout:    60 * time.Second,
		BrowserBin: `C:\Program Files\Google\Chrome\Application\chrome.exe`,
		Logger:     logger,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("create scraper")
	}
	defer sc.Close()

	// 1) Search for "Müller" over the last 30 days (common German name that returns results).
	fmt.Fprintln(os.Stderr, "=== Search: 'Müller', last 30 days ===")
	ctx1, cancel1 := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel1()
	q := scraper.SearchQuery{
		Name:     "Müller",
		DateFrom: time.Now().AddDate(0, 0, -30),
		DateTo:   time.Now(),
		MaxPages: 1,
	}
	res, err := sc.Search(ctx1, q)
	if err != nil {
		logger.Error().Err(err).Msg("search failed")
	} else {
		first := res.Records
		if len(first) > 5 {
			first = first[:5]
		}
		fmt.Println("First 5 results:")
		printJSON(first)
		fmt.Fprintf(os.Stderr, "Total found on page: %d\n", res.Totalfound)
	}

	// 2) Search for "Schmidt GmbH" - a typical company name search.
	fmt.Fprintln(os.Stderr, "\n=== Search: 'Schmidt GmbH', last 30 days ===")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel2()
	res2, err := sc.Search(ctx2, scraper.SearchQuery{
		Name:     "Schmidt",
		DateFrom: time.Now().AddDate(0, 0, -30),
		DateTo:   time.Now(),
		MaxPages: 1,
	})
	if err != nil {
		logger.Error().Err(err).Msg("search2 failed")
	} else {
		out := res2.Records
		if len(out) > 3 {
			out = out[:3]
		}
		fmt.Println("First 3 results:")
		printJSON(out)
		fmt.Fprintf(os.Stderr, "Total found: %d\n", res2.Totalfound)
	}
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println("marshal error:", err)
		return
	}
	fmt.Println(string(b))
}
