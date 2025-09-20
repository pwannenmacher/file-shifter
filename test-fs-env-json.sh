#!/bin/bash
echo "=== Test der alten ENV-Struktur (Rückwärtskompatibilität) ==="

# Build die Anwendung
go build -o file-shifter . || exit 1

# Cleanup
rm -rf input output1 output2 .env env.yaml 2>/dev/null

# Erstelle alte ENV-Struktur
cat > .env << 'EOL'
LOG_LEVEL=INFO
INPUT=./input
OUTPUTS=[{"path":"./output1","type":"filesystem"},{"path":"./output2","type":"filesystem"}]
EOL

echo "Teste alte ENV-Struktur:"
cat .env

# Test-Dateien erstellen
mkdir -p input
echo "Test der alten ENV-Struktur" > input/old-env.txt

# Kurzer Test
./file-shifter &
PID=$!
sleep 3
kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true

# Prüfe Ergebnisse
echo ""
echo "=== Ergebnisse ==="
if [ -d "output1" ] && [ -f "output1/old-env.txt" ]; then
    echo "✅ Alte ENV-Struktur funktioniert"
else
    echo "❌ Alte ENV-Struktur funktioniert nicht"
fi

# Cleanup
rm -rf input output1 output2 .env file-shifter