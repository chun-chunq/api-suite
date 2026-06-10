FROM golang:1.26-alpine AS builder
ENV GOTOOLCHAIN=local
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o dashboard ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/dashboard .
EXPOSE 8088
CMD ["./dashboard"]
