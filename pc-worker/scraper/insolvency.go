package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// InsolvencyQuery mirrors insolvency-api SearchQuery.
type InsolvencyQuery struct {
	Name        string `json:"name"`
	DateFrom    string `json:"dateFrom"`
	DateTo      string `json:"dateTo"`
	CourtFilter string `json:"courtFilter"`
	State       string `json:"state"`
	MaxResults  int    `json:"maxResults"`
}

// InsolvencyResult mirrors insolvency-api SearchResult.
type InsolvencyResult struct {
	Results    []InsolvencyEntry `json:"results"`
	TotalCount int               `json:"totalCount"`
	Page       int               `json:"page"`
	ScrapedAt  time.Time         `json:"scrapedAt"`
}

type InsolvencyEntry struct {
	ReferenceNumber string `json:"referenceNumber"`
	Court           string `json:"court"`
	Debtor          string `json:"debtor"`
	Address         string `json:"address"`
	PublishedAt     string `json:"publishedAt"`
	Category        string `json:"category"`
	DetailURL       string `json:"detailUrl"`
}

const insolvenzURL = "https://www.insolvenzbekanntmachungen.de/cgi-bin/bl_suche.pl"

// RunInsolvency parses the payload JSON, runs the scrape, and returns raw JSON.
func RunInsolvency(ctx context.Context, chromeBin string, payload json.RawMessage, log zerolog.Logger) (json.RawMessage, error) {
	var q InsolvencyQuery
	if err := json.Unmarshal(payload, &q); err != nil {
		return nil, fmt.Errorf("insolvency payload ungültig: %w", err)
	}
	if q.MaxResults <= 0 {
		q.MaxResults = 50
	}

	browser, cleanup, err := launchBrowser(chromeBin, log)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("neue Seite konnte nicht erstellt werden: %w", err)
	}
	defer page.Close()

	if err := page.Context(ctx).Navigate(insolvenzURL); err != nil {
		return nil, fmt.Errorf("Navigation fehlgeschlagen: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("Seitenladung fehlgeschlagen: %w", err)
	}

	// Fill name field
	if q.Name != "" {
		if el, err := page.Element(`input[name="select_name1"]`); err == nil {
			el.Input(q.Name)
		}
	}
	if q.State != "" {
		if el, err := page.Element(`select[name="bundesland"]`); err == nil {
			el.Select([]string{q.State}, true, rod.SelectorTypeText)
		}
	}
	if q.DateFrom != "" {
		if el, err := page.Element(`input[name="datum_von"]`); err == nil {
			el.Input(q.DateFrom)
		}
	}
	if q.DateTo != "" {
		if el, err := page.Element(`input[name="datum_bis"]`); err == nil {
			el.Input(q.DateTo)
		}
	}

	// Submit
	if el, err := page.Element(`input[type="submit"]`); err == nil {
		wait := page.MustWaitNavigation()
		el.Click("left", 1)
		wait()
	}

	// Parse results table
	rows, _ := page.Elements(`table tr`)
	var entries []InsolvencyEntry
	for _, row := range rows {
		cells, _ := row.Elements("td")
		if len(cells) < 4 {
			continue
		}
		getText := func(i int) string {
			if i >= len(cells) {
				return ""
			}
			t, _ := cells[i].Text()
			return strings.TrimSpace(t)
		}
		entry := InsolvencyEntry{
			PublishedAt:     getText(0),
			Court:           getText(1),
			ReferenceNumber: getText(2),
			Debtor:          getText(3),
		}
		if entry.ReferenceNumber == "" || entry.Court == "" {
			continue
		}
		if a, err := cells[2].Element("a"); err == nil {
			if href, err := a.Attribute("href"); err == nil && href != nil {
				entry.DetailURL = *href
			}
		}
		entries = append(entries, entry)
		if len(entries) >= q.MaxResults {
			break
		}
	}

	result := InsolvencyResult{
		Results:    entries,
		TotalCount: len(entries),
		Page:       1,
		ScrapedAt:  time.Now(),
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
