#!/bin/bash

echo "=== File Shifter Test Suite Overview ==="
echo ""
echo "Verfügbare Test-Skripte für systematisches Testen:"
echo "(Jedes Script baut die Anwendung neu und räumt automatisch auf)"
echo ""

echo "📝 STANDARD & FILESYSTEM TESTS:"
echo "  test-default.sh      - Test ohne Konfigurationsdateien (interne Defaults)"
echo "  test-fs-env.sh       - Test mit .env für Filesystem-Ziele (input → output1, output2)"
echo "  test-fs-yaml.sh      - Test mit env.yaml für Filesystem-Ziele (input → output1, output2)"
echo "  test-yml-format.sh   - Test für env.yml/env.yaml Format-Unterstützung"
echo ""

echo "☁️  S3 TESTS (MinIO erforderlich):"
echo "  test-s3-env.sh       - Test mit .env für S3-Ziele (input → S3 bucket)"
echo "  test-s3-yaml.sh      - Test mit env.yaml für S3-Ziele (input → S3 bucket)"
echo ""

echo "🔄 KOMBINIERTE TESTS:"
echo "  test-combined.sh     - Test mit .env + env.yaml (Prioritäts-Logik prüfen)"
echo ""

echo "🧹 CLEANUP:"
echo "  clean.sh             - Interaktives Aufräumen"
echo "  clean-auto.sh        - Automatisches Aufräumen (inkl. ausführbare Datei)"
echo ""

echo "🔨 BUILD-FEATURES:"
echo "  ✅ Jedes Test-Script baut die Anwendung automatisch"
echo "  ✅ Build-Fehler führen zu sofortigem Test-Abbruch"
echo "  ✅ Ausführbare Datei wird nach jedem Test automatisch entfernt"
echo "  ✅ Unabhängige Tests mit aktuellstem Code"
echo ""

echo "🚀 QUICK START:"
echo "  1. Für einfaches Filesystem-Testen:"
echo "     ./test-fs-env.sh"
echo ""
echo "  2. Für S3-Testen (MinIO muss laufen):"
echo "     docker run -d -p 9000:9000 -p 9001:9001 \\"
echo "       --name minio \\"
echo "       -e MINIO_ROOT_USER=minioadmin \\"
echo "       -e MINIO_ROOT_PASSWORD=minioadmin \\"
echo "       quay.io/minio/minio server /data --console-address ':9001'"
echo "     ./test-s3-env.sh"
echo ""
echo "  3. Für vollständigen Test aller Konfigurationstypen:"
echo "     ./test-combined.sh"
echo ""
echo "  4. Aufräumen nach Tests:"
echo "     ./clean-auto.sh"
echo ""

if [ "$1" = "--run-all" ]; then
    echo "🏃 RUNNING ALL TESTS..."
    echo ""
    
    # Check if MinIO is running
    if curl -s http://localhost:9000/minio/health/live > /dev/null 2>&1; then
        echo "✅ MinIO verfügbar - führe alle Tests aus"
        for script in test-default.sh test-fs-env.sh test-fs-yaml.sh test-yml-format.sh test-s3-env.sh test-s3-yaml.sh test-combined.sh; do
            echo ""
            echo "▶️  Starte $script..."
            ./$script
            echo "✅ $script abgeschlossen"
            ./clean-auto.sh > /dev/null 2>&1
        done
    else
        echo "⚠️  MinIO nicht verfügbar - führe nur Filesystem-Tests aus"
        for script in test-default.sh test-fs-env.sh test-fs-yaml.sh test-yml-format.sh; do
            echo ""
            echo "▶️  Starte $script..."
            ./$script
            echo "✅ $script abgeschlossen"
            ./clean-auto.sh > /dev/null 2>&1
        done
    fi
    
    echo ""
    echo "🎉 Alle verfügbaren Tests abgeschlossen!"
fi