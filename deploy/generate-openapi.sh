#!/usr/bin/env bash
# generate-openapi.sh — generates an OpenAPI 3.0 spec for the full API suite
# Output: stdout (pipe to openapi.yaml)
# Usage: ./generate-openapi.sh > openapi.yaml
set -euo pipefail

DOMAIN="${DOMAIN:-api.yourdomain.com}"

cat <<EOF
openapi: "3.0.3"
info:
  title: "API Suite"
  description: |
    A comprehensive API suite covering business data, science, entertainment, and finance.
    All endpoints are accessible via HTTPS. Authentication via API key header.
  version: "1.0.0"
  contact:
    email: "your@email.com"
  license:
    name: "Commercial"

servers:
  - url: "https://${DOMAIN}"
    description: "Production"

security:
  - apiKey: []

components:
  securitySchemes:
    apiKey:
      type: apiKey
      in: header
      name: X-API-Key
      description: |
        API key for authentication. Get your key at RapidAPI, APILayer, or by contacting us.
        Anonymous access is available for testing (20 req/min).

  responses:
    Unauthorized:
      description: "API key missing or invalid"
      content:
        application/json:
          schema:
            type: object
            properties:
              error:
                type: string
                example: "unauthorized"
              message:
                type: string
    RateLimitExceeded:
      description: "Rate limit exceeded for your plan tier"
      content:
        application/json:
          schema:
            type: object
            properties:
              error:
                type: string
                example: "rate_limit_exceeded"
              tier:
                type: string
              message:
                type: string
    ServiceUnavailable:
      description: "Upstream API temporarily unavailable (circuit open)"
      content:
        application/json:
          schema:
            type: object
            properties:
              error:
                type: string
                example: "service_unavailable"
              upstream:
                type: string
              circuit_state:
                type: string

