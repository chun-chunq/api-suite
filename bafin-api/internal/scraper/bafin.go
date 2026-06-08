// Package scraper scrapes the public BaFin institution database.
// Data source: https://portal.mvp.bafin.de/database/InstInfo/
// Legal basis: KWG §32, WpIG §15, ZAG §10 — publicly required disclosure
// BaFin is the German Federal Financial Supervisory Authority (Bundesanstalt für Finanzdienstleistungsaufsicht).
package scraper

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

const (
	bafInSearchURL = "https://portal.mvp.bafin.de/database/InstInfo/"
	// BaFin license type codes used in the portal dropdown
)

// LicenseTypes maps user-friendly names to BaFin portal values.
var LicenseTypes = map[string]string{
	"bank":               "CRR-Kreditinstitut",
	"investmentfirm":     "Wertpapierinstitut",
	"paymentinstitution": "Zahlungsinstitut",
	"emoneyinstitution":  "E-Geld-Institut",
	"cryptoassets":       "Kryptowertedienstleister",
	"insurance":          "Versicherungsunternehmen",
	"fundmanager":        "Kapitalverwaltungsgesellschaft",
	"broker":             "Finanzdienstleistungsinstitut",
}

// Scraper scrapes BaFin's public institution database.
type Scraper struct {
	browser   *rod.Browser
	chromeBin string
	log       zerolog.Logger
}

// Options for creating a Scraper.
type Options struct {
	Logger    zerolog.Logger
	BrowserBin string
}

// New creates and launches a headless browser for BaFin scraping.
func New(opts Options) (*Scraper, error) {
	s := &Scraper{log: opts.Logger, chromeBin: opts.BrowserBin}

	l := launcher.New().
		Headless(true).
		NoSandbox(true).
		Set("disable-dev-shm-usage", "").
		Set("disable-gpu", "").
		Set("disable-images", "") // save bandwidth
	if opts.BrowserBin != "" {
		l = l.Bin(opts.BrowserBin)
	}

	url, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch browser: %w", err)
	}
	s.browser = rod.New().ControlURL(url).MustConnect()
	return s, nil
}

// Close shuts down the browser.
func (s *Scraper) Close() {
	if s.browser != nil {
		s.browser.MustClose()
	}
}

// Search searches for BaFin-licensed institutions.
func (s *Scraper) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	if q.MaxResults <= 0 || q.MaxResults > 200 {
		q.MaxResults = 50
	}

	page, err := s.browser.Page(proto.TargetCreateTarget{URL: bafInSearchURL})
	if err != nil {
		return nil, fmt.Errorf("open BaFin page: %w", err)
	}
	defer page.Close()

	// Wait for the page to load
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("BaFin page load timeout: %w", err)
	}

	// Give JS time to render
	time.Sleep(2 * time.Second)

	// Fill the company name field if provided
	if q.Name != "" {
		if err := s.fillField(page, `input[name="companyName"], input[id*="Name"], input[placeholder*="Name"], #companyname`, q.Name); err != nil {
			s.log.Warn().Err(err).Msg("could not fill name field, trying search anyway")
		}
	}

	// Select license type if specified
	if q.LicenseType != "" {
		bafInType := q.LicenseType
		if mapped, ok := LicenseTypes[strings.ToLower(q.LicenseType)]; ok {
			bafInType = mapped
		}
		_ = s.selectOption(page, `select[name*="type"], select[id*="type"], select[name*="Type"]`, bafInType)
	}

	// Submit search form
	if err := s.submitForm(page); err != nil {
		return nil, fmt.Errorf("submit BaFin form: %w", err)
	}

	// Wait for results
	time.Sleep(3 * time.Second)
	if err := page.WaitLoad(); err != nil {
		s.log.Warn().Msg("BaFin result page slow to load")
	}

	// Parse results table
	institutions, err := s.parseResults(page, q.MaxResults)
	if err != nil {
		return nil, fmt.Errorf("parse BaFin results: %w", err)
	}

	// Apply client-side filters
	if q.StatusFilter != "" {
		filtered := institutions[:0]
		for _, inst := range institutions {
			if strings.EqualFold(inst.Status, q.StatusFilter) {
				filtered = append(filtered, inst)
			}
		}
		institutions = filtered
	}

	return &SearchResult{
		Total:   len(institutions),
		Results: institutions,
		Query:   q,
	}, nil
}

func (s *Scraper) fillField(page *rod.Page, selector, value string) error {
	// Try multiple selectors
	for _, sel := range strings.Split(selector, ",") {
		sel = strings.TrimSpace(sel)
		el, err := page.Element(sel)
		if err != nil {
			continue
		}
		if err := el.Input(value); err != nil {
			continue
		}
		return nil
	}
	return fmt.Errorf("no field found for selector: %s", selector)
}

