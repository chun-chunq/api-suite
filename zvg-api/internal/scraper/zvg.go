package scraper

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

const (
	baseURL           = "https://www.zvg-portal.de"
	searchURL         = baseURL + "/index.php?button=Termine+suchen"
	defaultBrowserBin = `C:\Program Files\Google\Chrome\Application\chrome.exe`
	linuxChromeBin    = `/usr/bin/chromium`
	defaultTimeout    = 60 * time.Second
)

// Scraper drives headless Chrome to search zvg-portal.de.
type Scraper struct {
	browser    *rod.Browser
	log        zerolog.Logger
	timeout    time.Duration
	browserBin string
}

// Options configures a Scraper.
type Options struct {
	Timeout    time.Duration
	BrowserBin string
	Logger     zerolog.Logger
}

// New launches a headless Chrome and returns a ready Scraper.
// If BROWSERLESS_URL is set, connects to a remote browserless instance instead
// of launching a local Chrome binary.
// Caller must call Close() when done.
func New(opts Options) (*Scraper, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}

	var u string
	var err error

	// Prefer remote browserless over local Chrome
	if wsURL := os.Getenv("BROWSERLESS_URL"); wsURL != "" {
		token := os.Getenv("BROWSERLESS_TOKEN")
		if token != "" {
			u = wsURL + "?token=" + token
		} else {
			u = wsURL
		}
	} else {
		// Fall back to local Chrome binary
		if strings.TrimSpace(opts.BrowserBin) == "" {
			if env := os.Getenv("CHROME_BIN"); env != "" {
				opts.BrowserBin = env
			} else if _, err2 := os.Stat(linuxChromeBin); err2 == nil {
				opts.BrowserBin = linuxChromeBin
			} else {
				opts.BrowserBin = defaultBrowserBin
			}
		}
		l := launcher.New().
			Bin(opts.BrowserBin).
			Leakless(false).
			Headless(true).
			Set("disable-gpu").
			Set("no-sandbox").
			Set("disable-dev-shm-usage").
			Set("disable-crash-reporter").
			Set("disable-breakpad").
			Set("no-first-run").
			Set("no-default-browser-check")
		u, err = l.Launch()
		if err != nil {
			return nil, fmt.Errorf("launch chrome (%s): %w", opts.BrowserBin, err)
		}
	}

	br := rod.New().ControlURL(u)
	if err := br.Connect(); err != nil {
		return nil, fmt.Errorf("connect chrome: %w", err)
	}
	return &Scraper{browser: br, log: opts.Logger, timeout: opts.Timeout, browserBin: opts.BrowserBin}, nil
}

// Close releases the browser process.
func (s *Scraper) Close() error {
	if s.browser == nil {
		return nil
	}
	return s.browser.Close()
}

// Search performs a search on zvg-portal.de and returns matching auctions.
func (s *Scraper) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	deadline := time.Now().Add(s.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	cctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	page, err := s.browser.Context(cctx).Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}
	defer page.Close()
	page = page.Context(cctx)

	if err := page.Navigate(searchURL); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}
	page.WaitLoad()
	time.Sleep(time.Second)

	if err := s.fillForm(page, q); err != nil {
		return nil, fmt.Errorf("fill form: %w", err)
	}

	if err := s.submitForm(page); err != nil {
		return nil, fmt.Errorf("submit: %w", err)
	}

	auctions := s.parseResults(page)
	stateOut := q.State
	if v, ok := StateValues[strings.ToUpper(q.State)]; ok {
		stateOut = v
	}
	return &SearchResult{
		Auctions:   auctions,
		TotalFound: len(auctions),
		State:      stateOut,
	}, nil
}

