# Build-Stage
FROM dhi.io/golang:1 AS builder

WORKDIR /app

# Copy dependencies and download
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Runtime-Stage
FROM dhi.io/alpine-base:3.23

USER 0

WORKDIR /app

# Copy binary from build stage
COPY --from=builder /app/main /app/main

# Volumes for input/output directories
RUN mkdir -p /app/input /app/output
RUN chmod -R 755 /app/input /app/output
VOLUME ["/app/input"]
VOLUME ["/app/output"]

# User for security
RUN adduser -D -s /bin/sh appuser
RUN chown -R appuser:appuser /app
USER appuser

# Expose health-check port
EXPOSE 8080

# Health-Check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Execute binary
CMD ["./main"]