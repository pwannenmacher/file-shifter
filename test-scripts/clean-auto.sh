#!/bin/bash

# File Shifter Quick Cleanup Script
# Automatisches Aufräumen ohne Benutzerinteraktion

echo "=== File Shifter Quick Cleanup ==="

# Alle laufenden file-shifter Prozesse beenden
pkill -f file-shifter 2>/dev/null

# Docker Compose Services stoppen
docker-compose down -v 2>/dev/null

# Test-Verzeichnisse entfernen (alle möglichen Kombinationen)
rm -rf input output output1 output2 yaml-input yaml-output 2>/dev/null

# Temporäre Konfigurationsdateien aufräumen
# Backup der Original-env.yaml falls vorhanden
if [ -f env.yaml ] && [ ! -f env.yaml.backup ]; then
    cp env.yaml env.yaml.backup 2>/dev/null
fi

# Test-Konfigurationsdateien entfernen
rm -f .env env.yaml 2>/dev/null

# Gebaute ausführbare Datei entfernen
rm -f file-shifter 2>/dev/null

# Test-Backup-Dateien aufräumen (falls vorhanden)
rm -f .env.backup.test env.yaml.backup.test 2>/dev/null

# Legacy temporäre Dateien entfernen
rm -f .env.test .env.s3test 2>/dev/null

# Original-Konfiguration wiederherstellen falls vorhanden
if [ -f env.yaml.backup ]; then
    cp env.yaml.backup env.yaml
    echo "Original env.yaml wiederhergestellt"
fi

echo "Quick Cleanup abgeschlossen!"
echo "- Alle Prozesse beendet"
echo "- Docker Services gestoppt" 
echo "- Test-Verzeichnisse entfernt"
echo "- Test-Konfigurationsdateien entfernt"
echo "- Gebaute ausführbare Datei entfernt"
echo "- Test-Backup-Dateien entfernt"
echo "- Original-Konfiguration wiederhergestellt"
echo ""
echo "Workspace ist bereit für neue Tests! 🧹✨"