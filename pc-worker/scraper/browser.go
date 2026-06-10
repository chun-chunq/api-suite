package scraper

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/rs/zerolog"
)

// launchBrowser starts a headless Chrome browser and returns a cleanup func.
func launchBrowser(chromeBin string, log zerolog.Logger) (*rod.Browser, func(), error) {
	l := launcher.New().
		Bin(chromeBin).
		Headless(true).
		Leakless(false). // required on Windows
		Set("disable-gpu", "").
		Set("no-sandbox", "").
		Set("disable-dev-shm-usage", "").
		Set("disable-setuid-sandbox", "")

	u, err := l.Launch()
	if err != nil {
		return nil, nil, fmt.Errorf("Chrome konnte nicht gestartet werden (%s): %w", chromeBin, err)
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		l.Cleanup()
		return nil, nil, fmt.Errorf("Chrome Verbindung fehlgeschlagen: %w", err)
	}

	cleanup := func() {
		browser.Close()
		l.Cleanup()
	}
	return browser, cleanup, nil
}
