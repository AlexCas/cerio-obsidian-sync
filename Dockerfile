# Stage 1: Build
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first for better Docker layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code.
COPY . .

# Build the osync binary statically.
# CGO_ENABLED=0 is required because we use modernc.org/sqlite (CGO-free).
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /osync ./cmd/osync

# Stage 2: Runtime (scratch for absolute minimal image)
FROM scratch

# Copy CA certificates for HTTPS calls to external services.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary from the build stage.
COPY --from=builder /osync /osync

# Expose the default server port.
EXPOSE 8080

# Data directory for persistent storage.
VOLUME /data

# Entry point: run the server subcommand.
ENTRYPOINT ["/osync", "server"]