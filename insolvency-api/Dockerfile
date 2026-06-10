# ---- build stage ----
FROM golang:1.26-bookworm AS build
ENV GOTOOLCHAIN=local

WORKDIR /src
COPY go.mod go.su[m] ./
RUN go mod download all || go mod tidy
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api

# ---- runtime stage ----
# Debian slim + Chromium for go-rod headless scraping.
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        chromium \
        ca-certificates \
        tzdata \
        fonts-liberation \
        libnss3 \
        libatk-bridge2.0-0 \
        libdrm2 \
        libxcomposite1 \
        libxdamage1 \
        libxrandr2 \
        libgbm1 \
        libxkbcommon0 \
        libasound2 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r appuser && useradd -r -g appuser -u 10001 appuser

COPY --from=build /out/api /usr/local/bin/api

USER appuser
ENV PORT=8080
ENV CHROME_BIN=/usr/bin/chromium
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/api"]
