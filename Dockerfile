# Stage 1: Build the Go binary
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Download dependencies first (layer cache optimization)
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build a statically-linked binary with no debug symbols
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /nexusguard_bot ./cmd/bot

# Stage 2: Create a minimal, distroless runner image
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /nexusguard_bot /nexusguard_bot

ENTRYPOINT ["/nexusguard_bot"]
