FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bafin-api ./cmd/server

FROM chromedp/headless-shell:latest
RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/bafin-api .
EXPOSE 8087
ENV GOGC=50
ENV GOMEMLIMIT=256MiB
ENV CHROME_BIN=/headless-shell/headless-shell
CMD ["./bafin-api"]
