package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/insolvency-api/internal/cache"
	"github.com/insolvency-api/internal/scraper"
)

// TaskMonitorCompany is the asynq task type for the daily monitoring of a single
// company.
const TaskMonitorCompany = "monitor:company"

// MonitorPayload is the JSON payload of a monitoring task.
type MonitorPayload struct {
	CompanyName    string `json:"companyName"`
	RegisterNumber string `json:"registerNumber"`
	State          string `json:"state"`
	WebhookURL     string `json:"webhookUrl"`
}

// NewMonitorTask constructs an asynq task for the given company.
func NewMonitorTask(p MonitorPayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskMonitorCompany, payload), nil
}

// MonitorHandler processes monitoring tasks: it scrapes recent insolvency
// announcements for the target company and records results in the cache for
// later webhook delivery / diffing.
type MonitorHandler struct {
	cache *cache.Cache
	log   zerolog.Logger
}

// NewMonitorHandler builds the handler.
func NewMonitorHandler(c *cache.Cache, log zerolog.Logger) *MonitorHandler {
	return &MonitorHandler{cache: c, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *MonitorHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p MonitorPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w: %w", err, asynq.SkipRetry)
	}

	h.log.Info().Str("company", p.CompanyName).Str("hrb", p.RegisterNumber).Msg("running monitor job")

	sc, err := scraper.New(scraper.Options{Logger: h.log})
	if err != nil {
		return err
	}
	defer sc.Close()

	q := scraper.SearchQuery{
		Name:           p.CompanyName,
		RegisterNumber: p.RegisterNumber,
		RegisterType:   "HRB",
		State:          p.State,
		DateFrom:       time.Now().AddDate(0, 0, -7),
		DateTo:         time.Now(),
	}

	res, err := sc.Search(ctx, q)
	if err != nil {
		return fmt.Errorf("monitor scrape: %w", err)
	}

	// Persist latest snapshot; webhook dispatch would diff against prior state.
	key := fmt.Sprintf("monitor:snapshot:%s:%s", p.RegisterNumber, p.State)
	if h.cache != nil {
		if err := h.cache.SetJSON(ctx, key, res, 48*time.Hour); err != nil {
			h.log.Warn().Err(err).Msg("failed to persist monitor snapshot")
		}
	}

	h.log.Info().Str("company", p.CompanyName).Int("found", res.Totalfound).
		Msg("monitor job complete")

	if p.WebhookURL != "" && res.Totalfound > 0 {
		h.log.Info().Str("webhook", p.WebhookURL).Msg("would dispatch webhook (stub)")
	}
	return nil
}

// RegisterScheduler schedules a daily monitoring sweep. In a full deployment the
// list of monitored companies would be loaded from a datastore; here we wire the
// periodic trigger.
func RegisterScheduler(scheduler *asynq.Scheduler, companies []MonitorPayload) error {
	for _, c := range companies {
		task, err := NewMonitorTask(c)
		if err != nil {
			return err
		}
		// Daily at 06:00 server time.
		if _, err := scheduler.Register("0 6 * * *", task); err != nil {
			return err
		}
	}
	return nil
}