func (s *Scraper) selectOption(page *rod.Page, selector, value string) error {
	for _, sel := range strings.Split(selector, ",") {
		sel = strings.TrimSpace(sel)
		el, err := page.Element(sel)
		if err != nil {
			continue
		}
		if err := el.Select([]string{value}, true, rod.SelectorTypeText); err != nil {
			continue
		}
		return nil
	}
	return fmt.Errorf("select option failed")
}

func (s *Scraper) submitForm(page *rod.Page) error {
	// Try common submit button selectors
	for _, sel := range []string{
		`button[type="submit"]`,
		`input[type="submit"]`,
		`button[id*="search"]`,
		`button[id*="Search"]`,
		`a[id*="search"]`,
	} {
		el, err := page.Element(sel)
		if err != nil {
			continue
		}
		return el.Click(proto.InputMouseButtonLeft, 1)
	}
	return fmt.Errorf("no submit button found")
}

func (s *Scraper) parseResults(page *rod.Page, maxResults int) ([]Institution, error) {
	// Try to extract data via JavaScript — more robust than HTML parsing for JS-rendered pages
	result, err := page.Eval(`() => {
		const rows = [];
		// Try standard table rows
		const tableRows = document.querySelectorAll('table tbody tr, .result-row, .institution-row, [class*="result"]');
		tableRows.forEach(row => {
			const cells = row.querySelectorAll('td, .cell, [class*="col"]');
			if (cells.length >= 2) {
				const getText = el => el ? el.innerText.trim() : '';
				const getLink = el => el ? el.querySelector('a')?.href || '' : '';
				rows.push({
					name:      getText(cells[0]),
					bafinId:   getText(cells[1]),
					licenseType: cells[2] ? getText(cells[2]) : '',
					status:    cells[3] ? getText(cells[3]) : '',
					city:      cells[4] ? getText(cells[4]) : '',
					country:   cells[5] ? getText(cells[5]) : 'DE',
					detailUrl: getLink(cells[0]),
				});
			}
		});

		// Fallback: look for any list-like structure with institution names
		if (rows.length === 0) {
			document.querySelectorAll('[class*="institution"], [class*="company"], [class*="entry"]').forEach(el => {
				const nameEl = el.querySelector('a, h3, h4, [class*="name"]');
				const idEl = el.querySelector('[class*="id"], [class*="bafinId"]');
				if (nameEl) {
					rows.push({
						name:    nameEl.innerText.trim(),
						bafinId: idEl ? idEl.innerText.trim() : '',
						licenseType: '',
						status: 'Active',
						city: '',
						country: 'DE',
						detailUrl: nameEl.href || nameEl.querySelector('a')?.href || '',
					});
				}
			});
		}
		return rows;
	}`)
	if err != nil {
		return nil, fmt.Errorf("JS evaluation failed: %w", err)
	}

	type rawRow struct {
		Name        string `json:"name"`
		BaFinID     string `json:"bafinId"`
		LicenseType string `json:"licenseType"`
		Status      string `json:"status"`
		City        string `json:"city"`
		Country     string `json:"country"`
		DetailURL   string `json:"detailUrl"`
	}

	var rawRows []rawRow
	if err := result.Value.Unmarshal(&rawRows); err != nil {
		return nil, fmt.Errorf("unmarshal rows: %w", err)
	}

	institutions := make([]Institution, 0, len(rawRows))
	for i, r := range rawRows {
		if i >= maxResults {
			break
		}
		if r.Name == "" {
			continue
		}
		status := r.Status
		if status == "" {
			status = "Active"
		}
		country := r.Country
		if country == "" {
			country = "DE"
		}
		institutions = append(institutions, Institution{
			BaFinID:     strings.TrimSpace(r.BaFinID),
			Name:        strings.TrimSpace(r.Name),
			Status:      normalizeStatus(status),
			LicenseType: strings.TrimSpace(r.LicenseType),
			City:        strings.TrimSpace(r.City),
			Country:     strings.TrimSpace(country),
			DetailURL:   strings.TrimSpace(r.DetailURL),
		})
	}
	return institutions, nil
}

func normalizeStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.Contains(s, "aktiv") || strings.Contains(s, "active") || strings.Contains(s, "zugelassen"):
		return "Active"
	case strings.Contains(s, "widerruf") || strings.Contains(s, "revoked"):
		return "Revoked"
	case strings.Contains(s, "aufgehoben") || strings.Contains(s, "withdrawn"):
		return "Withdrawn"
	case strings.Contains(s, "erloschen") || strings.Contains(s, "expired"):
		return "Expired"
	default:
		if s == "" {
			return "Active"
		}
		return s
	}
}
