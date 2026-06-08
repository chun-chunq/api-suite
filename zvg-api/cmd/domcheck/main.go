// Command domcheck inspects the ZVG portal search form and result table DOM.
// Run: go run ./cmd/domcheck
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

const (
	chrome    = `C:\Program Files\Google\Chrome\Application\chrome.exe`
	searchURL = "https://www.zvg-portal.de/index.php?button=Termine+suchen"
)

func main() {
	l := launcher.New().
		Bin(chrome).
		Leakless(false).
		Headless(true).
		Set("disable-gpu").
		Set("no-sandbox")

	u, err := l.Launch()
	if err != nil {
		log.Fatal("launch:", err)
	}

	br := rod.New().ControlURL(u)
	if err := br.Connect(); err != nil {
		log.Fatal("connect:", err)
	}
	defer br.Close()

	page, err := br.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		log.Fatal("page:", err)
	}
	defer page.Close()

	fmt.Println("=== Navigating to ZVG search page ===")
	if err := page.Navigate(searchURL); err != nil {
		log.Fatal("navigate:", err)
	}
	page.WaitLoad()
	time.Sleep(2 * time.Second)

	info := page.MustInfo()
	html, _ := page.HTML()
	fmt.Printf("Final URL: %s (%d bytes)\n", info.URL, len(html))
	os.WriteFile(`C:\zvg-api\search_page.html`, []byte(html), 0644)

	// Inspect form action + method
	formInfo, _ := page.Eval(`() => {
		const f = document.querySelector("form");
		if (!f) return "NO FORM";
		return JSON.stringify({action: f.action, method: f.method, id: f.id, name: f.name});
	}`)
	fmt.Println("Form:", formInfo)

	// Set Bundesland = Bayern
	page.Eval(`() => {
		const s = document.querySelector("select[name='land_abk']");
		if (s) { s.value = "by"; s.dispatchEvent(new Event('change', {bubbles:true})); }
	}`)
	time.Sleep(2 * time.Second)

	// Check if Gericht dropdown updated after Bayern
	gerichte, _ := page.Eval(`() => {
		const s = document.querySelector("select[name='ger_id']");
		if (!s) return "NOT FOUND";
		return JSON.stringify(Array.from(s.options).slice(0,30).map(o=>({v:o.value,t:o.text.trim()})));
	}`)
	fmt.Println("Gerichte after Bayern:", gerichte)

	// Submit with proper navigation wait — use the ZVG form (name="globe"), NOT the site-search form
	fmt.Println("\n=== Submitting search (Bayern) ===")
	waitNav := page.MustWaitNavigation()
	page.Eval(`() => {
		const f = document.forms["globe"];
		if (!f) { console.error("form globe not found"); return; }
		// Bypass onsubmit validation
		f.onsubmit = null;
		f.submit();
	}`)
	waitNav()
	time.Sleep(3 * time.Second)

	info2 := page.MustInfo()
	html2, _ := page.HTML()
	fmt.Printf("Results URL: %s (%d bytes)\n", info2.URL, len(html2))
	os.WriteFile(`C:\zvg-api\results_page.html`, []byte(html2), 0644)
	img2, _ := page.Screenshot(false, nil)
	os.WriteFile(`C:\zvg-api\results_page.png`, img2, 0644)

	fmt.Println("\n=== TABLES ===")
	tables, _ := page.Elements("table")
	fmt.Printf("Found %d tables\n", len(tables))
	for i, t := range tables {
		id, _ := t.Attribute("id")
		cls, _ := t.Attribute("class")
		txt, _ := t.Text()
		preview := strings.ReplaceAll(txt, "\n", " | ")
		if len(preview) > 150 {
			preview = preview[:150]
		}
		idStr, clsStr := "", ""
		if id != nil {
			idStr = *id
		}
		if cls != nil {
			clsStr = *cls
		}
		fmt.Printf("  [%d] id=%q class=%q → %q\n", i, idStr, clsStr, preview)
	}

	fmt.Println("\n=== TH HEADERS ===")
	ths, _ := page.Elements("th")
	for i, th := range ths {
		if i >= 30 {
			break
		}
		t, _ := th.Text()
		fmt.Printf("  th[%d]: %q\n", i, strings.TrimSpace(t))
	}

	fmt.Println("\n=== FIRST 30 TABLE ROWS ===")
	rows, _ := page.Elements("table tr")
	for i, row := range rows {
		if i >= 30 {
			fmt.Println("  ...")
			break
		}
		t, _ := row.Text()
		t = strings.ReplaceAll(t, "\n", " | ")
		if len(t) > 200 {
			t = t[:200]
		}
		fmt.Printf("  [%d] %s\n", i, t)
	}

	fmt.Println("\n=== FIRST 30 TD cells ===")
	tds, _ := page.Elements("td")
	for i, td := range tds {
		if i >= 30 {
			break
		}
		cls, _ := td.Attribute("class")
		t, _ := td.Text()
		clsStr := ""
		if cls != nil {
			clsStr = *cls
		}
		t = strings.TrimSpace(strings.ReplaceAll(t, "\n", " "))
		if len(t) > 100 {
			t = t[:100]
		}
		fmt.Printf("  td[%d] class=%q: %q\n", i, clsStr, t)
	}

	// Anchor links in result rows (to detail pages)
	fmt.Println("\n=== LINKS IN RESULTS ===")
	links, _ := page.Eval(`() => {
		return JSON.stringify(Array.from(document.querySelectorAll("table a")).slice(0,20).map(a=>({
			href: a.href, text: a.textContent.trim().substring(0,80)
		})));
	}`)
	fmt.Println(links)

	fmt.Println("\nDone. Files saved to C:\\zvg-api\\")
}
