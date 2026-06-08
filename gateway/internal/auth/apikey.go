// Package auth handles API key validation for multiple marketplace providers.
// Each marketplace injects its key in a different header:
//
//   RapidAPI        → X-RapidAPI-Key (user key) + X-RapidAPI-Proxy-Secret (verification)
//   APILayer        → apikey header
//   Postman         → X-Api-Key header
//   Direct (admin)  → X-Admin-Secret header
//   Custom          → X-API-Key header (generic)
package auth

import (
	"crypto/subtle"
	"os"
	"strings"
)

// Source identifies where the API key came from.
type Source string

const (
	SourceRapidAPI  Source = "rapidapi"
	SourceAPILayer  Source = "apilayer"
	SourcePostman   Source = "postman"
	SourceDirect    Source = "direct"
	SourceAdmin     Source = "admin"
	SourceAnonymous Source = "anonymous"
)

// KeyResult is the result of API key validation.
type KeyResult struct {
	Valid    bool
	Source   Source
	Tier     string // "free", "basic", "pro", "ultra"
	Key      string // the validated key (for logging, redacted)
}

// Validator validates API keys from multiple sources.
type Validator struct {
	// RapidAPI
	rapidAPIProxySecret string // X-RapidAPI-Proxy-Secret from env

	// Direct API keys (stored as KEY:TIER pairs)
	directKeys map[string]string // key → tier

	// Admin secret
	adminSecret string

	// Whether anonymous (no key) requests are allowed
	allowAnonymous bool
}

// NewValidator reads configuration from environment variables.
func NewValidator() *Validator {
	v := &Validator{
		rapidAPIProxySecret: os.Getenv("RAPIDAPI_PROXY_SECRET"),
		adminSecret:         os.Getenv("ADMIN_SECRET"),
		allowAnonymous:      os.Getenv("ALLOW_ANONYMOUS") != "false",
		directKeys:          make(map[string]string),
	}

	// Direct keys: API_KEYS=key1:pro,key2:basic,key3:free
	if raw := os.Getenv("API_KEYS"); raw != "" {
		for _, pair := range strings.Split(raw, ",") {
			parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
			if len(parts) == 2 {
				v.directKeys[parts[0]] = parts[1]
			} else if len(parts) == 1 && parts[0] != "" {
				v.directKeys[parts[0]] = "basic"
			}
		}
	}

	return v
}

// Validate inspects the request headers and returns the key result.
// Headers checked (in order):
//   1. X-Admin-Secret  → admin access
//   2. X-RapidAPI-Proxy-Secret + X-RapidAPI-Key → RapidAPI
//   3. apikey / X-Api-Key / X-API-Key → direct/APILayer/Postman
//   4. No key → anonymous (if allowed)
func (v *Validator) Validate(getHeader func(string) string) KeyResult {
	// ── 1. Admin ──────────────────────────────────────────────────────────────
	if adminKey := getHeader("X-Admin-Secret"); adminKey != "" && v.adminSecret != "" {
		if secureCompare(adminKey, v.adminSecret) {
			return KeyResult{Valid: true, Source: SourceAdmin, Tier: "ultra", Key: redact(adminKey)}
		}
		return KeyResult{Valid: false, Source: SourceAdmin}
	}

	// ── 2. RapidAPI ──────────────────────────────────────────────────────────
	if proxySecret := getHeader("X-RapidAPI-Proxy-Secret"); proxySecret != "" {
		if v.rapidAPIProxySecret == "" {
			// Not configured — let it through (dev mode)
			userKey := getHeader("X-RapidAPI-Key")
			return KeyResult{Valid: true, Source: SourceRapidAPI, Tier: "basic", Key: redact(userKey)}
		}
		if secureCompare(proxySecret, v.rapidAPIProxySecret) {
			userKey := getHeader("X-RapidAPI-Key")
			tier := v.tierForRapidAPI(getHeader("X-RapidAPI-Subscription"))
			return KeyResult{Valid: true, Source: SourceRapidAPI, Tier: tier, Key: redact(userKey)}
		}
		return KeyResult{Valid: false, Source: SourceRapidAPI}
	}

	// ── 3. Direct / APILayer / Postman ────────────────────────────────────────
	for _, headerName := range []string{"apikey", "X-Api-Key", "X-API-Key", "Authorization"} {
		key := getHeader(headerName)
		if key == "" {
			continue
		}
		// Strip "Bearer " prefix for Authorization header
		key = strings.TrimPrefix(key, "Bearer ")
		key = strings.TrimSpace(key)
		if tier, ok := v.directKeys[key]; ok {
			return KeyResult{Valid: true, Source: SourceDirect, Tier: tier, Key: redact(key)}
		}
		return KeyResult{Valid: false, Source: SourceDirect}
	}

	// ── 4. Anonymous ─────────────────────────────────────────────────────────
	if v.allowAnonymous {
		return KeyResult{Valid: true, Source: SourceAnonymous, Tier: "free"}
	}

	return KeyResult{Valid: false, Source: SourceAnonymous}
}

// tierForRapidAPI maps subscription plan names to our internal tiers.
func (v *Validator) tierForRapidAPI(sub string) string {
	sub = strings.ToLower(sub)
	switch {
	case strings.Contains(sub, "ultra") || strings.Contains(sub, "enterprise"):
		return "ultra"
	case strings.Contains(sub, "pro"):
		return "pro"
	case strings.Contains(sub, "basic") || strings.Contains(sub, "starter"):
		return "basic"
	default:
		return "free"
	}
}

// RateLimitForTier returns requests per minute for the given tier.
func RateLimitForTier(tier string) int {
	switch tier {
	case "ultra":
		return 1000
	case "pro":
		return 300
	case "basic":
		return 100
	case "free":
		return 20
	default:
		return 20
	}
}

// secureCompare does a constant-time string comparison.
func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// redact returns the first 8 chars + "..." for logging.
func redact(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:8] + "..."
}
