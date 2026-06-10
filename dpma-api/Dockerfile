FROM golang:1.26-bookworm AS build
ENV GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download all
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        chromium ca-certificates tzdata fonts-liberation \
        libnss3 libatk-bridge2.0-0 libdrm2 libxcomposite1 \
        libxdamage1 libxrandr2 libgbm1 libxkbcommon0 libasound2 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r appuser && useradd -r -g appuser -u 10001 appuser

COPY --from=build /out/server /usr/local/bin/server
USER appuser

ENV CHROME_BIN=/usr/bin/chromium
ENV PORT=8083
# Low idle memory: GC more aggressively, disable preforking
ENV GOGC=50
ENV GOMEMLIMIT=256MiB

EXPOSE 8083
ENTRYPOINT ["/usr/local/bin/server"]
