package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

func main() {
	l := launcher.New().
		Headless(true).
		Leakless(false).
		Bin(`C:\Program Files\Google\Chrome\Application\chrome.exe`).
		Set("disable-gpu").
		Set("no-sandbox")

	controlURL, err := l.Launch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "launch: %v\n", err)
		os.Exit(1)
	}
	browser := rod.New().ControlURL(controlURL)
	browser.Connect()
	defer browser.Close()

	page, _ := browser.Page(proto.TargetCreateTarget{URL: "https://www.handelsregister.de/rp_web/welcome.xhtml"})
	page.WaitLoad()
	time.Sleep(2 * time.Second)

	// Accept cookies
	if btn, err := page.Timeout(5 * time.Second).Element(`a[id$="j_idt17"], .cookie-btn`); err == nil {
		btn.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(1 * time.Second)
	}

	// Click advanced search
	if link, err := page.Timeout(8 * time.Second).Element(`[id="naviForm:erweiterteSucheLink"]`); err == nil {
		link.Click(proto.InputMouseButtonLeft, 1)
		page.WaitLoad()
		time.Sleep(2 * time.Second)
	}

	// Fill search term — must click first to focus, then type
	if ta, err := page.Timeout(5 * time.Second).Element(`[id="form:schlagwoerter"]`); err == nil {
		ta.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(300 * time.Millisecond)
		ta.Input("BMW")
		// Trigger blur to fire JSF change event
		page.Eval(`document.getElementById("form:schlagwoerter").dispatchEvent(new Event("change", {bubbles: true}))`)
		fmt.Println("Filled search term: BMW")
	} else {
		fmt.Println("ERROR: search textarea not found:", err)
		os.Exit(1)
	}

	time.Sleep(500 * time.Millisecond)

	// Click search button using JavaScript to ensure it fires
	result, err2 := page.Eval(`(function(){
		var btn = document.getElementById("form:btnSuche");
		if (!btn) return "BUTTON NOT FOUND";
		btn.click();
		return "clicked: " + btn.id + " text=" + btn.textContent.trim();
	})()`)
	if err2 != nil {
		fmt.Println("JS click error:", err2)
	} else {
		fmt.Println("JS click result:", result.Value)
	}

	// Wait for AJAX
	time.Sleep(10 * time.Second)
	fmt.Println("Page URL after wait:", page.MustInfo().URL)

	// Take screenshots at different scroll positions
	page.Eval(`window.scrollTo(0, 0)`)
	time.Sleep(300 * time.Millisecond)
	img, _ := page.Screenshot(false, nil)
	os.WriteFile(`C:\handelsregister-api\results_top.png`, img, 0644)

	page.Eval(`window.scrollTo(0, 1000)`)
	time.Sleep(300 * time.Millisecond)
	img2, _ := page.Screenshot(false, nil)
	os.WriteFile(`C:\handelsregister-api\results_mid.png`, img2, 0644)

	page.Eval(`window.scrollTo(0, 2000)`)
	time.Sleep(300 * time.Millisecond)
	img3, _ := page.Screenshot(false, nil)
	os.WriteFile(`C:\handelsregister-api\results_bottom.png`, img3, 0644)

	fmt.Println("Screenshots saved: results_top/mid/bottom.png")

	info, _ := page.Info()
	fmt.Printf("URL: %s\nTitle: %s\n", info.URL, info.Title)

	// Dump all table rows
	fmt.Println("\n=== TABLE ROWS (first 10) ===")
	rows, _ := page.Elements("tr")
	for i, row := range rows {
		if i >= 10 {
			break
		}
		text, _ := row.Text()
		id, _ := row.Attribute("id")
		cls, _ := row.Attribute("class")
		idStr, clsStr := "", ""
		if id != nil { idStr = *id }
		if cls != nil { clsStr = *cls }
		t := strings.TrimSpace(text)
		if t != "" {
			fmt.Printf("  [%d] id=%q class=%q text=%q\n", i, idStr, clsStr, t[:min(len(t), 100)])
		}
	}

	// Check for result count text
	fmt.Println("\n=== RESULT COUNT TEXT ===")
	for _, sel := range []string{".ui-paginator-current", "[class*='treffer']", "[class*='result']", "[id*='result']"} {
		els, _ := page.Elements(sel)
		for _, el := range els {
			text, _ := el.Text()
			if text != "" {
				fmt.Printf("  [%s] %q\n", sel, text)
			}
		}
	}

	// Full page HTML snippet around results
	html, _ := page.HTML()
	if idx := strings.Index(html, "treffer"); idx > 0 {
		start := idx - 200
		if start < 0 { start = 0 }
		end := idx + 500
		if end > len(html) { end = len(html) }
		fmt.Printf("\n=== HTML around 'treffer' ===\n%s\n", html[start:end])
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
