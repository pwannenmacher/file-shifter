#!/bin/bash

echo "=== Test mit .env für Filesystem-Ziele ==="

# Build die Anwendung
echo "Baue file-shifter..."
go build -o file-shifter . || {
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

echo "Erstelle Test-Umgebung mit .env-Konfiguration..."

# .env-Datei für Filesystem-Test erstellen
cat > .env << 'EOF'
LOG_LEVEL=INFO
INPUT=./input
OUTPUT_1_PATH=./output1
OUTPUT_1_TYPE=filesystem
OUTPUT_2_PATH=./output2
OUTPUT_2_TYPE=filesystem
EOF

echo "Erstelle .env-Konfiguration:"
cat .env

# Test-Dateien erstellen  
mkdir -p input/subdir
echo "Filesystem ENV Test-Datei" > input/fs-env.txt
echo "Subdirectory Test-Datei" > input/subdir/sub.txt

echo ""
echo "Starte file-shifter..."

# Starte die Anwendung im Hintergrund
./file-shifter &
PID=$!

# Warte kurz für die Initialisierung
sleep 2

# Prüfe ob der Prozess läuft
if ! ps -p $PID > /dev/null; then
    echo "❌ file-shifter konnte nicht gestartet werden"
    exit 1
fi

# Warte auf Verarbeitung
sleep 5

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
echo "Test mit .env für Filesystem-Ziele abgeschlossen."

# Cleanup: Entferne temporäre Dateien und stelle Backup wieder her
rm -f file-shifter .env
[ -f .env.backup.test ] && mv .env.backup.test .env
[ -f env.yaml.backup.test ] && mv env.yaml.backup.test env.yaml
echo "🧹 Aufgeräumt und Original-Konfiguration wiederhergestellt"