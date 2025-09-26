#!/bin/bash

echo "=== Test mit kombinierter .env + env.yaml Konfiguration ==="

# Build die Anwendung
echo "Baue file-shifter..."
(cd .. && go build -o test-scripts/file-shifter .) || {
    echo "❌ Build fehlgeschlagen"
    exit 1
}
echo "✅ Build erfolgreich"

# Aufräumen
rm -rf input output1 output2 2>/dev/null

# Backup vorhandener Konfigurationsdateien
[ -f .env ] && cp .env .env.backup.test
[ -f env.yaml ] && cp env.yaml env.yaml.backup.test
rm -f .env env.yaml 2>/dev/null

# Prüfe ob MinIO läuft
echo "Prüfe MinIO-Verfügbarkeit..."
if ! curl -s http://localhost:9000/minio/health/live > /dev/null 2>&1; then
    echo "❌ MinIO ist nicht verfügbar auf localhost:9000"
    echo "Starte MinIO mit: docker run -p 9000:9000 -p 9001:9001 --name minio -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin quay.io/minio/minio server /data --console-address ':9001'"
    exit 1
fi
echo "✅ MinIO ist verfügbar"

echo "Erstelle Test-Umgebung mit kombinierter Konfiguration..."

# env.yaml-Datei erstellen (wird durch .env überschrieben)
cat > env.yaml << 'EOF'
log:
  level: DEBUG
input: ./yaml-input
output:
  - path: ./yaml-output
    type: filesystem
  - path: s3://wrong-bucket/output
    type: s3
    endpoint: wrong-endpoint:9000
    access-key: wrong-key
    secret-key: wrong-secret
    ssl: false
    region: us-west-1
EOF

# .env-Datei erstellen (hat Priorität über env.yaml)
cat > .env << 'EOF'
LOG_LEVEL=INFO
INPUT=./input
OUTPUT_1_PATH=./output1
OUTPUT_1_TYPE=filesystem
OUTPUT_2_PATH=./output2
OUTPUT_2_TYPE=filesystem
OUTPUT_3_PATH=s3://combined-bucket/output
OUTPUT_3_TYPE=s3
OUTPUT_3_ENDPOINT=localhost:9000
OUTPUT_3_ACCESS_KEY=minioadmin
OUTPUT_3_SECRET_KEY=minioadmin
OUTPUT_3_SSL=false
OUTPUT_3_REGION=eu-central-1
EOF

echo "Erstelle env.yaml-Konfiguration (niedrigere Priorität):"
cat env.yaml

echo ""
echo "Erstelle .env-Konfiguration (höhere Priorität):"
cat .env

# Test-Dateien erstellen  
mkdir -p input/subdir
echo "Combined Test-Datei" > input/combined.txt
echo "Subdirectory Test-Datei" > input/subdir/sub.txt

echo ""
echo "Starte file-shifter..."
echo "Erwartung: .env überschreibt env.yaml - Input aus ./input, Output zu output1, output2 und S3"

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
if [ -d "input" ] && [ "$(find input -type f 2>/dev/null | wc -l)" -gt 0 ]; then
    find input -type f 2>/dev/null || echo "Keine Dateien"
    echo "❌ Dateien noch im Input-Directory"
else
    echo "✅ Input-Directory ist leer"
fi

echo ""
echo "Output1-Directory:"
if [ -d "output1" ] && [ "$(find output1 -type f 2>/dev/null | wc -l)" -gt 0 ]; then
    find output1 -type f 2>/dev/null
    echo "✅ Dateien in Output1 gefunden"
else
    echo "❌ Keine Dateien in Output1 gefunden"
fi

echo ""
echo "Output2-Directory:"
if [ -d "output2" ] && [ "$(find output2 -type f 2>/dev/null | wc -l)" -gt 0 ]; then
    find output2 -type f 2>/dev/null
    echo "✅ Dateien in Output2 gefunden"
else
    echo "❌ Keine Dateien in Output2 gefunden"
fi

echo ""
echo "S3-Bucket-Inhalt (combined-bucket):"
# Prüfe S3-Bucket-Inhalt mit MinIO-Client falls verfügbar
if command -v mc > /dev/null 2>&1; then
    echo "Prüfe mit MinIO-Client..."
    mc alias set testminio http://localhost:9000 minioadmin minioadmin > /dev/null 2>&1
    if mc ls testminio/combined-bucket/ 2>/dev/null; then
        echo "✅ Dateien in S3-Bucket gefunden"
    else
        echo "❌ Keine Dateien in S3-Bucket gefunden"
    fi
else
    echo "💡 MinIO-Client (mc) nicht verfügbar für Bucket-Verifikation"
    echo "   Prüfe Logs oben für erfolgreiche S3-Uploads"
fi

echo ""
echo "Prioritäts-Test:"
if [ -d "yaml-input" ] || [ -d "yaml-output" ]; then
    echo "❌ Falsche Verzeichnisse aus env.yaml wurden verwendet"
else
    echo "✅ .env hat korrekt env.yaml überschrieben"
fi

echo ""
echo "Test mit kombinierter Konfiguration abgeschlossen."

# Cleanup: Entferne temporäre Dateien und stelle Backup wieder her
rm -f file-shifter .env env.yaml
[ -f .env.backup.test ] && mv .env.backup.test .env
[ -f env.yaml.backup.test ] && mv env.yaml.backup.test env.yaml
echo "🧹 Aufgeräumt und Original-Konfiguration wiederhergestellt"