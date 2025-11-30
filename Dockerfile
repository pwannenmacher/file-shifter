# Build-Stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy dependencies and download
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Runtime-Stage
FROM alpine:latest

# Install required packages (including lsof for file monitoring)
RUN apk --no-cache add ca-certificates lsof tzdata wget

WORKDIR /root/

# Copy binary from build stage
COPY --from=builder /app/main .

# Volumes for input/output directories
RUN mkdir -p /app/input /app/output
RUN chmod -R 755 /app/input /app/output
VOLUME ["/app/input"]
VOLUME ["/app/output"]

# User for security
RUN adduser -D -s /bin/sh appuser
USER appuser

# Expose health-check port
EXPOSE 8080

# Health-Check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Execute binary
CMD ["./main"]