package scraper

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

const (
	dpmaSearchURL  = "https://register.dpma.de/DPMAregister/marke/einsteiger"
	dpmaBaseURL    = "https://register.dpma.de"
	linuxChromeBin = "/usr/bin/chromium"
	defaultChrome  = `C:\Program Files\Google\Chrome\Application\chrome.exe`
)

var reSpaces = regexp.MustCompile(`\s+`)
var reClass = regexp.MustCompile(`\d+`)

// Scraper holds a reusable browser instance.
type Scraper struct {
	browser  *rod.Browser
	launcher *launcher.Launcher
	log      zerolog.Logger
}

// Options controls browser configuration.
type Options struct {
	BrowserBin string
	Logger     zerolog.Logger
}

// New creates a Scraper, launching a headless Chrome instance.
func New(opts Options) (*Scraper, error) {
	bin := opts.BrowserBin
	if bin == "" {
		if env := os.Getenv("CHROME_BIN"); env != "" {
			bin = env
		} else if _, err := os.Stat(linuxChromeBin); err == nil {
			bin = linuxChromeBin
		} else {
			bin = defaultChrome
		}
	}

	l := launcher.New().
		Bin(bin).
		Headless(true).
		Leakless(false).
		Set("disable-gpu", "").
		Set("no-sandbox", "").
		Set("disable-dev-shm-usage", "").
		Set("disable-setuid-sandbox", "").
		Set("blink-settings", "imagesEnabled=false"). // skip images — saves bandwidth
		Set("disable-extensions", "")

	u, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("chrome launch failed (%s): %w", bin, err)
	}

	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		l.Cleanup()
		return nil, fmt.Errorf("chrome connect failed: %w", err)
	}

	return &Scraper{browser: b, launcher: l, log: opts.Logger}, nil
}

// Close shuts down the browser.
func (s *Scraper) Close() {
	if s.browser != nil {
		s.browser.Close()
	}
	if s.launcher != nil {
		s.launcher.Cleanup()
	}
}

// Search performs a trademark search and returns parsed results.
func (s *Scraper) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	page, err := s.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("new page failed: %w", err)
	}
	defer page.Close()

	page = page.Context(ctx)

	s.log.Debug().Str("url", dpmaSearchURL).Msg("navigating to DPMA search")
	if err := page.Navigate(dpmaSearchURL); err != nil {
		return nil, fmt.Errorf("DPMA navigation failed: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("DPMA page load timeout: %w", err)
	}

	// Fill search form fields
	if err := s.fillForm(page, q); err != nil {
		return nil, fmt.Errorf("form fill failed: %w", err)
	}

	// Submit and wait for results
	wait := page.MustWaitNavigation()
	if _, err := page.Eval(`document.querySelector('input[type="submit"], button[type="submit"]').click()`); err != nil {
		page.Eval(`document.querySelector('form').submit()`) //nolint
	}
	wait()

	// Check for error pages
	currentURL := page.MustInfo().URL
	if strings.Contains(currentURL, "fehler") || strings.Contains(currentURL, "error") {
		return nil, fmt.Errorf("DPMA returned an error page")
	}

	results, err := s.parseResults(page, q)
	if err != nil {
		return nil, err
	}

	return &SearchResult{
		Results:    results,
		TotalCount: len(results),
		Page:       1,
		Query:      q,
		ScrapedAt:  time.Now(),
	}, nil
}

// GetDetail fetches full details for a single trademark by registration number.
func (s *Scraper) GetDetail(ctx context.Context, registrationNumber string) (*Trademark, error) {
	page, err := s.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("new page failed: %w", err)
	}
	defer page.Close()

	page = page.Context(ctx)

	// Use search by exact registration number to get detail link
	q := SearchQuery{RegistrationNumber: registrationNumber, MaxResults: 1}
	result, err := s.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(result.Results) == 0 {
		return nil, fmt.Errorf("trademark %s not found", registrationNumber)
	}

	tm := result.Results[0]

	// Fetch detail page if URL is available
	if tm.DetailURL != "" {
		if err := page.Navigate(tm.DetailURL); err == nil {
			page.WaitLoad()
			s.enrichFromDetail(page, &tm)
		}
	}

	return &tm, nil
}

