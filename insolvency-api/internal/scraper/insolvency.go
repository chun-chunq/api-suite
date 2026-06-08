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
	baseURL    = "https://neu.insolvenzbekanntmachungen.de"
	searchPath = "/ap/suche.jsf"
	searchURL  = baseURL + searchPath

	defaultBrowserBin = `C:\Program Files\Google\Chrome\Application\chrome.exe`
	linuxChromeBin    = `/usr/bin/chromium`
	defaultTimeout    = 60 * time.Second

	dateLayoutDE  = "02.01.2006"
	dateLayoutISO = "2006-01-02"

	// Confirmed element ids from live DOM inspection (Jakarta Faces 4.x).
	selForm     = `frm_suche`
	selName     = `input[id="frm_suche:litx_firmaNachName:text"]`
	selDateFrom = `input[id="frm_suche:ldi_datumVon:datumHtml5"]`
	selDateTo   = `input[id="frm_suche:ldi_datumBis:datumHtml5"]`
	selRegNum   = `input[id="frm_suche:ireg_registereintrag:itx_registernummer"]`
	selVorname  = `input[id="frm_suche:litx_vorname:text"]`
	selCity     = `input[id="frm_suche:litx_sitzWohnsitz:text"]`
	selSubmit   = `input[id="frm_suche:cbt_suchen"]`
	idSubmit    = `frm_suche:cbt_suchen`

	// Select dropdown element ids (set via JS).
	idWildcard   = `frm_suche:lsom_wildcard:lsom`
	idBundesland = `frm_suche:lsom_bundesland:lsom`
	idGericht    = `frm_suche:lsom_gericht:lsom`
	idGegenstand = `frm_suche:lsom_gegenstand:lsom`
)

// matchModeValue maps the public match mode to the wildcard dropdown value.
// Default is "1" (beginnt mit / starts with).
func matchModeValue(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "exact":
		return "0"
	case "contains":
		return "2"
	case "startswith", "":
		return "1"
	default:
		return "1"
	}
}

// Scraper drives a headless Chrome instance (via go-rod) to perform searches on
// the German insolvency portal, which runs Jakarta Faces 4.x and does not work
// reliably with plain net/http POSTs.
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

// New launches a headless Chrome browser and returns a ready Scraper. The caller
// must call Close to release the browser process.
func New(opts Options) (*Scraper, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if strings.TrimSpace(opts.BrowserBin) == "" {
		// Prefer CHROME_BIN env var so the same binary works on Linux (Docker)
		// and Windows (dev) without code changes.
		if env := os.Getenv("CHROME_BIN"); env != "" {
			opts.BrowserBin = env
		} else if _, err := os.Stat(linuxChromeBin); err == nil {
			opts.BrowserBin = linuxChromeBin
		} else {
			opts.BrowserBin = defaultBrowserBin
		}
	}

	// Leakless(false) avoids the bundled leakless.exe helper that Windows
	// Defender frequently quarantines.
	l := launcher.New().
		Bin(opts.BrowserBin).
		Leakless(false).
		Headless(true).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage")

	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch chrome (%s): %w", opts.BrowserBin, err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connect to chrome: %w", err)
	}

	return &Scraper{
		browser:    browser,
		log:        opts.Logger,
		timeout:    opts.Timeout,
		browserBin: opts.BrowserBin,
	}, nil
}

// Close releases the browser process.
func (s *Scraper) Close() error {
	if s.browser == nil {
		return nil
	}
	return s.browser.Close()
}

// Search runs a single search against the portal and parses the result table.
func (s *Scraper) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	// Bound the whole operation by the configured timeout, while still honouring
	// any earlier deadline carried by ctx.
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
	defer func() { _ = page.Close() }()

	page = page.Context(cctx).Timeout(s.timeout)

	if err := page.Navigate(searchURL); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("wait load: %w", err)
	}

	// Wait for the search form to be present.
	if _, err := page.Element(selSubmit); err != nil {
		return nil, fmt.Errorf("search form not found: %w", err)
	}

	if err := s.fillForm(page, q); err != nil {
		return nil, fmt.Errorf("fill form: %w", err)
	}

	if err := s.submitAndWait(page); err != nil {
		return nil, fmt.Errorf("submit search: %w", err)
	}

	records := s.parseResults(page)

	result := &SearchResult{
		Records:    records,
		Totalfound: len(records),
		Pages:      1,
	}
	return result, nil
}

// SearchByHRB looks up insolvency announcements for a commercial register number.
func (s *Scraper) SearchByHRB(ctx context.Context, hrb, state string) (*SearchResult, error) {
	q := SearchQuery{
		State:          state,
		RegisterType:   "HRB",
		RegisterNumber: hrb,
		DateFrom:       time.Now().AddDate(-5, 0, 0),
		DateTo:         time.Now(),
	}
	return s.Search(ctx, q)
}

