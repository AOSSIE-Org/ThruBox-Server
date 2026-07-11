# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies for CGo (required by go-sqlite3)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Download dependencies first (layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN CGO_ENABLED=1 go build -o relay-server -ldflags="-s -w" ./cmd/relay

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/relay-server /usr/local/bin/relay-server
COPY --from=builder /app/config.yaml /etc/relay/config.yaml

# Create data directory for SQLite
RUN mkdir -p /data

# Set default storage path to the mounted volume
ENV RELAY_STORAGE_PATH=/data/relay.db

EXPOSE 3000

VOLUME ["/data"]

CMD ["relay-server"]
