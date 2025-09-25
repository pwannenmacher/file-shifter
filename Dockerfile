# Build-Stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Dependencies kopieren und herunterladen
COPY go.mod go.sum ./
RUN go mod download

# Source-Code kopieren
COPY . .

# Binary kompilieren
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Runtime-Stage
FROM alpine:latest

# Benötigte Pakete installieren
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Binary aus Build-Stage kopieren
COPY --from=builder /app/main .

# Volumes für Input/Output-Verzeichnisse
VOLUME ["/app/input"]
VOLUME ["/app/output"]

# Benutzer für Security
RUN adduser -D -s /bin/sh appuser
USER appuser

# Health-Check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ps aux | grep '[m]ain' || exit 1

# Binary ausführen
CMD ["./main"]