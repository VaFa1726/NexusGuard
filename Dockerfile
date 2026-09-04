# ─── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

# Install CA certificates for HTTPS calls
RUN apk add --no-cache ca-certificates git

WORKDIR /app

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build a statically-linked binary — no CGO, stripped symbols
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /nexusguard_bot ./cmd/bot

# ─── Stage 2: Minimal runtime ─────────────────────────────────────────────────
FROM alpine:3.20

# CA certs for HTTPS (Telegram API)
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /nexusguard_bot /app/nexusguard_bot

# Run as non-root
RUN adduser -D -u 1001 nexusguard
USER nexusguard

ENTRYPOINT ["/app/nexusguard_bot"]