// SearchByName searches for a company/last name and optional first name and
// city, using the "starts with" match mode over the last five years.
func (s *Scraper) SearchByName(ctx context.Context, lastName, firstName, city string) (*SearchResult, error) {
	q := SearchQuery{
		Name:      lastName,
		FirstName: firstName,
		City:      city,
		MatchMode: "startswith",
		DateFrom:  time.Now().AddDate(-5, 0, 0),
		DateTo:    time.Now(),
	}
	return s.Search(ctx, q)
}

// fillForm populates the visible search inputs. Each set is best-effort: a
// missing optional input must not abort the search.
func (s *Scraper) fillForm(page *rod.Page, q SearchQuery) error {
	// Match mode (wildcard) — set value silently (no change event) to avoid
	// triggering any JSF AJAX listener that could reset the form.
	if err := setSelectSilent(page, idWildcard, matchModeValue(q.MatchMode)); err != nil {
		s.log.Debug().Err(err).Msg("wildcard dropdown not settable")
	}

	// Bundesland — triggers an AJAX update of the Gericht dropdown, so wait
	// afterwards before touching dependent fields.
	if val := stateDropdownValue(q.State); val != "" {
		if err := setSelect(page, idBundesland, val); err != nil {
			s.log.Debug().Err(err).Msg("bundesland dropdown not settable")
		} else {
			// The Bundesland change fires an AJAX request that repopulates the
			// Gericht dropdown; give it time to settle.
			wait := page.WaitRequestIdle(2*time.Second, nil, nil, nil)
			_ = rod.Try(wait)
			time.Sleep(time.Second)
		}
	}

	// Gericht — depends on the Bundesland that was just selected.
	if court := strings.TrimSpace(q.Court); court != "" {
		if err := setSelect(page, idGericht, court); err != nil {
			s.log.Debug().Err(err).Msg("gericht dropdown not settable")
		}
	}

	if name := strings.TrimSpace(q.Name); name != "" {
		if err := setInput(page, selName, name); err != nil {
			return fmt.Errorf("set name: %w", err)
		}
	}
	if fn := strings.TrimSpace(q.FirstName); fn != "" {
		if err := setInput(page, selVorname, fn); err != nil {
			s.log.Debug().Err(err).Msg("vorname field not fillable")
		}
	}
	if city := strings.TrimSpace(q.City); city != "" {
		if err := setInput(page, selCity, city); err != nil {
			s.log.Debug().Err(err).Msg("city field not fillable")
		}
	}
	if subj := subjectDropdownValue(q.Subject); subj != "" {
		if err := setSelect(page, idGegenstand, subj); err != nil {
			s.log.Debug().Err(err).Msg("gegenstand dropdown not settable")
		}
	}
	if !q.DateFrom.IsZero() {
		_ = setDateInput(page, selDateFrom, q.DateFrom.Format(dateLayoutISO))
	}
	if !q.DateTo.IsZero() {
		_ = setDateInput(page, selDateTo, q.DateTo.Format(dateLayoutISO))
	}
	if rn := strings.TrimSpace(q.RegisterNumber); rn != "" {
		if err := setInput(page, selRegNum, rn); err != nil {
			s.log.Debug().Err(err).Msg("register number field not fillable")
		}
	}
	return nil
}

// submitAndWait clicks the search button and waits for the results table or
// the "Keine Treffer" indicator to appear.
func (s *Scraper) submitAndWait(page *rod.Page) error {
	_, err := page.Eval(fmt.Sprintf(
		`() => { const b = document.getElementById(%q); if (b) b.click(); }`, idSubmit))
	if err != nil {
		return fmt.Errorf("click submit: %w", err)
	}

	// Wait for either the results table or the no-results indicator.
	err = rod.Try(func() {
		page.Race().
			Element(`table#tbl_ergebnis`).MustHandle(func(*rod.Element) {}).
			Element(`span#otx_keineTreffer`).MustHandle(func(*rod.Element) {}).
			MustDo()
	})
	if err != nil {
		s.log.Debug().Err(err).Msg("results marker timeout; proceeding to parse")
	}
	return nil
}

// parseResults extracts records from the confirmed results table #tbl_ergebnis.
// Each row contains labelled spans for date, case number, court, debtor and location.
func (s *Scraper) parseResults(page *rod.Page) []Record {
	rows, err := page.Elements(`table#tbl_ergebnis tbody tr`)
	if err != nil || len(rows) == 0 {
		return nil
	}

	var records []Record
	for _, row := range rows {
		rec := s.parseResultRow(row)
		if rec.DebtorName == "" && rec.Aktenzeichen == "" {
			continue
		}
		records = append(records, rec)
	}
	return records
}

