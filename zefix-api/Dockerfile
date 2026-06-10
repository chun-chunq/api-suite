FROM golang:1.26-alpine AS builder
ENV GOTOOLCHAIN=local
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o zefix-api ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/zefix-api .
EXPOSE 8086
ENV GOGC=100
ENV GOMEMLIMIT=128MiB
CMD ["./zefix-api"]
