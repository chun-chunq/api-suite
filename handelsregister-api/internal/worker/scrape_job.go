package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/handelsregister-api/internal/cache"
	"github.com/handelsregister-api/internal/scraper"
)

// Task type identifiers.
const (
	TypeScrapeCompany = "scrape:company"
	TypeScrapeSearch  = "scrape:search"
)

// ScrapeCompanyPayload is enqueued for an HRB lookup.
type ScrapeCompanyPayload struct {
	HRB   string `json:"hrb"`
	State string `json:"state"`
}

// ScrapeSearchPayload is enqueued for a name search.
type ScrapeSearchPayload struct {
	Name string `json:"name"`
}

// NewScrapeCompanyTask builds an asynq task for a company lookup.
func NewScrapeCompanyTask(hrb, state string) (*asynq.Task, error) {
	payload, err := json.Marshal(ScrapeCompanyPayload{HRB: hrb, State: state})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeScrapeCompany, payload), nil
}

// NewScrapeSearchTask builds an asynq task for a name search.
func NewScrapeSearchTask(name string) (*asynq.Task, error) {
	payload, err := json.Marshal(ScrapeSearchPayload{Name: name})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeScrapeSearch, payload), nil
}

// Handlers wires a scraper and cache into asynq task processors.
type Handlers struct {
	scraper *scraper.Scraper
	cache   *cache.Cache
	log     zerolog.Logger
}

// NewHandlers constructs the worker handler set.
func NewHandlers(s *scraper.Scraper, c *cache.Cache, log zerolog.Logger) *Handlers {
	return &Handlers{scraper: s, cache: c, log: log}
}

// Register attaches all task handlers to an asynq.ServeMux.
func (h *Handlers) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeScrapeCompany, h.handleScrapeCompany)
	mux.HandleFunc(TypeScrapeSearch, h.handleScrapeSearch)
}

func (h *Handlers) handleScrapeCompany(ctx context.Context, t *asynq.Task) error {
	var p ScrapeCompanyPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		// Malformed payloads are not retryable.
		return fmt.Errorf("worker: unmarshal payload: %w: %w", err, asynq.SkipRetry)
	}

	log := h.log.With().Str("task", t.Type()).Str("hrb", p.HRB).Logger()
	log.Info().Msg("processing company scrape")

	data, err := h.scraper.GetByHRB(ctx, p.HRB, p.State)
	if err != nil {
		log.Error().Err(err).Msg("scrape failed")
		return fmt.Errorf("worker: scrape company: %w", err)
	}

	if err := h.cache.SetJSON(ctx, cache.CompanyKey(p.HRB, p.State), data); err != nil {
		log.Error().Err(err).Msg("cache store failed")
		return fmt.Errorf("worker: cache company: %w", err)
	}

	log.Info().Msg("company cached")
	return nil
}

func (h *Handlers) handleScrapeSearch(ctx context.Context, t *asynq.Task) error {
	var p ScrapeSearchPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("worker: unmarshal payload: %w: %w", err, asynq.SkipRetry)
	}

	log := h.log.With().Str("task", t.Type()).Str("name", p.Name).Logger()
	log.Info().Msg("processing search scrape")

	results, err := h.scraper.Search(ctx, p.Name)
	if err != nil {
		log.Error().Err(err).Msg("search failed")
		return fmt.Errorf("worker: scrape search: %w", err)
	}

	if err := h.cache.SetJSON(ctx, cache.SearchKey(p.Name), results); err != nil {
		return fmt.Errorf("worker: cache search: %w", err)
	}

	log.Info().Int("hits", len(results)).Msg("search cached")
	return nil
}
