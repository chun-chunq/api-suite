package scraper

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

const (
	// maskURL is the advanced search mask of handelsregister.de.
	maskURL = "https://www.handelsregister.de/rp_web/erweitertesuche.xhtml"
	// legacyMaskURL is kept for reference; the portal migrated from mask.do.
	legacyMaskURL = "https://www.handelsregister.de/rp_web/mask.do"
)

// ErrNotFound is returned when the registry yields no matching company.
var ErrNotFound = errors.New("scraper: company not found")

// Scraper drives a headless Chrome instance against handelsregister.de.
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

// New launches a headless browser and returns a ready Scraper.
// Call Close when finished to release the browser process.
func New(opts Options) (*Scraper, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 45 * time.Second
	}

	l := launcher.New().
		Headless(true).
		Leakless(false).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage")

	if opts.BrowserBin != "" {
		l = l.Bin(opts.BrowserBin)
	}

	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("scraper: launch browser: %w", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("scraper: connect browser: %w", err)
	}

	return &Scraper{
		browser:    browser,
		log:        opts.Logger,
		timeout:    opts.Timeout,
		browserBin: opts.BrowserBin,
	}, nil
}

// Close shuts down the underlying browser.
func (s *Scraper) Close() error {
	if s.browser == nil {
		return nil
	}
	return s.browser.Close()
}

// GetByHRB fetches a single company by HRB number and Bundesland (state).
func (s *Scraper) GetByHRB(ctx context.Context, hrb, state string) (*CompanyData, error) {
	hrb = strings.TrimSpace(hrb)
	if hrb == "" {
		return nil, errors.New("scraper: empty HRB number")
	}

	results, err := s.search(ctx, searchQuery{
		term:        hrb,
		registerNum: hrb,
		state:       state,
	})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNotFound
	}

	// The first hit is the strongest match for an exact register number.
	return s.expandResult(ctx, results[0], state)
}

// Search performs a fuzzy company-name search and returns lightweight hits.
func (s *Scraper) Search(ctx context.Context, name string) ([]SearchResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("scraper: empty search name")
	}
	return s.search(ctx, searchQuery{term: name})
}

type searchQuery struct {
	term        string
	registerNum string
	state       string
}

// search submits the advanced search mask and parses the result table.
func (s *Scraper) search(ctx context.Context, q searchQuery) ([]SearchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	page, err := s.browser.Context(ctx).Page(proto.TargetCreateTarget{URL: "https://www.handelsregister.de/rp_web/welcome.xhtml"})
	if err != nil {
		return nil, fmt.Errorf("scraper: open homepage: %w", err)
	}
	defer func() {
		if cerr := page.Close(); cerr != nil {
			s.log.Warn().Err(cerr).Msg("closing scrape page")
		}
	}()

	page = page.Context(ctx)

	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("scraper: wait homepage load: %w", err)
	}

	// Accept cookie banner if present.
	if cookieBtn, err := page.Timeout(5 * time.Second).Element(`a[id$="j_idt17"], .cookie-btn`); err == nil {
		_ = cookieBtn.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(500 * time.Millisecond)
	}

	// Navigate via the sidebar link — direct URL navigation returns 404 without proper JSF session.
	advLink, err := page.Timeout(8 * time.Second).Element(`[id="naviForm:erweiterteSucheLink"]`)
	if err != nil {
		return nil, fmt.Errorf("scraper: advanced search link not found: %w", err)
	}
	if err := advLink.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, fmt.Errorf("scraper: click advanced search: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("scraper: wait mask load: %w", err)
	}
	time.Sleep(2 * time.Second)

	// Debug: log current page URL
	if info, err := page.Info(); err == nil {
		s.log.Debug().Str("url", info.URL).Str("title", info.Title).Msg("on advanced search page")
	}

	// Fill the search term. The advanced mask uses a JSF form; the company-name
	// field id is stable across the portal's PrimeFaces layout.
	if err := s.fillSearchTerm(page, q.term); err != nil {
		return nil, err
	}

	// Optionally constrain by Bundesland.
	if q.state != "" {
		if err := s.selectState(page, q.state); err != nil {
			s.log.Debug().Err(err).Str("state", q.state).Msg("state filter not applied")
		}
	}

	if err := s.submitSearch(page); err != nil {
		return nil, err
	}

	results, err := s.parseResults(page, q.state)
	if err != nil {
		return nil, err
	}

	s.log.Info().
		Str("term", q.term).
		Int("hits", len(results)).
		Msg("search completed")

	return results, nil
}