// parseResultRow extracts one Record from a result table row using confirmed span IDs.
// Span IDs follow the pattern tbl_ergebnis:N:<field>.
func (s *Scraper) parseResultRow(row *rod.Element) Record {
	spanText := func(titleAttr string) string {
		el, err := row.Element(fmt.Sprintf(`span[title=%q]`, titleAttr))
		if err != nil || el == nil {
			return ""
		}
		t, _ := el.Text()
		return normalize(t)
	}

	// Date span title contains a soft-hyphen (U+00AD) in the portal HTML.
	// Use a partial attribute selector to avoid encoding issues.
	dateStr := func() string {
		el, err := row.Element(`span[title*="lichungsdatum"]`)
		if err != nil || el == nil {
			return ""
		}
		t, _ := el.Text()
		return normalize(t)
	}()

	rec := Record{
		Aktenzeichen:    spanText("aktuelles Aktenzeichen"),
		Court:           spanText("Gericht"),
		DebtorName:      spanText("Name, Vorname/Bezeichnung"),
		PublicationDate: parseGermanDate(dateStr),
	}

	// Location (Sitz/Wohnsitz) → city field
	loc := spanText("Sitz / Wohnsitz")
	if loc != "" {
		rec.DebtorAddress.City = loc
	}

	// Register field: "HRB 1234 Amtsgericht München"
	reg := spanText("Register")
	if reg != "" {
		if m := reRegister.FindStringSubmatch(reg); len(m) >= 3 {
			rec.RegisterType = m[1]
			rec.RegisterNumber = m[2]
		}
		if rec.RegisterCourt == "" && rec.Court != "" {
			rec.RegisterCourt = rec.Court
		}
	}

	rec.SourceURL = baseURL + "/ap/ergebnis.jsf"
	return rec
}

// ---- pure parsing helpers ----

var (
	reWhitespace = regexp.MustCompile(`\s+`)
	reGermanDate = regexp.MustCompile(`\b(\d{2}\.\d{2}\.\d{4})\b`)
	reRegister   = regexp.MustCompile(`(HRB|HRA|VR|GnR|PR|GsR)\s*(\d+)`)
)

func normalize(s string) string {
	return strings.TrimSpace(reWhitespace.ReplaceAllString(s, " "))
}

func parseGermanDate(s string) string {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(dateLayoutDE, s); err == nil {
		return t.Format(dateLayoutISO)
	}
	if m := reGermanDate.FindString(s); m != "" {
		if t, err := time.Parse(dateLayoutDE, m); err == nil {
			return t.Format(dateLayoutISO)
		}
	}
	return ""
}

// ---- rod input helpers ----

// setInput clears and types into a text input identified by selector.
func setInput(page *rod.Page, selector, value string) error {
	el, err := page.Element(selector)
	if err != nil {
		return err
	}
	if err := el.SelectAllText(); err != nil {
		return err
	}
	if err := el.Input(""); err != nil { // clears the selection
		return err
	}
	return el.Input(value)
}

// setSelect sets a <select> value and fires a bubbling change event so JSF/Mojarra
// AJAX listeners (e.g. Bundesland → Gericht cascade) fire.
func setSelect(page *rod.Page, fieldID, value string) error {
	_, err := page.Eval(fmt.Sprintf(`() => {
		const s = document.getElementById(%q);
		if (!s) return false;
		s.value = %q;
		s.dispatchEvent(new Event('change', { bubbles: true }));
		return true;
	}`, fieldID, value))
	return err
}

// setSelectSilent sets a <select> value without dispatching any event.
// Use this for dropdowns that must not trigger AJAX side-effects.
func setSelectSilent(page *rod.Page, fieldID, value string) error {
	_, err := page.Eval(fmt.Sprintf(`() => {
		const s = document.getElementById(%q);
		if (!s) return false;
		s.value = %q;
		return true;
	}`, fieldID, value))
	return err
}

// subjectDropdownValue maps the public Subject keys to the Gegenstand dropdown
// value. Empty / "all" means no filter. Unknown values are passed through
// verbatim so callers can supply a raw portal value if the mapping is missing.
func subjectDropdownValue(subject string) string {
	switch strings.ToLower(strings.TrimSpace(subject)) {
	case "", "all", "alle":
		return ""
	case "opening", "eroeffnung", "eröffnung":
		return "Eröffnungen"
	case "rejection", "abweisung":
		return "Abweisungen mangels Masse"
	case "termination", "aufhebung", "einstellung":
		return "Aufhebungen"
	case "security", "sicherung", "sicherungsmassnahmen":
		return "Sicherungsmaßnahmen"
	default:
		return strings.TrimSpace(subject)
	}
}

// setDateInput sets an HTML5 date input. Typing into native date inputs is
// unreliable across locales, so we set the value via JS and dispatch the events
// the JSF client listeners expect.
func setDateInput(page *rod.Page, selector, isoValue string) error {
	el, err := page.Element(selector)
	if err != nil {
		return err
	}
	_, err = el.Eval(`(v) => {
		this.value = v;
		this.dispatchEvent(new Event('input', { bubbles: true }));
		this.dispatchEvent(new Event('change', { bubbles: true }));
	}`, isoValue)
	return err
}
