# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY services/api/go.mod services/api/go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY services/api/ .

# Build binary (CGO disabled for Alpine compatibility)
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/busk-api .

# Runtime stage
FROM alpine:3.19

# Install ca-certificates for HTTPS
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/busk-api .

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health || exit 1

# Expose port
EXPOSE 8080

# Run API
CMD ["./busk-api"]
