package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"crypto-api/internal/client"
)

type CryptoHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewCryptoHandler(c *client.Client, log zerolog.Logger) *CryptoHandler {
	return &CryptoHandler{client: c, log: log}
}

// GET /v1/crypto/top?currency=usd&limit=10
func (h *CryptoHandler) GetTop(c *fiber.Ctx) error {
	currency := strings.ToLower(strings.TrimSpace(c.Query("currency", "usd")))
	limit := 10
	if l := c.Query("limit"); l != "" {
		if v, e := strconv.Atoi(l); e == nil && v > 0 && v <= 250 {
			limit = v
		}
	}

	coins, err := h.client.GetTopCoins(c.Context(), currency, limit)
	if err != nil {
		h.log.Error().Err(err).Msg("GetTopCoins failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"coins": coins, "total": len(coins), "currency": currency})
}

// GET /v1/crypto/price?ids=bitcoin,ethereum&currency=usd
func (h *CryptoHandler) GetPrices(c *fiber.Ctx) error {
	idsStr := strings.TrimSpace(c.Query("ids"))
	if idsStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ids is required (comma-separated CoinGecko IDs)"})
	}
	ids := strings.Split(idsStr, ",")
	currency := strings.ToLower(strings.TrimSpace(c.Query("currency", "usd")))

	coins, err := h.client.GetPrices(c.Context(), ids, currency)
	if err != nil {
		h.log.Error().Err(err).Strs("ids", ids).Msg("GetPrices failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"coins": coins, "currency": currency})
}

// GET /v1/crypto/trending
func (h *CryptoHandler) GetTrending(c *fiber.Ctx) error {
	coins, err := h.client.GetTrending(c.Context())
	if err != nil {
		h.log.Error().Err(err).Msg("GetTrending failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"trending": coins})
}

// GET /v1/crypto/coin/:id?currency=usd
func (h *CryptoHandler) GetCoinDetail(c *fiber.Ctx) error {
	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "coin ID is required"})
	}
	currency := strings.ToLower(strings.TrimSpace(c.Query("currency", "usd")))

	detail, err := h.client.GetCoinDetail(c.Context(), id, currency)
	if err != nil {
		h.log.Error().Err(err).Str("id", id).Msg("GetCoinDetail failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(detail)
}
