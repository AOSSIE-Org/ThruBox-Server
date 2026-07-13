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

RUN addgroup -S relay && adduser -S -G relay relay

COPY --from=builder /app/relay-server /usr/local/bin/relay-server
COPY --from=builder /app/config.yaml /etc/relay/config.yaml

# Create data directory for SQLite and set ownership
RUN mkdir -p /data && chown -R relay:relay /data

# Set default storage path to the mounted volume
ENV RELAY_STORAGE_PATH=/data/relay.db

USER relay

EXPOSE 3000

VOLUME ["/data"]

CMD ["relay-server"]
