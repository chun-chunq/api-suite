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
		fmt.Fprintf(os.Stderr, "launch error: %v\n", err)
		os.Exit(1)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "connect error: %v\n", err)
		os.Exit(1)
	}
	defer browser.Close()

	page, err := browser.Page(proto.TargetCreateTarget{URL: "https://www.handelsregister.de/rp_web/welcome.xhtml"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "page error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Step 1: Loading homepage...")
	page.WaitLoad()
	time.Sleep(2 * time.Second)

	// Accept cookie
	fmt.Println("Step 2: Accepting cookies...")
	if btn, err := page.Timeout(5 * time.Second).Element(`a[id$="j_idt17"], .cookie-btn`); err == nil {
		btn.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(1 * time.Second)
		fmt.Println("  Cookie accepted!")
	} else {
		fmt.Println("  No cookie banner found:", err)
	}

	// Click advanced search
	fmt.Println("Step 3: Clicking advanced search link...")
	if advLink, err := page.Timeout(8 * time.Second).Element(`[id="naviForm:erweiterteSucheLink"]`); err == nil {
		advLink.Click(proto.InputMouseButtonLeft, 1)
		page.WaitLoad()
		time.Sleep(2 * time.Second)
		fmt.Println("  Clicked!")
	} else {
		fmt.Println("  Link not found:", err)
	}

	info, _ := page.Info()
	fmt.Printf("  URL: %s\n", info.URL)
	fmt.Printf("  Title: %s\n", info.Title)

	img, _ := page.Screenshot(false, nil)
	os.WriteFile(`C:\handelsregister-api\screenshot2.png`, img, 0644)
	fmt.Println("  Screenshot saved to screenshot2.png")

	// Dump inputs
	fmt.Println("\n=== INPUT elements ===")
	inputs, _ := page.Elements("input, textarea, select")
	for _, inp := range inputs {
		id, _ := inp.Attribute("id")
		name, _ := inp.Attribute("name")
		typ, _ := inp.Attribute("type")
		idStr, nameStr, typStr := "", "", ""
		if id != nil {
			idStr = *id
		}
		if name != nil {
			nameStr = *name
		}
		if typ != nil {
			typStr = *typ
		}
		if typStr != "hidden" {
			tag, _ := inp.Eval(`() => this.tagName`)
			fmt.Printf("  <%s> id=%q name=%q type=%q\n", strings.ToLower(tag.Value.String()), idStr, nameStr, typStr)
		}
	}

	fmt.Println("\n=== BUTTONS ===")
	buttons, _ := page.Elements("button, input[type=submit], a.ui-commandlink")
	for _, btn := range buttons {
		id, _ := btn.Attribute("id")
		text, _ := btn.Text()
		idStr := ""
		if id != nil {
			idStr = *id
		}
		t := strings.TrimSpace(text)
		if t != "" || idStr != "" {
			fmt.Printf("  id=%q text=%q\n", idStr, t)
		}
	}
}
