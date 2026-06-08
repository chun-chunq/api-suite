// Package sanctions provides an in-memory index of the EU Consolidated
// Sanctions List (FSF) for fast name-based searches.
//
// Data source: https://data.europa.eu/data/datasets/consolidated-list-of-persons-groups-and-entities-subject-to-eu-financial-sanctions
// License: EU Open Data / CC BY 4.0
package sanctions

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// EU FSF XML download URL — the official consolidated list.
// This is a public download, no authentication required.
const listURL = "https://webgate.ec.europa.eu/fsd/fsf/public/files/xmlFullSanctionsList_1_1/content?token=dG9rZW4tMjAxNw"

// fallback URL if the first fails
const listURLFallback = "https://webgate.ec.europa.eu/fsd/fsf/public/files/xmlFullSanctionsList/content?token=dG9rZW4tMjAxNw"

// Index is a thread-safe in-memory sanctions index.
type Index struct {
	mu       sync.RWMutex
	entities []Entity
	// normalized lower-case name → entity indices for fast lookup
	nameIdx  map[string][]int
	listDate string
	loaded   atomic.Bool

	refreshInterval time.Duration
	log             zerolog.Logger
	nextRefresh     time.Time
}

// New creates a new Index and starts a background refresh loop.
func New(refreshInterval time.Duration, log zerolog.Logger) *Index {
	idx := &Index{
		nameIdx:         make(map[string][]int),
		refreshInterval: refreshInterval,
		log:             log,
	}
	// Load synchronously on start so the API is ready immediately.
	if err := idx.refresh(); err != nil {
		log.Error().Err(err).Msg("initial sanctions list load failed — will retry in background")
	}
	go idx.refreshLoop()
	return idx
}

// Search searches for sanctioned entities matching the query string.
// Matches on any name variant (whole name, first+last, aliases).
// Returns up to maxResults entities.
func (idx *Index) Search(query string, maxResults int) []Entity {
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 50
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	norm := normalizeName(query)

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	seen := make(map[int]bool)
	var results []Entity

	// 1. Exact / contains match on normalized name index
	for key, indices := range idx.nameIdx {
		if strings.Contains(key, norm) {
			for _, i := range indices {
				if !seen[i] {
					seen[i] = true
					results = append(results, idx.entities[i])
					if len(results) >= maxResults {
						return results
					}
				}
			}
		}
	}

	return results
}

// Check returns all exact-ish matches for a name (for yes/no compliance checks).
// A "match" means the normalized query is contained in any name variant.
func (idx *Index) Check(query string) []Entity {
	return idx.Search(query, 10)
}

// Status returns the current state of the index.
func (idx *Index) Status() Status {
	idx.mu.RLock()
	count := len(idx.entities)
	date := idx.listDate
	next := idx.nextRefresh
	idx.mu.RUnlock()
	return Status{
		Loaded:      idx.loaded.Load(),
		Count:       count,
		ListDate:    date,
		NextRefresh: next.UTC().Format(time.RFC3339),
	}
}

// ListDate returns the date of the currently loaded list.
func (idx *Index) ListDate() string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.listDate
}

// refreshLoop re-downloads and re-indexes the list periodically.
func (idx *Index) refreshLoop() {
	for {
		next := time.Now().Add(idx.refreshInterval)
		idx.mu.Lock()
		idx.nextRefresh = next
		idx.mu.Unlock()

		time.Sleep(idx.refreshInterval)
		if err := idx.refresh(); err != nil {
			idx.log.Error().Err(err).Msg("sanctions list refresh failed")
		} else {
			idx.log.Info().Str("listDate", idx.ListDate()).Msg("sanctions list refreshed")
		}
	}
}

// refresh downloads the latest EU FSF XML and rebuilds the index.
func (idx *Index) refresh() error {
	idx.log.Info().Str("url", listURL).Msg("downloading EU sanctions list")

	data, listDate, err := download(listURL)
	if err != nil {
		idx.log.Warn().Err(err).Msg("primary URL failed, trying fallback")
		data, listDate, err = download(listURLFallback)
		if err != nil {
			return fmt.Errorf("both URLs failed: %w", err)
		}
	}

	entities, err := parseXML(data)
	if err != nil {
		return fmt.Errorf("parse sanctions XML: %w", err)
	}

	// Build name index
	nameIdx := make(map[string][]int, len(entities)*3)
	for i, e := range entities {
		for _, n := range e.Names {
			key := normalizeName(n.WholeName)
			if key != "" {
				nameIdx[key] = append(nameIdx[key], i)
			}
			// Also index individual tokens for partial matching
			for _, token := range strings.Fields(key) {
				if len(token) >= 3 {
					nameIdx[token] = appendUnique(nameIdx[token], i)
				}
			}
		}
	}

	idx.mu.Lock()
	idx.entities = entities
	idx.nameIdx = nameIdx
	idx.listDate = listDate
	idx.mu.Unlock()
	idx.loaded.Store(true)

	idx.log.Info().
		Int("entities", len(entities)).
		Str("listDate", listDate).
		Msg("sanctions list loaded and indexed")
	return nil
}

