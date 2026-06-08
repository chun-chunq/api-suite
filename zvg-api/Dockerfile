# ---- build stage ----
FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.su[m] ./
RUN go mod download all || go mod tidy
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api

# ---- runtime stage ----
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=build /out/api /usr/local/bin/api

USER appuser
ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/api"]
