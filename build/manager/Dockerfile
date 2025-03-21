FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 go build -o /turkis-manager ./cmd/manager

# Use a small image for the final container
FROM alpine:3.18

# Install ca-certificates for HTTPS and curl for healthchecks
RUN apk add --no-cache ca-certificates docker-cli socat

# Copy the binary from the builder stage
COPY --from=builder /turkis-manager /usr/local/bin/turkis-manager

# Create necessary directories
RUN mkdir -p /etc/haproxy/certs /var/www/lego /config

# Set executable permissions
RUN chmod +x /usr/local/bin/turkis-manager

# Run as non-root user for better security
RUN addgroup -S turkis && adduser -S turkis -G turkis
USER turkis

# Run the manager
ENTRYPOINT ["/usr/local/bin/turkis-manager"]