// download fetches the XML list, handling gzip transparently.
func download(url string) ([]byte, string, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	var reader io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") ||
		strings.HasSuffix(url, ".gz") {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, "", err
		}
		defer gr.Close()
		reader = gr
	}

	data, err := io.ReadAll(io.LimitReader(reader, 50<<20)) // max 50 MB
	if err != nil {
		return nil, "", err
	}

	// Extract generation date from XML header if present
	listDate := time.Now().UTC().Format("2006-01-02")
	if i := strings.Index(string(data[:min(500, len(data))]), "generationDate="); i >= 0 {
		sub := string(data[i+16:])
		if end := strings.IndexByte(sub, '"'); end > 0 {
			listDate = sub[:end]
		}
	}

	return data, listDate, nil
}

// EU FSF XML schema (version 1.1)
type xmlExport struct {
	XMLName         xml.Name          `xml:"export"`
	GenerationDate  string            `xml:"generationDate,attr"`
	SanctionEntities []xmlSanctionEntity `xml:"sanctionEntity"`
}

type xmlSanctionEntity struct {
	LogicalID   string         `xml:"logicalId,attr"`
	SubjectType xmlSubjectType `xml:"subjectType"`
	NameAliases []xmlNameAlias `xml:"nameAlias"`
	Addresses   []xmlAddress   `xml:"address"`
	BirthDates  []xmlBirthDate `xml:"birthdate"`
	Citizenships []xmlCitizenship `xml:"citizenship"`
	Passports   []xmlPassport  `xml:"identification"`
	Remark      string         `xml:"remark"`
	Regulation  xmlRegulation  `xml:"regulation"`
}

type xmlSubjectType struct {
	Code string `xml:"classificationCode,attr"`
}

type xmlNameAlias struct {
	FirstName  string `xml:"firstName,attr"`
	MiddleName string `xml:"middleName,attr"`
	LastName   string `xml:"lastName,attr"`
	WholeName  string `xml:"wholeName,attr"`
}

type xmlAddress struct {
	Street  string `xml:"street,attr"`
	City    string `xml:"city,attr"`
	Country string `xml:"countryDescription,attr"`
}

type xmlBirthDate struct {
	Date string `xml:"birthdate,attr"`
	Year string `xml:"year,attr"`
}

type xmlCitizenship struct {
	Country string `xml:"countryDescription,attr"`
}

type xmlPassport struct {
	Number string `xml:"number,attr"`
	Type   string `xml:"identificationTypeDescription,attr"`
}

type xmlRegulation struct {
	Programme      string `xml:"programme,attr"`
	NumberTitle    string `xml:"numberTitle,attr"`
	PublicationURL string `xml:"publicationUrl,attr"`
}

// parseXML parses the EU FSF XML into Entity slices.
func parseXML(data []byte) ([]Entity, error) {
	var export xmlExport
	if err := xml.Unmarshal(data, &export); err != nil {
		return nil, err
	}

	entities := make([]Entity, 0, len(export.SanctionEntities))
	for _, xe := range export.SanctionEntities {
		e := Entity{
			ID:          xe.LogicalID,
			SubjectType: mapSubjectType(xe.SubjectType.Code),
			Remark:      strings.TrimSpace(xe.Remark),
			Regulation:  strings.TrimSpace(xe.Regulation.NumberTitle),
			Programme:   strings.TrimSpace(xe.Regulation.Programme),
		}

		// Names
		for _, na := range xe.NameAliases {
			whole := strings.TrimSpace(na.WholeName)
			if whole == "" {
				parts := []string{na.FirstName, na.MiddleName, na.LastName}
				var nonEmpty []string
				for _, p := range parts {
					if p = strings.TrimSpace(p); p != "" {
						nonEmpty = append(nonEmpty, p)
					}
				}
				whole = strings.Join(nonEmpty, " ")
			}
			if whole == "" {
				continue
			}
			e.Names = append(e.Names, Name{
				FirstName:  strings.TrimSpace(na.FirstName),
				MiddleName: strings.TrimSpace(na.MiddleName),
				LastName:   strings.TrimSpace(na.LastName),
				WholeName:  whole,
			})
		}
		if len(e.Names) == 0 {
			continue // skip entries without names
		}

		// Addresses
		for _, a := range xe.Addresses {
			parts := []string{a.Street, a.City, a.Country}
			var nonEmpty []string
			for _, p := range parts {
				if p = strings.TrimSpace(p); p != "" {
					nonEmpty = append(nonEmpty, p)
				}
			}
			if addr := strings.Join(nonEmpty, ", "); addr != "" {
				e.Addresses = append(e.Addresses, addr)
			}
		}

		// Birth dates
		for _, bd := range xe.BirthDates {
			if d := strings.TrimSpace(bd.Date); d != "" {
				e.BirthDates = append(e.BirthDates, d)
			} else if y := strings.TrimSpace(bd.Year); y != "" {
				e.BirthDates = append(e.BirthDates, y)
			}
		}

		// Nationalities
		for _, c := range xe.Citizenships {
			if country := strings.TrimSpace(c.Country); country != "" {
				e.Nationalities = append(e.Nationalities, country)
			}
		}

		// Passports / IDs
		for _, p := range xe.Passports {
			if num := strings.TrimSpace(p.Number); num != "" {
				e.Passports = append(e.Passports, num)
			}
		}

		entities = append(entities, e)
	}

	return entities, nil
}

func mapSubjectType(code string) string {
	switch strings.ToLower(code) {
	case "p":
		return "person"
	case "e":
		return "entity"
	case "s":
		return "ship"
	default:
		return "other"
	}
}

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Remove common punctuation that differs between name variants
	s = strings.NewReplacer(
		".", "", ",", "", "-", " ", "_", " ", "'", "", "\"", "",
	).Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func appendUnique(slice []int, val int) []int {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
