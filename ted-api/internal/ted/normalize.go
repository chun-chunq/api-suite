package ted

import (
	"strings"
	"time"
)

// Notice is the normalized, API-consumer-friendly representation of a TED notice.
type Notice struct {
	PublicationNumber string `json:"publicationNumber"`
	NoticeType        string `json:"noticeType"`
	NoticeTypeLabel   string `json:"noticeTypeLabel"`
	PublicationDate   string `json:"publicationDate"`
	TitleDE           string `json:"titleDe"`
	TitleEN           string `json:"titleEn"`
	BuyerName         string `json:"buyerName"`
	DetailURL         string `json:"detailUrl"`
	PDFURL            string `json:"pdfUrl"`
}

// SearchResult wraps the normalized notice list.
type SearchResult struct {
	Notices    []Notice `json:"notices"`
	TotalFound int      `json:"totalFound"`
	Page       int      `json:"page"`
	Limit      int      `json:"limit"`
}

// NormalizeResponse converts a raw TED API response into clean notices.
func NormalizeResponse(raw *RawSearchResponse, page, limit int) *SearchResult {
	notices := make([]Notice, 0, len(raw.Notices))
	for _, r := range raw.Notices {
		notices = append(notices, normalizeNotice(r))
	}
	return &SearchResult{
		Notices:    notices,
		TotalFound: raw.TotalNoticeCount,
		Page:       page,
		Limit:      limit,
	}
}

func normalizeNotice(r RawNotice) Notice {
	n := Notice{
		PublicationNumber: r.PublicationNumber,
		NoticeType:        r.NoticeType,
		NoticeTypeLabel:   noticeTypeLabel(r.NoticeType),
		PublicationDate:   normalizeDate(r.PublicationDate),
		TitleDE:           pickLang(r.NoticeTitle, "deu", "eng"),
		TitleEN:           pickLang(r.NoticeTitle, "eng", "deu"),
		BuyerName:         pickBuyer(r.BuyerName),
		DetailURL:         detailURL(r.PublicationNumber),
	}
	// Extract German PDF link from links map
	if pdf, ok := r.Links["pdf"].(map[string]interface{}); ok {
		if deu, ok := pdf["DEU"].(string); ok {
			n.PDFURL = deu
		}
	}
	return n
}

func pickLang(m map[string]string, preferred, fallback string) string {
	if v, ok := m[preferred]; ok && v != "" {
		return v
	}
	if v, ok := m[fallback]; ok && v != "" {
		return v
	}
	// Return any non-empty value
	for _, v := range m {
		if v != "" {
			return v
		}
	}
	return ""
}

func pickBuyer(m map[string][]string) string {
	for _, lang := range []string{"deu", "eng"} {
		if vals, ok := m[lang]; ok && len(vals) > 0 {
			return vals[0]
		}
	}
	for _, vals := range m {
		if len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func detailURL(pubNum string) string {
	if pubNum == "" {
		return ""
	}
	return "https://ted.europa.eu/de/notice/-/detail/" + pubNum
}

func normalizeDate(s string) string {
	// "2026-06-01+02:00" → "2026-06-01"
	if len(s) >= 10 {
		s = s[:10]
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return s
	}
	return s
}

// noticeTypeLabel maps TED notice type codes to human-readable labels.
func noticeTypeLabel(t string) string {
	labels := map[string]string{
		"cn-standard":   "Auftragsbekanntmachung (Contract Notice)",
		"can-standard":  "Bekanntmachung vergebener Aufträge (Contract Award Notice)",
		"pin-only":      "Vorinformation (Prior Information Notice)",
		"qu-sy":         "Qualifikationssystem (Qualification System)",
		"cn-desg":       "Wettbewerbsbekanntmachung (Design Contest)",
		"can-desg":      "Bekanntmachung Wettbewerbsergebnis (Design Contest Result)",
		"cn-social":     "Soziale Leistungen (Social Services Notice)",
		"can-social":    "Vergabe soziale Leistungen (Social Services Award)",
		"pin-cfc-social":"Vorinformation Sozial (PIN Social)",
		"cn-defence":    "Verteidigung (Defence Notice)",
		"veat":          "Freiwillige Vorabbekanntmachung (VEAT)",
	}
	if l, ok := labels[t]; ok {
		return l
	}
	return t
}

// BuildQuery constructs a TED expert search query string from human-friendly parameters.
//
//	country   — ISO 3166-1 alpha-3 (DEU, FRA, AUT, CHE, …)
//	keyword   — full-text keyword
//	dateFrom  — YYYYMMDD
//	dateTo    — YYYYMMDD
//	noticeType — notice type code (cn-standard, can-standard, …)
func BuildQuery(country, keyword, dateFrom, dateTo, noticeType string) string {
	var parts []string

	if country != "" {
		country = strings.ToUpper(country)
		parts = append(parts, "place-of-performance IN ("+country+")")
	}

	if dateFrom != "" {
		parts = append(parts, "publication-date >= "+cleanDate(dateFrom))
	}
	if dateTo != "" {
		parts = append(parts, "publication-date <= "+cleanDate(dateTo))
	}

	// Keyword and notice type are added via separate query fragments
	if noticeType != "" {
		parts = append(parts, "notice-type = "+noticeType)
	}

	if keyword != "" {
		// TED expert query: use ~ operator for contains search on notice-title.
		// Multiple words: chain with AND.
		words := strings.Fields(keyword)
		for _, w := range words {
			parts = append(parts, "notice-title ~ "+w)
		}
	}

	if len(parts) == 0 {
		return "place-of-performance IN (DEU)"
	}
	return strings.Join(parts, " AND ")
}

// cleanDate strips dashes/slashes: "2026-06-01" → "20260601"
func cleanDate(d string) string {
	return strings.ReplaceAll(strings.ReplaceAll(d, "-", ""), "/", "")
}
