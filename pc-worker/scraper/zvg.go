package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// ZVGQuery mirrors zvg-api SearchQuery.
type ZVGQuery struct {
	State         string   `json:"state"`
	CourtID       string   `json:"courtId"`
	CaseNumber    string   `json:"caseNumber"`
	ProcedureType string   `json:"procedureType"`
	ObjectTypes   []string `json:"objectTypes"`
	PostalCode    string   `json:"postalCode"`
	City          string   `json:"city"`
	Street        string   `json:"street"`
	ObjectText    string   `json:"objectText"`
	SortBy        string   `json:"sortBy"`
	MaxResults    int      `json:"maxResults"`
}

// ZVGResult mirrors zvg-api SearchResult.
type ZVGResult struct {
	Auctions   []ZVGAuction `json:"auctions"`
	TotalCount int          `json:"totalCount"`
	ScrapedAt  time.Time    `json:"scrapedAt"`
}

type ZVGAuction struct {
	ZvgID             string `json:"zvgId"`
	CaseNumber        string `json:"caseNumber"`
	Court             string `json:"court"`
	State             string `json:"state"`
	ObjectDescription string `json:"objectDescription"`
	MarketValue       string `json:"marketValue"`
	AuctionDate       string `json:"auctionDate"`
	AuctionTime       string `json:"auctionTime"`
	DetailURL         string `json:"detailUrl"`
	LastUpdated       string `json:"lastUpdated"`
}

const zvgPortalURL = "https://www.zvg-portal.de/index.php?button=Suchen"

var reEuro = regexp.MustCompile(`[\d.,]+[\s\x{00A0}]*€`)

// stateValues maps two-letter abbreviation (uppercase) → portal value (lowercase).
var stateValues = map[string]string{
	"BB": "bb", "BE": "be", "BW": "bw", "BY": "by",
	"HB": "hb", "HE": "he", "HH": "hh", "MV": "mv",
	"NI": "ni", "NW": "nw", "RP": "rp", "SH": "sh",
	"SL": "sl", "SN": "sn", "ST": "st", "TH": "th",
}

// RunZVG parses the payload JSON, runs the scrape, and returns raw JSON.
func RunZVG(ctx context.Context, chromeBin string, payload json.RawMessage, log zerolog.Logger) (json.RawMessage, error) {
	var q ZVGQuery
	if err := json.Unmarshal(payload, &q); err != nil {
		return nil, fmt.Errorf("zvg payload ungültig: %w", err)
	}
	if q.MaxResults <= 0 {
		q.MaxResults = 100
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

	if err := page.Context(ctx).Navigate(zvgPortalURL); err != nil {
		return nil, fmt.Errorf("ZVG-Portal Navigation fehlgeschlagen: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("ZVG-Portal Ladezeit überschritten: %w", err)
	}

	// Fill Bundesland
	stateVal := stateValues[strings.ToUpper(q.State)]
	if stateVal != "" {
		page.Eval(fmt.Sprintf(`document.forms["globe"]["land_abk"].value = %q`, stateVal))
	}

	// Fill other fields
	setInput := func(name, val string) {
		if val != "" {
			page.Eval(fmt.Sprintf(`document.forms["globe"][%q].value = %q`, name, val))
		}
	}
	setInput("ger_name", q.CourtID)
	setInput("aktenzeichen", q.CaseNumber)
	setInput("verfahren", q.ProcedureType)
	setInput("plz", q.PostalCode)
	setInput("ort", q.City)
	setInput("strasse", q.Street)
	setInput("objekt", q.ObjectText)
	if q.SortBy != "" {
		setInput("order_by", q.SortBy)
	} else {
		setInput("order_by", "2")
	}
	for _, ot := range q.ObjectTypes {
		page.Eval(fmt.Sprintf(`
			(function(){
				var boxes = document.querySelectorAll('input[name="art_id[]"]');
				for(var i=0;i<boxes.length;i++){
					if(boxes[i].value === %q){ boxes[i].checked=true; break; }
				}
			})()`, ot))
	}

	// Submit form
	wait := page.MustWaitNavigation()
	page.Eval(`document.forms["globe"].submit()`)
	wait()

	// Parse results
	rows, _ := page.Elements(`table.data tr`)
	var auctions []ZVGAuction
	var current ZVGAuction
	var inItem bool

	for _, row := range rows {
		cells, _ := row.Elements("td")
		if len(cells) == 0 {
			if inItem && current.CaseNumber != "" {
				auctions = append(auctions, current)
				if len(auctions) >= q.MaxResults {
					break
				}
				current = ZVGAuction{}
				inItem = false
			}
			continue
		}
		inItem = true
		label, _ := cells[0].Text()
		label = strings.TrimSpace(label)
		var val string
		if len(cells) > 1 {
			v, _ := cells[1].Text()
			val = strings.TrimSpace(v)
		}
		switch {
		case strings.Contains(label, "Aktenzeichen"):
			current.CaseNumber = strings.TrimSpace(strings.ReplaceAll(val, "(Detailansicht)", ""))
			current.State = q.State
			// grab link
			if len(cells) > 1 {
				if a, err := cells[1].Element("a"); err == nil {
					if href, err := a.Attribute("href"); err == nil && href != nil {
						current.DetailURL = "https://www.zvg-portal.de" + *href
					}
				}
			}
		case strings.Contains(label, "Amtsgericht"):
			current.Court = val
		case strings.Contains(label, "Objekt"):
			current.ObjectDescription = val
		case strings.Contains(label, "Verkehrswert"):
			if m := reEuro.FindString(val); m != "" {
				current.MarketValue = m
			} else {
				current.MarketValue = val
			}
		case strings.Contains(label, "Termin"):
			parts := strings.Fields(val)
			if len(parts) >= 2 {
				current.AuctionDate = parts[0]
				current.AuctionTime = parts[1]
			} else {
				current.AuctionDate = val
			}
		case strings.Contains(label, "Aktualisierung"):
			current.LastUpdated = val
		}
	}
	if inItem && current.CaseNumber != "" {
		auctions = append(auctions, current)
	}

	result := ZVGResult{
		Auctions:   auctions,
		TotalCount: len(auctions),
		ScrapedAt:  time.Now(),
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