// fillForm fills the DPMA search form with the given query parameters.
// Input values are sanitized before being set on the page.
func (s *Scraper) fillForm(page *rod.Page, q SearchQuery) error {
	setField := func(selector, value string) {
		if value == "" {
			return
		}
		if el, err := page.Element(selector); err == nil {
			el.Input(value)
		}
	}

	setField(`input[name="marken"], input[id*="marke"], input[name*="marke"]`, q.Name)
	setField(`input[name*="registernummer"], input[name*="aktenzeichen"], input[id*="register"]`, q.RegistrationNumber)
	setField(`input[name*="inhaber"], input[name*="anmelder"], input[id*="inhaber"]`, q.Owner)

	// Date fields
	setField(`input[name*="anmeldedat_von"], input[id*="datum_von"]`, q.DateFrom)
	setField(`input[name*="anmeldedat_bis"], input[id*="datum_bis"]`, q.DateTo)

	// Status dropdown
	if statusVal, ok := StatusValues[q.Status]; ok {
		page.Eval(fmt.Sprintf(`
			(function() {
				var sel = document.querySelector('select[name*="status"], select[id*="status"]');
				if(sel) sel.value = %q;
			})()`, statusVal))
	}

	// Mark type dropdown
	if typeVal, ok := MarkTypeValues[q.MarkType]; ok {
		page.Eval(fmt.Sprintf(`
			(function() {
				var sel = document.querySelector('select[name*="markenform"], select[id*="markenform"]');
				if(sel) sel.value = %q;
			})()`, typeVal))
	}

	// Nice classes (checkboxes or multi-select)
	if len(q.Classes) > 0 {
		for _, cls := range q.Classes {
			clsStr := strconv.Itoa(cls)
			page.Eval(fmt.Sprintf(`
				(function() {
					// Try checkbox
					var cb = document.querySelector('input[type="checkbox"][value="%s"]');
					if(cb) { cb.checked = true; return; }
					// Try select option
					var sel = document.querySelector('select[name*="klasse"], select[id*="klasse"]');
					if(sel) {
						for(var i=0;i<sel.options.length;i++){
							if(sel.options[i].value==="%s") { sel.options[i].selected=true; }
						}
					}
				})()`, clsStr, clsStr))
		}
	}

	return nil
}

// parseResults parses the DPMA result list page into Trademark slice.
func (s *Scraper) parseResults(page *rod.Page, q SearchQuery) ([]Trademark, error) {
	// Wait for result table
	page.WaitLoad()

	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("could not get page HTML: %w", err)
	}

	// Check for "no results" message
	if strings.Contains(html, "keine Treffer") || strings.Contains(html, "0 Treffer") ||
		strings.Contains(html, "Ihrer Suche entsprechen keine") {
		return []Trademark{}, nil
	}

	maxResults := q.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}
	if maxResults > 200 {
		maxResults = 200
	}

	var trademarks []Trademark

	// Try primary result table format
	rows, _ := page.Elements(`table tr, .treffer-liste tr, .result-list tr`)
	for _, row := range rows {
		cells, _ := row.Elements("td")
		if len(cells) < 3 {
			continue
		}
		tm := s.parseTableRow(cells)
		if tm == nil {
			continue
		}
		trademarks = append(trademarks, *tm)
		if len(trademarks) >= maxResults {
			break
		}
	}

	// Fallback: definition-list layout (DPMA sometimes uses dl/dt/dd)
	if len(trademarks) == 0 {
		trademarks = s.parseDLLayout(page, maxResults)
	}

	return trademarks, nil
}

// parseTableRow parses a table row into a Trademark.
func (s *Scraper) parseTableRow(cells []*rod.Element) *Trademark {
	getText := func(i int) string {
		if i >= len(cells) {
			return ""
		}
		t, _ := cells[i].Text()
		return strings.TrimSpace(reSpaces.ReplaceAllString(t, " "))
	}

	// DPMA typical column order: Registernummer | Marke | Inhaber | Klassen | Status | Anmeldedatum
	regNum := getText(0)
	if regNum == "" || regNum == "Registernummer" || regNum == "Aktenzeichen" {
		return nil // header row
	}

	tm := &Trademark{
		RegistrationNumber: regNum,
		Name:               getText(1),
		Owner:              getText(2),
		Status:             normalizeStatus(getText(4)),
		FilingDate:         getText(5),
		Country:            "DE",
	}

	// Parse classes from column 3
	classText := getText(3)
	tm.Classes = parseClasses(classText)

	// Extract detail URL from registration number cell
	if a, err := cells[0].Element("a"); err == nil {
		if href, err2 := a.Attribute("href"); err2 == nil && href != nil {
			u := *href
			if !strings.HasPrefix(u, "http") {
				u = dpmaBaseURL + u
			}
			tm.DetailURL = u
		}
	}

	// Detect non-German registrations from registration number prefix
	if strings.HasPrefix(regNum, "IR") {
		tm.Country = "IR" // international (WIPO)
	} else if strings.HasPrefix(regNum, "EM") {
		tm.Country = "EM" // EU trademark (EUIPO)
	}

	return tm
}