paths:
  # ── VAT Validation ────────────────────────────────────────────────────────
  /v1/vat/validate:
    get:
      tags: [VAT & Tax]
      summary: "Validate EU VAT number"
      description: "Validate a EU VAT number via the official VIES service."
      parameters:
        - name: vat
          in: query
          required: true
          schema:
            type: string
          example: "DE123456789"
      responses:
        "200":
          description: "VAT validation result"
        "401":
          \$ref: "#/components/responses/Unauthorized"
        "503":
          \$ref: "#/components/responses/ServiceUnavailable"

  /v1/vat/countries:
    get:
      tags: [VAT & Tax]
      summary: "List EU VAT countries"
      responses:
        "200":
          description: "List of EU member states with VAT codes"

  # ── FX Rates ─────────────────────────────────────────────────────────────
  /v1/fx/latest:
    get:
      tags: [Finance & FX]
      summary: "Latest FX rates (ECB)"
      parameters:
        - name: base
          in: query
          schema:
            type: string
            default: EUR
          example: USD
        - name: symbols
          in: query
          schema:
            type: string
          example: "USD,GBP,JPY"
      responses:
        "200":
          description: "Current exchange rates"

  /v1/fx/convert:
    get:
      tags: [Finance & FX]
      summary: "Convert currency amount"
      parameters:
        - name: from
          in: query
          required: true
          schema:
            type: string
          example: USD
        - name: to
          in: query
          required: true
          schema:
            type: string
          example: EUR
        - name: amount
          in: query
          required: true
          schema:
            type: number
          example: 100
      responses:
        "200":
          description: "Converted amount with rate"

  /v1/fx/history:
    get:
      tags: [Finance & FX]
      summary: "FX rate history"
      parameters:
        - name: base
          in: query
          schema:
            type: string
          example: EUR
        - name: start
          in: query
          required: true
          schema:
            type: string
            format: date
          example: "2024-01-01"
        - name: end
          in: query
          required: true
          schema:
            type: string
            format: date
          example: "2024-03-31"
      responses:
        "200":
          description: "Historical exchange rates"

  # ── Countries ────────────────────────────────────────────────────────────
  /v1/countries/all:
    get:
      tags: [Geographic]
      summary: "All countries"
      responses:
        "200":
          description: "All 250 countries with metadata"

  /v1/countries/code/{code}:
    get:
      tags: [Geographic]
      summary: "Country by ISO code"
      parameters:
        - name: code
          in: path
          required: true
          schema:
            type: string
          example: DE
      responses:
        "200":
          description: "Country details"
        "404":
          description: "Country not found"

  # ── NASA ─────────────────────────────────────────────────────────────────
  /v1/nasa/apod:
    get:
      tags: [Science & Space]
      summary: "NASA Astronomy Picture of the Day"
      parameters:
        - name: date
          in: query
          schema:
            type: string
            format: date
          example: "2024-01-15"
      responses:
        "200":
          description: "APOD entry with image URL and explanation"

  /v1/nasa/neo:
    get:
      tags: [Science & Space]
      summary: "Near Earth Objects feed"
      parameters:
        - name: start
          in: query
          required: true
          schema:
            type: string
            format: date
        - name: end
          in: query
          required: true
          schema:
            type: string
            format: date
      responses:
        "200":
          description: "Near-Earth asteroid data"

  # ── Air Quality ───────────────────────────────────────────────────────────
  /v1/air/current:
    get:
      tags: [Environment]
      summary: "Current air quality"
      parameters:
        - name: lat
          in: query
          required: true
          schema:
            type: number
          example: 48.137154
        - name: lon
          in: query
          required: true
          schema:
            type: number
          example: 11.576124
      responses:
        "200":
          description: "Air quality index with category classification"

  # ── Trivia ────────────────────────────────────────────────────────────────
  /v1/trivia/questions:
    get:
      tags: [Entertainment]
      summary: "Get trivia questions"
      parameters:
        - name: amount
          in: query
          schema:
            type: integer
            default: 10
            maximum: 50
        - name: category
          in: query
          schema:
            type: integer
          description: "Category ID. Use /v1/trivia/categories to list."
        - name: difficulty
          in: query
          schema:
            type: string
            enum: [easy, medium, hard]
        - name: type
          in: query
          schema:
            type: string
            enum: [multiple, boolean]
      responses:
        "200":
          description: "Trivia questions with shuffled answer choices"

  # ── Jokes ────────────────────────────────────────────────────────────────
  /v1/jokes/:
    get:
      tags: [Entertainment]
      summary: "Get jokes"
      parameters:
        - name: category
          in: query
          schema:
            type: string
            enum: [Any, Programming, Misc, Dark, Pun, Spooky, Christmas]
            default: Any
        - name: count
          in: query
          schema:
            type: integer
            default: 1
            maximum: 10
        - name: safe
          in: query
          schema:
            type: boolean
            default: false
      responses:
        "200":
          description: "Jokes with category and content flags"

  # ── World Bank ────────────────────────────────────────────────────────────
  /v1/worldbank/country/{code}:
    get:
      tags: [Business & Economics]
      summary: "World Bank country metadata"
      parameters:
        - name: code
          in: path
          required: true
          schema:
            type: string
          example: DE
      responses:
        "200":
          description: "Country info with income level, region, capital"

  /v1/worldbank/indicator:
    get:
      tags: [Business & Economics]
      summary: "World Bank economic indicator"
      description: "Fetch GDP, population, inflation, unemployment, and 10+ other indicators."
      parameters:
        - name: country
          in: query
          required: true
          schema:
            type: string
          example: DE
        - name: indicator
          in: query
          required: true
          schema:
            type: string
          example: NY.GDP.MKTP.CD
        - name: start
          in: query
          schema:
            type: integer
          example: 2010
        - name: end
          in: query
          schema:
            type: integer
          example: 2023
      responses:
        "200":
          description: "Time series indicator data"

  # ── Clinical Trials ───────────────────────────────────────────────────────
  /v1/trials/search:
    get:
      tags: [Healthcare & Science]
      summary: "Search ClinicalTrials.gov"
      parameters:
        - name: q
          in: query
          schema:
            type: string
          example: "diabetes type 2"
        - name: status
          in: query
          schema:
            type: string
            enum: [RECRUITING, COMPLETED, NOT_YET_RECRUITING, ACTIVE_NOT_RECRUITING]
        - name: phase
          in: query
          schema:
            type: string
            enum: [PHASE1, PHASE2, PHASE3, PHASE4]
        - name: limit
          in: query
          schema:
            type: integer
            default: 10
            maximum: 100
      responses:
        "200":
          description: "Clinical trial studies matching criteria"

  # ── Gateway ────────────────────────────────────────────────────────────────
  /gateway/status:
    get:
      tags: [Internal]
      summary: "Gateway circuit breaker status"
      description: "Shows health and circuit state for all upstream APIs."
      responses:
        "200":
          description: "Upstream health summary"
EOF