// fillForm populates the globe form fields.
func (s *Scraper) fillForm(page *rod.Page, q SearchQuery) error {
	// Bundesland
	if q.State != "" {
		val := StateValues[q.State]
		if val == "" {
			val = strings.ToLower(q.State)
		}
		page.Eval(fmt.Sprintf(`() => {
			const s = document.querySelector("select[name='land_abk']");
			if (s) { s.value = %q; s.dispatchEvent(new Event('change',{bubbles:true})); }
		}`, val))
		// Wait for Gericht cascade
		time.Sleep(2 * time.Second)
	}

	// Gericht
	if q.CourtID != "" {
		page.Eval(fmt.Sprintf(`() => {
			const s = document.querySelector("select[name='ger_id']");
			if (s) { s.value = %q; }
		}`, q.CourtID))
	}

	// Verfahrensart
	if q.ProcedureType != "" {
		page.Eval(fmt.Sprintf(`() => {
			const s = document.querySelector("select[name='art']");
			if (s) s.value = %q;
		}`, q.ProcedureType))
	}

	// Objektarten (multi-select obj_liste)
	if len(q.ObjectTypes) > 0 {
		page.Eval(fmt.Sprintf(`() => {
			const s = document.querySelector("select[name='obj_liste']");
			if (!s) return;
			const vals = %s;
			for (const opt of s.options) {
				opt.selected = vals.includes(opt.value);
			}
		}`, jsonStringArray(q.ObjectTypes)))
	}

	// PLZ
	if q.PostalCode != "" {
		page.Eval(fmt.Sprintf(`() => {
			const el = document.getElementById("plz");
			if (el) el.value = %q;
		}`, q.PostalCode))
	}

	// Ort
	if q.City != "" {
		page.Eval(fmt.Sprintf(`() => {
			const el = document.getElementById("ort");
			if (el) el.value = %q;
		}`, q.City))
	}

	// Straße
	if q.Street != "" {
		page.Eval(fmt.Sprintf(`() => {
			const el = document.querySelector("input[name='str']");
			if (el) el.value = %q;
		}`, q.Street))
	}

	// Aktenzeichen (az1 = main part)
	if q.CaseNumber != "" {
		page.Eval(fmt.Sprintf(`() => {
			const el = document.getElementById("az1");
			if (el) el.value = %q;
		}`, q.CaseNumber))
	}

	// Object free-text
	if q.ObjectText != "" {
		page.Eval(fmt.Sprintf(`() => {
			const el = document.getElementById("obj");
			if (el) el.value = %q;
		}`, q.ObjectText))
	}

	// Sort order (default: Termin = "2")
	sortVal := "2"
	if q.SortBy != "" {
		sortVal = q.SortBy
	}
	page.Eval(fmt.Sprintf(`() => {
		const s = document.querySelector("select[name='order_by']");
		if (s) s.value = %q;
	}`, sortVal))

	return nil
}

// submitForm submits the globe form and waits for navigation.
func (s *Scraper) submitForm(page *rod.Page) error {
	waitNav := page.MustWaitNavigation()
	_, err := page.Eval(`() => {
		const f = document.forms["globe"];
		if (!f) return "no form";
		f.onsubmit = null;
		f.submit();
		return "submitted";
	}`)
	if err != nil {
		return fmt.Errorf("eval submit: %w", err)
	}
	waitNav()
	time.Sleep(time.Second)
	return nil
}

// parseResults extracts auction entries from the results page.
// The page uses a flat <table><tr> structure where each row is a label-value pair.
// Groups of rows (Aktenzeichen, Amtsgericht, Objekt/Lage, Verkehrswert, Termin) form one auction.
func (s *Scraper) parseResults(page *rod.Page) []Auction {
	type row struct {
		label string
		value string
		href  string // set when label=="Aktenzeichen"
	}

	// Collect all rows from all result tables (skip pagination table at [0]).
	tables, err := page.Elements("table")
	if err != nil || len(tables) < 2 {
		return nil
	}

	var rows []row
	for _, table := range tables[1:] {
		trs, _ := table.Elements("tr")
		for _, tr := range trs {
			tds, _ := tr.Elements("td")
			if len(tds) < 2 {
				// separator row
				rows = append(rows, row{})
				continue
			}
			label, _ := tds[0].Text()
			value, _ := tds[1].Text()
			label = normalize(label)
			value = normalize(value)

			r := row{label: label, value: value}
			// Extract href from Aktenzeichen link
			if strings.Contains(label, "Aktenzeichen") {
				if a, err := tds[1].Element("a"); err == nil {
					href, _ := a.Attribute("href")
					if href != nil {
						r.href = *href
					}
					az, _ := a.Text()
					az = strings.ReplaceAll(az, "(Detailansicht)", "")
					r.value = normalize(az)
				}
			}
			rows = append(rows, r)
		}
	}

	// Group rows into auctions by empty separators.
	var auctions []Auction
	var current map[string]string
	var currentHref string

	flush := func() {
		if current == nil {
			return
		}
		az := current["Aktenzeichen"]
		if az == "" {
			current = nil
			currentHref = ""
			return
		}
		a := Auction{
			CaseNumber:        az,
			Court:             current["Amtsgericht"],
			ObjectDescription: current["Objekt/Lage"],
			DetailURL:         resolveURL(currentHref),
		}
		a.ZvgID = extractZvgID(currentHref)
		a.State = extractParam(currentHref, "land_abk")
		a.MarketValue = extractEuro(current["Verkehrswert in €"])
		a.LastUpdated = extractUpdated(current["_updated"])
		raw := current["Termin"]
		a.AuctionDateRaw = raw
		a.AuctionDate, a.AuctionTime = parseAuctionDate(raw)
		auctions = append(auctions, a)
		current = nil
		currentHref = ""
	}

	for _, r := range rows {
		if r.label == "" && r.value == "" {
			flush()
			continue
		}
		if strings.Contains(r.label, "Aktenzeichen") {
			flush()
			current = make(map[string]string)
			current["Aktenzeichen"] = r.value
			currentHref = r.href
		} else if current != nil {
			switch {
			case strings.Contains(r.label, "Amtsgericht"):
				current["Amtsgericht"] = r.value
			case strings.Contains(r.label, "Objekt"):
				current["Objekt/Lage"] = r.value
			case strings.Contains(r.label, "Verkehrswert"):
				if current["Verkehrswert in €"] == "" {
					current["Verkehrswert in €"] = r.value
				}
			case strings.Contains(r.label, "Termin"):
				current["Termin"] = r.value
			case strings.Contains(r.label, "letzte Aktualisierung") || strings.Contains(r.value, "letzte Aktualisierung"):
				current["_updated"] = r.value
			}
		}
	}
	flush()
	return auctions
}

