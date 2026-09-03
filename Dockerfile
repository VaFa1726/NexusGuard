# Stage 1: Build the Go binary
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /nexusguard_bot ./cmd/bot

# Stage 2: Create a minimal runner container
FROM alpine:3.19

# Add CA certificates for TLS (important for Telegram API)
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /nexusguard_bot /app/nexusguard_bot

CMD ["/app/nexusguard_bot"]