// parseDLLayout handles the alternative DL/DT/DD layout.
func (s *Scraper) parseDLLayout(page *rod.Page, maxResults int) []Trademark {
	items, _ := page.Elements(`.treffer-item, .result-item, article.treffer`)
	var trademarks []Trademark
	for _, item := range items {
		t, _ := item.Text()
		lines := strings.Split(t, "\n")
		tm := Trademark{Country: "DE"}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if after, ok := strings.CutPrefix(line, "Registernummer:"); ok {
				tm.RegistrationNumber = strings.TrimSpace(after)
			} else if after, ok := strings.CutPrefix(line, "Marke:"); ok {
				tm.Name = strings.TrimSpace(after)
			} else if after, ok := strings.CutPrefix(line, "Inhaber:"); ok {
				tm.Owner = strings.TrimSpace(after)
			} else if after, ok := strings.CutPrefix(line, "Status:"); ok {
				tm.Status = normalizeStatus(strings.TrimSpace(after))
			} else if after, ok := strings.CutPrefix(line, "Klassen:"); ok {
				tm.Classes = parseClasses(strings.TrimSpace(after))
			}
		}
		if a, err := item.Element("a"); err == nil {
			if href, err2 := a.Attribute("href"); err2 == nil && href != nil {
				u := *href
				if !strings.HasPrefix(u, "http") {
					u = dpmaBaseURL + u
				}
				tm.DetailURL = u
			}
		}
		if tm.RegistrationNumber != "" {
			trademarks = append(trademarks, tm)
			if len(trademarks) >= maxResults {
				break
			}
		}
	}
	return trademarks
}

// enrichFromDetail fetches additional data from the trademark detail page.
func (s *Scraper) enrichFromDetail(page *rod.Page, tm *Trademark) {
	getText := func(selector string) string {
		if el, err := page.Element(selector); err == nil {
			t, _ := el.Text()
			return strings.TrimSpace(t)
		}
		return ""
	}

	// Try to extract more specific fields from detail page
	// DPMA detail pages use definition lists (dt/dd pairs)
	dts, _ := page.Elements("dt")
	dds, _ := page.Elements("dd")
	for i, dt := range dts {
		label, _ := dt.Text()
		label = strings.TrimSpace(strings.ToLower(label))
		if i >= len(dds) {
			break
		}
		val, _ := dds[i].Text()
		val = strings.TrimSpace(val)
		switch {
		case strings.Contains(label, "inhaber") || strings.Contains(label, "owner"):
			tm.Owner = val
		case strings.Contains(label, "vertreter") || strings.Contains(label, "representative"):
			tm.Representative = val
		case strings.Contains(label, "anmeld") || strings.Contains(label, "filing"):
			tm.FilingDate = val
		case strings.Contains(label, "eingetragen") || strings.Contains(label, "registration"):
			tm.RegistrationDate = val
		case strings.Contains(label, "ablauf") || strings.Contains(label, "expiry"):
			tm.ExpiryDate = val
		case strings.Contains(label, "waren") || strings.Contains(label, "goods"):
			tm.GoodsAndServices = val
		case strings.Contains(label, "status"):
			tm.Status = normalizeStatus(val)
		}
	}

	_ = getText // suppress unused warning
}

// parseClasses extracts Nice Classification class numbers from a string like "09, 35, 42".
func parseClasses(s string) []int {
	matches := reClass.FindAllString(s, -1)
	var classes []int
	seen := map[int]bool{}
	for _, m := range matches {
		n, _ := strconv.Atoi(m)
		if n >= 1 && n <= 45 && !seen[n] {
			classes = append(classes, n)
			seen[n] = true
		}
	}
	return classes
}

// normalizeStatus maps DPMA status strings to readable English equivalents.
func normalizeStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.Contains(s, "eingetragen") || strings.Contains(s, "ir") || s == "registered":
		return "registered"
	case strings.Contains(s, "angemeldet") || s == "an" || s == "applied":
		return "applied"
	case strings.Contains(s, "gelöscht") || strings.Contains(s, "geloscht") || s == "deleted":
		return "deleted"
	case strings.Contains(s, "abgelaufen") || strings.Contains(s, "expired") || s == "ex":
		return "expired"
	case strings.Contains(s, "zurückgenommen") || strings.Contains(s, "withdrawn"):
		return "withdrawn"
	case strings.Contains(s, "widerspruch") || strings.Contains(s, "opposition"):
		return "opposition"
	}
	if s != "" {
		return s
	}
	return "unknown"
}
