// Package client wraps the NASA Open APIs.
// Docs: https://api.nasa.gov/
// Free API key at: https://api.nasa.gov/#signUp (instant, no credit card)
// DEMO_KEY rate limit: 30 req/hour, 50 req/day — register for 1000 req/hour.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBase = "https://api.nasa.gov"

// APODEntry is NASA's Astronomy Picture of the Day.
type APODEntry struct {
	Date           string `json:"date"`
	Title          string `json:"title"`
	Explanation    string `json:"explanation"`
	URL            string `json:"url"`
	HDURL          string `json:"hdurl,omitempty"`
	MediaType      string `json:"media_type"` // "image" or "video"
	ServiceVersion string `json:"service_version,omitempty"`
	Copyright      string `json:"copyright,omitempty"`
	ThumbnailURL   string `json:"thumbnail_url,omitempty"` // for videos
}

// MarsPhoto is a photo taken by a Mars rover.
type MarsPhoto struct {
	ID      int    `json:"id"`
	Sol     int    `json:"sol"`      // Martian day
	EarthDate string `json:"earth_date"`
	ImgSrc  string `json:"img_src"`
	Camera  struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
	} `json:"camera"`
	Rover struct {
		Name        string `json:"name"`
		Status      string `json:"status"`
		LaunchDate  string `json:"launch_date"`
		LandingDate string `json:"landing_date"`
	} `json:"rover"`
}

// MarsPhotoResult wraps a list of Mars rover photos.
type MarsPhotoResult struct {
	Rover  string      `json:"rover"`
	Sol    int         `json:"sol,omitempty"`
	Date   string      `json:"earth_date,omitempty"`
	Camera string      `json:"camera,omitempty"`
	Total  int         `json:"total"`
	Photos []MarsPhoto `json:"photos"`
}

// NearEarthObject is an asteroid/comet from the NASA NEO feed.
type NearEarthObject struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	NASAJplURL         string  `json:"nasa_jpl_url,omitempty"`
	AbsMagnitudeH      float64 `json:"absolute_magnitude_h"`
	EstDiamMinKM       float64 `json:"estimated_diameter_min_km"`
	EstDiamMaxKM       float64 `json:"estimated_diameter_max_km"`
	PotentiallyHazardous bool  `json:"potentially_hazardous"`
	CloseApproachDate  string  `json:"close_approach_date,omitempty"`
	RelVelocityKPH     string  `json:"relative_velocity_kph,omitempty"`
	MissDistanceKM     string  `json:"miss_distance_km,omitempty"`
}

// NEOFeedResult wraps the NEO close approach feed for a date range.
type NEOFeedResult struct {
	StartDate    string            `json:"start_date"`
	EndDate      string            `json:"end_date"`
	TotalObjects int               `json:"total_objects"`
	Hazardous    int               `json:"potentially_hazardous_count"`
	NEOs         []NearEarthObject `json:"neos"`
}

// Client is the NASA API client.
type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string
}

