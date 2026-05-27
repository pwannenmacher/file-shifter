#!/bin/bash

echo "=== Test mit .env für S3-Ziele ==="

# Build die Anwendung
echo "Baue file-shifter..."
(cd .. && go build -o test-scripts/file-shifter .) || {
    echo "❌ Build fehlgeschlagen"
    exit 1
}
echo "✅ Build erfolgreich"

# Aufräumen
rm -rf input 2>/dev/null

# Backup vorhandener Konfigurationsdateien
[[ -f .env ]] && cp .env .env.backup.test
[[ -f env.yaml ]] && cp env.yaml env.yaml.backup.test
rm -f .env env.yaml 2>/dev/null

# Prüfe ob MinIO läuft
echo "Prüfe MinIO-Verfügbarkeit..."
if ! curl -s http://localhost:9000/minio/health/live > /dev/null 2>&1; then
    echo "❌ MinIO ist nicht verfügbar auf localhost:9000"
    echo "Starte MinIO mit: docker run -p 9000:9000 -p 9001:9001 --name minio -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin quay.io/minio/minio server /data --console-address ':9001'"
    exit 1
fi
echo "✅ MinIO ist verfügbar"

echo "Erstelle Test-Umgebung mit .env-Konfiguration..."

# .env-Datei für S3-Test erstellen
cat > .env << 'EOF'
LOG_LEVEL=INFO
INPUT=./input
OUTPUT_1_PATH=s3://test-bucket/output
OUTPUT_1_TYPE=s3
OUTPUT_1_ENDPOINT=localhost:9000
OUTPUT_1_ACCESS_KEY=minioadmin
OUTPUT_1_SECRET_KEY=minioadmin
OUTPUT_1_SSL=false
OUTPUT_1_REGION=eu-central-1
EOF

echo "Erstelle .env-Konfiguration:"
cat .env

# Test-Dateien erstellen  
mkdir -p input/subdir
echo "S3 ENV Test-Datei" > input/s3-env.txt
echo "Subdirectory Test-Datei" > input/subdir/sub.txt

echo ""
echo "Starte file-shifter..."

# Starte die Anwendung im Hintergrund
./file-shifter &
PID=$!

# Warte kurz für die Initialisierung
sleep 3

# Prüfe ob der Prozess läuft
if ! ps -p $PID > /dev/null; then
    echo "❌ file-shifter konnte nicht gestartet werden"
    exit 1
fi

# Warte auf Verarbeitung
sleep 7

# Stoppe die Anwendung
echo "Stoppe file-shifter..."
kill -TERM $PID 2>/dev/null || kill -9 $PID 2>/dev/null
wait $PID 2>/dev/null

echo ""
echo "=== Ergebnisse ==="

echo "Input-Directory:"
if [[ -d "input" ]] && [[ "$(find input -type f 2>/dev/null | wc -l)" -gt 0 ]]; then
    find input -type f 2>/dev/null || echo "Keine Dateien"
    echo "❌ Dateien noch im Input-Directory"
else
    echo "✅ Input-Directory ist leer"
fi

echo ""
echo "S3-Bucket-Inhalt (test-bucket):"
# Prüfe S3-Bucket-Inhalt mit MinIO-Client falls verfügbar
if command -v mc > /dev/null 2>&1; then
    echo "Prüfe mit MinIO-Client..."
    mc alias set testminio http://localhost:9000 minioadmin minioadmin > /dev/null 2>&1
    if mc ls testminio/test-bucket/ 2>/dev/null; then
        echo "✅ Dateien in S3-Bucket gefunden"
    else
        echo "❌ Keine Dateien in S3-Bucket gefunden"
    fi
else
    echo "💡 MinIO-Client (mc) nicht verfügbar für Bucket-Verifikation"
    echo "   Prüfe Logs oben für erfolgreiche S3-Uploads"
fi

echo ""
echo "Test mit .env für S3-Ziele abgeschlossen."

# Cleanup: Entferne temporäre Dateien und stelle Backup wieder her
rm -f file-shifter .env
[[ -f .env.backup.test ]] && mv .env.backup.test .env
[[ -f env.yaml.backup.test ]] && mv env.yaml.backup.test env.yaml
echo "🧹 Aufgeräumt und Original-Konfiguration wiederhergestellt"