#!/bin/bash

# Test-Skript für .yml-Format (anstatt .yaml)
# Teste beide unterstützte Dateiformate: env.yml und env.yaml

set -e

echo "=== Test der verschiedenen YAML-Dateiformate ==="
# Build die Anwendung
echo "Baue file-shifter..."
(cd .. && go build -o test-scripts/file-shifter .) || {
    echo "❌ Build fehlgeschlagen"
    exit 1
}
echo "✅ Build erfolgreich"

# Backup vorhandener Konfigurationsdateien
[ -f .env ] && cp .env .env.backup.test
[ -f env.yaml ] && cp env.yaml env.yaml.backup.test
[ -f env.yml ] && cp env.yml env.yml.backup.test
rm -f .env env.yaml env.yml 2>/dev/null

echo "Erstelle Test-Umgebung..."

# Erstelle Input-Dateien
mkdir -p input/subdir
echo "Test mit env.yml" > input/yml-test.txt
echo "Test mit env.yml (subdirectory)" > input/subdir/sub.txt

# Test 1: env.yml-Format
echo ""
echo "=== Test 1: env.yml-Format ==="
cat > env.yml << 'EOF'
log:
  level: INFO
input: ./input
output:
  - path: ./output-yml
    type: filesystem
EOF

echo "Starte file-shifter mit env.yml..."
./file-shifter &
APP_PID=$!
sleep 6
kill $APP_PID 2>/dev/null || true
wait $APP_PID 2>/dev/null || true

echo ""
echo "=== Ergebnisse Test 1 ==="
if [ -d "output-yml" ] && [ -n "$(ls output-yml 2>/dev/null)" ]; then
    echo "✅ env.yml funktioniert - Dateien kopiert:"
    find output-yml -type f | sort
else
    echo "❌ env.yml Test fehlgeschlagen"
fi

# Cleanup Test 1
rm -rf output-yml
rm -f env.yml

# Test 2: env.yaml-Format
echo ""
echo "=== Test 2: env.yaml-Format ==="

# Input-Dateien neu erstellen für Test 2
echo "Test mit env.yaml" > input/yaml-test.txt
echo "Test mit env.yaml (subdirectory)" > input/subdir/sub2.txt

cat > env.yaml << 'EOF'
log:
  level: INFO
input: ./input
output:
  - path: ./output-yaml
    type: filesystem
EOF

echo "Starte file-shifter mit env.yaml..."
./file-shifter &
APP_PID=$!
sleep 6
kill $APP_PID 2>/dev/null || true
wait $APP_PID 2>/dev/null || true

echo ""
echo "=== Ergebnisse Test 2 ==="
if [ -d "output-yaml" ] && [ -n "$(ls output-yaml 2>/dev/null)" ]; then
    echo "✅ env.yaml funktioniert - Dateien kopiert:"
    find output-yaml -type f | sort
else
    echo "❌ env.yaml Test fehlgeschlagen"
fi

# Cleanup Test 2
rm -rf output-yaml
rm -f env.yaml

# Test 3: Beide Dateien vorhanden (sollte Fehler geben)
echo ""
echo "=== Test 3: Konflikt-Test (beide Dateien vorhanden) ==="
cat > env.yml << 'EOF'
log:
  level: INFO
input: ./input
output:
  - path: ./output-yml
    type: filesystem
EOF

cat > env.yaml << 'EOF'
log:
  level: INFO
input: ./input
output:
  - path: ./output-yaml
    type: filesystem
EOF

echo "Starte file-shifter mit beiden Dateien (sollte Fehler ausgeben)..."

# Starte die Anwendung und erfasse Output
./file-shifter > output.log 2>&1 &
APP_PID=$!
sleep 3
kill $APP_PID 2>/dev/null || true
wait $APP_PID 2>/dev/null || true

# Lese den Output aus der Datei
OUTPUT=$(cat output.log 2>/dev/null || echo "")

if echo "$OUTPUT" | grep -q "konflikt.*sowohl.*env.yaml.*als.*auch.*env.yml"; then
    echo "✅ Konflikt-Erkennung funktioniert korrekt"
else
    echo "❌ Konflikt-Erkennung fehlgeschlagen"
    echo "Ausgabe: $OUTPUT"
fi

# Cleanup der temporären Log-Datei
rm -f output.log

echo ""
echo "🧹 Aufräumen..."

# Cleanup
rm -rf input output-yml output-yaml
rm -f env.yml env.yaml output.log

# Restore backups
[ -f .env.backup.test ] && mv .env.backup.test .env
[ -f env.yaml.backup.test ] && mv env.yaml.backup.test env.yaml
[ -f env.yml.backup.test ] && mv env.yml.backup.test env.yml

rm -f file-shifter

echo "Test der YAML-Dateiformate abgeschlossen."