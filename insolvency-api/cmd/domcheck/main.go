package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const chromeBin = `C:\Program Files\Google\Chrome\Application\chrome.exe`

func main() {
	l := launcher.New().
		Bin(chromeBin).
		Leakless(false).
		Headless(true).
		Set("disable-gpu").
		Set("no-sandbox")

	u, err := l.Launch()
	if err != nil {
		log.Fatal(err)
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		log.Fatal(err)
	}
	defer browser.Close()

	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		log.Fatal(err)
	}
	defer page.Close()

	fmt.Println("=== Navigating to search page ===")
	if err := page.Navigate("https://neu.insolvenzbekanntmachungen.de/ap/suche.jsf"); err != nil {
		log.Fatal(err)
	}
	page.WaitLoad()
	time.Sleep(2 * time.Second)

	// Screenshot before search
	img, _ := page.Screenshot(false, nil)
	os.WriteFile("C:/insolvency-api/before_search.png", img, 0644)
	fmt.Println("Screenshot saved: before_search.png")

	// Fill name field with a common German company name
	if el, err := page.Element(`input[id="frm_suche:litx_firmaNachName:text"]`); err == nil {
		el.SelectAllText()
		el.Input("Müller")
		fmt.Println("Filled name: Müller")
	}

	// Set dates
	now := time.Now()
	from := now.AddDate(0, 0, -30).Format("2006-01-02")
	to := now.Format("2006-01-02")

	for _, sel := range []struct{ s, v string }{
		{`input[id="frm_suche:ldi_datumVon:datumHtml5"]`, from},
		{`input[id="frm_suche:ldi_datumBis:datumHtml5"]`, to},
	} {
		if el, err := page.Element(sel.s); err == nil {
			el.Eval(`(v) => { this.value = v; this.dispatchEvent(new Event('input',{bubbles:true})); this.dispatchEvent(new Event('change',{bubbles:true})); }`, sel.v)
			fmt.Printf("Set date %s = %s\n", sel.s, sel.v)
		} else {
			fmt.Printf("WARNING: date field not found: %s\n", sel.s)
		}
	}

	// Dump all input fields in the search form
	allInputs, _ := page.Eval(`() => {
		const f = document.getElementById("frm_suche");
		if (!f) return "form not found";
		const inputs = f.querySelectorAll("input, select");
		return JSON.stringify(Array.from(inputs).map(el => ({
			id: el.id, name: el.name, type: el.type, value: el.value.substring(0,50)
		})));
	}`)
	fmt.Println("Form inputs:", allInputs)

	// Check date field values before submit
	dateDebug, _ := page.Eval(`() => {
		const f = document.getElementById("frm_suche:ldi_datumVon:datumHtml5");
		const t = document.getElementById("frm_suche:ldi_datumBis:datumHtml5");
		return JSON.stringify({
			from: f ? f.value : "NOT FOUND",
			to: t ? t.value : "NOT FOUND",
			fromType: f ? f.type : "",
			toType: t ? t.type : ""
		});
	}`)
	fmt.Println("Date field state:", dateDebug)

	// Try setting dates via rod Input (keyboard simulation)
	if dfEl, err := page.Element(`input[id="frm_suche:ldi_datumVon:datumHtml5"]`); err == nil {
		dfEl.MustClick()
		dfEl.Input(from)
		fmt.Println("Set from date via Input():", from)
	}
	if dtEl, err := page.Element(`input[id="frm_suche:ldi_datumBis:datumHtml5"]`); err == nil {
		dtEl.MustClick()
		dtEl.Input(to)
		fmt.Println("Set to date via Input():", to)
	}

	// Verify dates again
	dateDebug2, _ := page.Eval(`() => {
		const f = document.getElementById("frm_suche:ldi_datumVon:datumHtml5");
		const t = document.getElementById("frm_suche:ldi_datumBis:datumHtml5");
		return JSON.stringify({from: f ? f.value : "NOT FOUND", to: t ? t.value : "NOT FOUND"});
	}`)
	fmt.Println("Date field state after Input():", dateDebug2)

	// Click submit button
	fmt.Println("Clicking submit button...")
	_, err = page.Eval(`() => { const b = document.getElementById("frm_suche:cbt_suchen"); if (b) { b.click(); return "clicked"; } return "not found"; }`)
	if err != nil {
		fmt.Println("ERROR eval:", err)
	}

	fmt.Println("Waiting 12s for navigation...")
	time.Sleep(12 * time.Second)
	page.WaitLoad()
	time.Sleep(3 * time.Second)

	// Screenshot after search
	img, _ = page.Screenshot(false, nil)
	os.WriteFile("C:/insolvency-api/after_search.png", img, 0644)
	fmt.Println("Screenshot saved: after_search.png")

	// URL
	info := page.MustInfo()
	fmt.Println("Current URL:", info.URL)

	// Full HTML
	html, _ := page.HTML()
	os.WriteFile("C:/insolvency-api/results_page.html", []byte(html), 0644)
	fmt.Println("HTML saved: results_page.html (", len(html), "bytes)")

	// Try to find tables
	tables, _ := page.Elements("table")
	fmt.Printf("Found %d tables\n", len(tables))
	for i, t := range tables {
		cls, _ := t.Attribute("class")
		id, _ := t.Attribute("id")
		clsStr := ""
		if cls != nil { clsStr = *cls }
		idStr := ""
		if id != nil { idStr = *id }
		txt, _ := t.Text()
		preview := txt
		if len(preview) > 100 {
			preview = preview[:100]
		}
		fmt.Printf("  Table[%d] id=%q class=%q: %q\n", i, idStr, clsStr, strings.ReplaceAll(preview, "\n", "|"))
	}

	// Try result-specific elements
	for _, sel := range []string{
		".treffer", ".ergebnis", ".result",
		"[class*=treffer]", "[class*=ergebnis]", "[class*=result]",
		"[id*=ergebnis]", "[id*=result]", "[id*=liste]",
		".ui-datatable", ".datatable", "table.list",
		"div#content table", "div.content table",
	} {
		els, err := page.Elements(sel)
		if err == nil && len(els) > 0 {
			fmt.Printf("  Selector %q → %d elements\n", sel, len(els))
		}
	}
}
