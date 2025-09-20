#!/bin/bash

echo "=== Test ohne Konfigurationsdateien (Standard-Defaults) ==="

# Build die Anwendung
echo "Baue file-shifter..."
go build -o file-shifter . || {
    echo "❌ Build fehlgeschlagen"
    exit 1
}
echo "✅ Build erfolgreich"

# Aufräumen
rm -rf input output .env env.yaml 2>/dev/null

echo "Erstelle Test-Umgebung ohne Konfigurationsdateien..."

# Test-Dateien erstellen  
mkdir -p input/subdir
echo "Standard Default Test-Datei" > input/default.txt
echo "Subdirectory Test-Datei" > input/subdir/sub.txt

echo "Teste ohne .env und env.yaml (Standard-Defaults):"

# Da keine Konfiguration vorhanden ist, sollte die App Defaults verwenden
# Basierend auf der Logik: input directory sollte aus env kommen oder leer sein
# output targets sollten aus env kommen oder leer sein

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
    echo "✅ Input-Directory ist leer oder existiert nicht"
fi

echo ""
echo "Output-Directory:"
if [ -d "output" ] && [ "$(find output -type f 2>/dev/null | wc -l)" -gt 0 ]; then
    find output -type f 2>/dev/null
    echo "✅ Dateien in Output-Directory gefunden"
else
    echo "❌ Keine Dateien in Output-Directory gefunden"
fi

echo ""
echo "Test der Standard-Defaults abgeschlossen."

# Cleanup: Entferne die gebaute ausführbare Datei
rm -f file-shifter
echo "🧹 Ausführbare Datei entfernt"