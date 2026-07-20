# cmd/canary/Dockerfile — Memory Canary Container
#
# Multi-stage build for minimal canary image.
#
# Reference: kgb://factory/workflow

FROM golang:1.22-alpine AS builder
WORKDIR /build

# Copy source
COPY cmd/canary/ .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o canary \
    .

# Final image - scratch for minimal footprint
FROM alpine:3.19

# Install ca-certificates for HTTPS (if needed)
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary
COPY --from=builder /build/canary /app/canary

# Non-root user
RUN adduser -D -u 1000 appuser
USER appuser

# Expose status port
EXPOSE 8080

ENTRYPOINT ["/app/canary"]
CMD ["--mode=bounded"]