// New returns a new NASA client. If apiKey is empty, uses DEMO_KEY.
func New(apiKey string) *Client {
	if apiKey == "" {
		apiKey = "DEMO_KEY"
	}
	return &Client{
		http:    &http.Client{Timeout: 20 * time.Second},
		baseURL: defaultBase,
		apiKey:  apiKey,
	}
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, params url.Values, out interface{}) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("api_key", c.apiKey)

	u := c.baseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("NASA API rate limit exceeded — register for a free key at api.nasa.gov")
	}
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("NASA API key invalid or rate limit exceeded")
	}
	if resp.StatusCode == http.StatusBadRequest {
		var errResp struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			Msg string `json:"msg"`
		}
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errResp); jsonErr == nil {
			msg := errResp.Error.Message
			if msg == "" {
				msg = errResp.Msg
			}
			return fmt.Errorf("NASA API error: %s", msg)
		}
		return fmt.Errorf("NASA API bad request (HTTP 400)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// ── Public API ───────────────────────────────────────────────────────────────

// GetAPOD returns the Astronomy Picture of the Day for a given date (YYYY-MM-DD).
// If date is empty, returns today's APOD.
func (c *Client) GetAPOD(ctx context.Context, date string) (*APODEntry, error) {
	params := url.Values{}
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return nil, fmt.Errorf("invalid date format: use YYYY-MM-DD")
		}
		params.Set("date", date)
	}
	params.Set("thumbs", "true")

	var entry APODEntry
	if err := c.get(ctx, "/planetary/apod", params, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// GetAPODRange returns APOD entries for a date range (max 7 days to stay polite).
func (c *Client) GetAPODRange(ctx context.Context, startDate, endDate string) ([]APODEntry, error) {
	if startDate == "" || endDate == "" {
		return nil, fmt.Errorf("start_date and end_date are required")
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start_date: use YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end_date: use YYYY-MM-DD")
	}
	if end.Before(start) {
		return nil, fmt.Errorf("end_date must be after start_date")
	}
	if end.Sub(start) > 7*24*time.Hour {
		return nil, fmt.Errorf("date range too large: max 7 days")
	}

	params := url.Values{
		"start_date": []string{startDate},
		"end_date":   []string{endDate},
		"thumbs":     []string{"true"},
	}

	var entries []APODEntry
	if err := c.get(ctx, "/planetary/apod", params, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// GetMarsPhotos returns Mars rover photos for a given sol (Martian day) or Earth date.
// rover: "curiosity", "opportunity", "spirit", "perseverance"
// camera: optional camera abbreviation (FHAZ, RHAZ, MAST, CHEMCAM, NAVCAM, etc.)
func (c *Client) GetMarsPhotos(ctx context.Context, rover, camera string, sol int, earthDate string, limit int) (*MarsPhotoResult, error) {
	rover = strings.ToLower(strings.TrimSpace(rover))
	if rover == "" {
		rover = "curiosity"
	}
	valid := map[string]bool{"curiosity": true, "opportunity": true, "spirit": true, "perseverance": true}
	if !valid[rover] {
		return nil, fmt.Errorf("invalid rover: must be curiosity, opportunity, spirit, or perseverance")
	}

	if limit <= 0 || limit > 25 {
		limit = 10
	}

	params := url.Values{}
	if earthDate != "" {
		if _, err := time.Parse("2006-01-02", earthDate); err != nil {
			return nil, fmt.Errorf("invalid earth_date: use YYYY-MM-DD")
		}
		params.Set("earth_date", earthDate)
	} else {
		if sol <= 0 {
			sol = 1000 // default to sol 1000 for curiosity
		}
		params.Set("sol", fmt.Sprintf("%d", sol))
	}
	if camera != "" {
		params.Set("camera", strings.ToLower(camera))
	}

	path := fmt.Sprintf("/mars-photos/api/v1/rovers/%s/photos", rover)

	var raw struct {
		Photos []MarsPhoto `json:"photos"`
	}
	if err := c.get(ctx, path, params, &raw); err != nil {
		return nil, err
	}

	photos := raw.Photos
	if len(photos) > limit {
		photos = photos[:limit]
	}

	result := &MarsPhotoResult{
		Rover:  rover,
		Total:  len(raw.Photos),
		Photos: photos,
		Camera: camera,
	}
	if earthDate != "" {
		result.Date = earthDate
	} else {
		result.Sol = sol
	}

	return result, nil
}

// GetNEOFeed returns Near Earth Objects for a date range (max 7 days).
func (c *Client) GetNEOFeed(ctx context.Context, startDate, endDate string) (*NEOFeedResult, error) {
	if startDate == "" {
		startDate = time.Now().Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().AddDate(0, 0, 6).Format("2006-01-02")
	}

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start_date: use YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end_date: use YYYY-MM-DD")
	}
	if end.Sub(start) > 7*24*time.Hour {
		return nil, fmt.Errorf("date range too large: max 7 days")
	}

	params := url.Values{
		"start_date": []string{startDate},
		"end_date":   []string{endDate},
	}

	var raw struct {
		ElementCount int `json:"element_count"`
		NearEarthObjects map[string][]struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Links struct {
				Self string `json:"self"`
			} `json:"links"`
			AbsMagnitudeH float64 `json:"absolute_magnitude_h"`
			EstDiam       struct {
				Kilometers struct {
					Min float64 `json:"estimated_diameter_min"`
					Max float64 `json:"estimated_diameter_max"`
				} `json:"kilometers"`
			} `json:"estimated_diameter"`
			Hazardous         bool `json:"is_potentially_hazardous_asteroid"`
			CloseApproachData []struct {
				CloseApproachDate string `json:"close_approach_date"`
				RelativeVelocity  struct {
					KPH string `json:"kilometers_per_hour"`
				} `json:"relative_velocity"`
				MissDistance struct {
					Kilometers string `json:"kilometers"`
				} `json:"miss_distance"`
			} `json:"close_approach_data"`
		} `json:"near_earth_objects"`
	}

	if err := c.get(ctx, "/neo/rest/v1/feed", params, &raw); err != nil {
		return nil, err
	}

	var neos []NearEarthObject
	hazardous := 0

	for _, dayNEOs := range raw.NearEarthObjects {
		for _, n := range dayNEOs {
			neo := NearEarthObject{
				ID:                   n.ID,
				Name:                 n.Name,
				NASAJplURL:           n.Links.Self,
				AbsMagnitudeH:        n.AbsMagnitudeH,
				EstDiamMinKM:         n.EstDiam.Kilometers.Min,
				EstDiamMaxKM:         n.EstDiam.Kilometers.Max,
				PotentiallyHazardous: n.Hazardous,
			}
			if len(n.CloseApproachData) > 0 {
				ca := n.CloseApproachData[0]
				neo.CloseApproachDate = ca.CloseApproachDate
				neo.RelVelocityKPH = ca.RelativeVelocity.KPH
				neo.MissDistanceKM = ca.MissDistance.Kilometers
			}
			if n.Hazardous {
				hazardous++
			}
			neos = append(neos, neo)
		}
	}

	return &NEOFeedResult{
		StartDate:    startDate,
		EndDate:      endDate,
		TotalObjects: raw.ElementCount,
		Hazardous:    hazardous,
		NEOs:         neos,
	}, nil
}
