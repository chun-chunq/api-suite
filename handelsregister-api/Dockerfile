# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.22-bookworm AS build

WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

ARG VERSION=docker
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/api ./cmd/api

# --- runtime stage ---
# Chromium is required by go-rod for the headless scraper.
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        chromium \
        ca-certificates \
        fonts-liberation \
        libnss3 \
        libatk1.0-0 \
        libatk-bridge2.0-0 \
        libcups2 \
        libdrm2 \
        libxkbcommon0 \
        libxcomposite1 \
        libxdamage1 \
        libxfixes3 \
        libxrandr2 \
        libgbm1 \
        libasound2 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r app && useradd -r -g app -G audio,video app

ENV BROWSER_BIN=/usr/bin/chromium \
    APP_ENV=production \
    PORT=8080

COPY --from=build /out/api /usr/local/bin/api

USER app
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD ["/usr/local/bin/api", "-healthcheck"] 2>/dev/null || exit 0

ENTRYPOINT ["/usr/local/bin/api"]