// GetCourts returns the list of Amtsgerichte for a Bundesland.
func (s *Scraper) GetCourts(ctx context.Context, state string) ([]map[string]string, error) {
	page, err := s.browser.Context(ctx).Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, err
	}
	defer page.Close()
	if err := page.Navigate(searchURL); err != nil {
		return nil, err
	}
	page.WaitLoad()
	time.Sleep(time.Second)

	val := StateValues[state]
	if val == "" {
		val = strings.ToLower(state)
	}
	page.Eval(fmt.Sprintf(`() => {
		const s = document.querySelector("select[name='land_abk']");
		if (s) { s.value = %q; s.dispatchEvent(new Event('change',{bubbles:true})); }
	}`, val))
	time.Sleep(2 * time.Second)

	res, err := page.Eval(`() => {
		const s = document.querySelector("select[name='ger_id']");
		if (!s) return JSON.stringify([]);
		return JSON.stringify(Array.from(s.options)
			.filter(o => o.value !== "0")
			.map(o => ({id: o.value, name: o.text.trim()})));
	}`)
	if err != nil {
		return nil, err
	}

	var courts []map[string]string
	if err := res.Value.Unmarshal(&courts); err != nil {
		return nil, err
	}
	return courts, nil
}

// ---- helpers ----

var (
	reWhitespace = regexp.MustCompile(`\s+`)
	reEuro       = regexp.MustCompile(`[\d.,]+[\s\x{00A0}]*€`)
	reZvgID      = regexp.MustCompile(`zvg_id=(\d+)`)
	reParam      = regexp.MustCompile(`[?&]([^=]+)=([^&]*)`)
	// "Montag, 08. Juni 2026, 10:00 Uhr"
	reAuctionDate = regexp.MustCompile(`(\d{2})\.\s*(\w+)\s+(\d{4}),\s*(\d{2}:\d{2})`)
)

var germanMonths = map[string]string{
	"Januar": "01", "Februar": "02", "März": "03", "April": "04",
	"Mai": "05", "Juni": "06", "Juli": "07", "August": "08",
	"September": "09", "Oktober": "10", "November": "11", "Dezember": "12",
}

func normalize(s string) string {
	return strings.TrimSpace(reWhitespace.ReplaceAllString(s, " "))
}

func extractEuro(s string) string {
	m := reEuro.FindString(s)
	if m == "" {
		return s
	}
	return strings.TrimSpace(m)
}

func extractZvgID(href string) string {
	m := reZvgID.FindStringSubmatch(href)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func extractParam(href, param string) string {
	for _, m := range reParam.FindAllStringSubmatch(href, -1) {
		if len(m) >= 3 && m[1] == param {
			return m[2]
		}
	}
	return ""
}

func resolveURL(href string) string {
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "http") {
		return href
	}
	return baseURL + "/" + strings.TrimPrefix(href, "/")
}

func extractUpdated(s string) string {
	// "(letzte Aktualisierung 20-04-2026 11:18)"
	s = strings.TrimPrefix(s, "(letzte Aktualisierung ")
	s = strings.TrimSuffix(s, ")")
	return strings.TrimSpace(s)
}

func parseAuctionDate(raw string) (date, clockTime string) {
	m := reAuctionDate.FindStringSubmatch(raw)
	if len(m) < 5 {
		return "", ""
	}
	day, month, year, t := m[1], m[2], m[3], m[4]
	mo := germanMonths[month]
	if mo == "" {
		return "", ""
	}
	return fmt.Sprintf("%s-%s-%s", year, mo, day), t
}

func jsonStringArray(vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