func (s *Scraper) fillSearchTerm(page *rod.Page, term string) error {
	// The search field is a <textarea id="form:schlagwoerter"> (not an input).
	selectors := []string{
		`[id="form:schlagwoerter"]`,
		`textarea[name="form:schlagwoerter"]`,
		`textarea[id$="schlagwoerter"]`,
	}
	for _, sel := range selectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err != nil {
			continue
		}
		if err := el.Input(term); err != nil {
			return fmt.Errorf("scraper: input search term: %w", err)
		}
		return nil
	}
	return errors.New("scraper: search input field not found")
}

func (s *Scraper) selectState(page *rod.Page, state string) error {
	// Bundesland is a checkbox per state: id="form:{StateName}_input"
	// State name must match exactly as shown in the portal (e.g., "Bayern", "Berlin").
	sel := fmt.Sprintf(`[id="form:%s_input"]`, state)
	cb, err := page.Timeout(5 * time.Second).Element(sel)
	if err != nil {
		return fmt.Errorf("state checkbox not found for %q: %w", state, err)
	}
	checked, _ := cb.Property("checked")
	if !checked.Bool() {
		return cb.Click(proto.InputMouseButtonLeft, 1)
	}
	return nil
}

func (s *Scraper) submitSearch(page *rod.Page) error {
	// Use rod native click — confirmed working from live DOM inspection.
	// Button id="form:btnSuche" is stable on the advanced search page.
	el, err := page.Timeout(10 * time.Second).Element(`[id="form:btnSuche"]`)
	if err != nil {
		return fmt.Errorf("scraper: search button not found: %w", err)
	}
	// Scroll button into view before clicking.
	el.ScrollIntoView()
	time.Sleep(300 * time.Millisecond)
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("scraper: click search button: %w", err)
	}
	return nil
}

var (
	reRegister = regexp.MustCompile(`(HR[AB])\s*([0-9]+)`)
	reWS       = regexp.MustCompile(`\s+`)
)

// parseResults scrapes the results grid into SearchResult slices.
func (s *Scraper) parseResults(page *rod.Page, state string) ([]SearchResult, error) {
	// The result rows live in a PrimeFaces datatable. No rows means no match.
	// Results navigate to sucheErgebnisse/welcome.xhtml — wait for navigation.
	// Poll URL for up to 15 seconds.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		info, err := page.Info()
		if err == nil && strings.Contains(info.URL, "sucheErgebnisse") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	info, _ := page.Info()
	if !strings.Contains(info.URL, "sucheErgebnisse") {
		return []SearchResult{}, nil
	}
	time.Sleep(1 * time.Second)

	// Each result is a pair of rows with class ui-datatable-even/odd.
	// The first row contains: State, Court, Type, RegisterNumber, OldCourt, RegisteredOffice, Status
	// The second row (ui-panelgrid-odd) contains: CompanyName, City, Status
	// We parse the datatable rows (even/odd alternate per result).
	rows, err := page.Timeout(8 * time.Second).Elements(`tr.ui-widget-content.ui-datatable-even, tr.ui-widget-content.ui-datatable-odd`)
	if err != nil || len(rows) == 0 {
		return []SearchResult{}, nil
	}

	out := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		// Each datatable row has two child tr elements:
		// - ui-panelgrid-even borderBottom1: State, Court, RegisterType, RegisterNumber
		// - ui-panelgrid-odd: CompanyName, City, Status
		headerRow, err := row.Element(`tr.borderBottom1`)
		if err != nil {
			continue
		}
		detailRow, err := row.Element(`tr.ui-panelgrid-odd`)
		if err != nil {
			continue
		}

		headerText, _ := headerRow.Text()
		detailText, _ := detailRow.Text()
		headerText = normalizeWS(headerText)
		detailText = normalizeWS(detailText)

		res := SearchResult{State: state}

		// Header: "{State} District court {CourtCity} {RegisterType} {RegisterNumber}"
		if m := reRegister.FindStringSubmatch(headerText); m != nil {
			res.HRBNummer = m[1] + " " + m[2]
		}
		for _, marker := range []string{"District court ", "Amtsgericht "} {
			if idx := strings.Index(headerText, marker); idx >= 0 {
				tail := headerText[idx+len(marker):]
				// Court city is up to the register type keyword
				for _, regType := range []string{" HRB", " HRA", " VR", " GnR", " PR"} {
					if ridx := strings.Index(tail, regType); ridx > 0 {
						res.Amtsgericht = normalizeWS(tail[:ridx])
						break
					}
				}
				if res.Amtsgericht == "" {
					res.Amtsgericht = normalizeWS(firstSegment(tail))
				}
				break
			}
		}

		// Detail: "CompanyName\tCity\tStatus\t..."
		// The detail row may also have a History section — take only first tab-segment.
		detailParts := strings.SplitN(detailText, "\t", 3)
		if len(detailParts) > 0 {
			res.Firmenname = strings.Trim(normalizeWS(detailParts[0]), `"`)
		}
		if len(detailParts) > 1 {
			res.Sitz = normalizeWS(detailParts[1])
		}

		if res.Firmenname != "" {
			out = append(out, res)
		}
	}

	return out, nil
}

// expandResult turns a search hit into a full CompanyData record. The portal
// requires opening the document/AD (Aktueller Abdruck) view for full details;
// here we assemble what is reliably visible plus the hit metadata.
func (s *Scraper) expandResult(ctx context.Context, hit SearchResult, state string) (*CompanyData, error) {
	register, num := splitRegister(hit.HRBNummer)

	data := &CompanyData{
		Firmenname:  hit.Firmenname,
		Rechtsform:  guessRechtsform(hit.Firmenname),
		HRBNummer:   hit.HRBNummer,
		Register:    register,
		Amtsgericht: hit.Amtsgericht,
		State:       state,
		Sitz:        Address{City: hit.Sitz, Country: "Deutschland"},
		Status:      StatusActive,
		ScrapedAt:   time.Now().UTC(),
		SourceURL:   maskURL,
	}
	_ = num

	return data, nil
}

// --- helpers ---

func textOf(el *rod.Element) string {
	t, err := el.Text()
	if err != nil {
		return ""
	}
	return t
}

func normalizeWS(s string) string {
	return strings.TrimSpace(reWS.ReplaceAllString(s, " "))
}

func firstSegment(s string) string {
	for _, sep := range []string{"\n", "  ", " - "} {
		if i := strings.Index(s, sep); i > 0 {
			return s[:i]
		}
	}
	return s
}

func splitRegister(hrb string) (register, num string) {
	if m := reRegister.FindStringSubmatch(hrb); m != nil {
		return m[1], m[2]
	}
	return "", hrb
}

// guessRechtsform infers the legal form from the company name suffix.
func guessRechtsform(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "gmbh & co. kg"), strings.Contains(n, "gmbh & co kg"):
		return "GmbH & Co. KG"
	case strings.Contains(n, "gmbh"):
		return "GmbH"
	case strings.Contains(n, " ag"), strings.HasSuffix(n, "ag"):
		return "AG"
	case strings.Contains(n, " kg"):
		return "KG"
	case strings.Contains(n, " ohg"):
		return "OHG"
	case strings.Contains(n, " ug"), strings.Contains(n, "ug (haftungsbeschränkt)"):
		return "UG (haftungsbeschränkt)"
	case strings.Contains(n, " se"):
		return "SE"
	case strings.Contains(n, " e.v."), strings.Contains(n, " ev"):
		return "e.V."
	default:
		return ""
	}
}
