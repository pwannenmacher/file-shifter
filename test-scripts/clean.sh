#!/bin/bash

# File Shifter Cleanup Script
# Räumt nach Tests auf und stellt den ursprünglichen Zustand wieder her

echo "=== File Shifter Cleanup ==="

# Alle laufenden file-shifter Prozesse beenden
echo "Beende alle laufenden file-shifter Prozesse..."
pkill -f file-shifter 2>/dev/null && echo "Prozesse beendet" || echo "Keine laufenden Prozesse gefunden"

# Docker Compose Services stoppen (falls sie laufen)
echo "Stoppe Docker Compose Services..."
docker-compose down -v 2>/dev/null && echo "Docker Services gestoppt" || echo "Keine Docker Services liefen"

# Test-Verzeichnisse entfernen
echo "Räume Test-Verzeichnisse auf..."
rm -rf input 2>/dev/null && echo "Input-Directory entfernt" || echo "Input-Directory existierte nicht"
rm -rf output1 2>/dev/null && echo "Output1-Directory entfernt" || echo "Output1-Directory existierte nicht"
rm -rf output2 2>/dev/null && echo "Output2-Directory entfernt" || echo "Output2-Directory existierte nicht"

# Temporäre .env-Dateien entfernen
echo "Entferne temporäre Konfigurationsdateien..."
if [ -f .env.backup ]; then
    mv .env.backup .env
    echo "Originale .env wiederhergestellt"
fi

rm -f .env.test .env.s3test 2>/dev/null && echo "Temporäre .env-Dateien entfernt" || echo "Keine temporären .env-Dateien gefunden"

# Kompilierte Binärdateien entfernen (optional)
read -p "Kompilierte Binärdateien entfernen? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -f file-shifter 2>/dev/null && echo "Binärdateien entfernt" || echo "Keine Binärdateien gefunden"
fi

# Docker Volumes und Images aufräumen (optional)
read -p "Docker Volumes und Images aufräumen? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Räume Docker-Ressourcen auf..."
    docker volume prune -f 2>/dev/null
    docker image prune -f 2>/dev/null
    echo "Docker-Ressourcen aufgeräumt"
fi

# Git-Status prüfen (falls es ein Git-Repository ist)
if [ -d .git ]; then
    echo ""
    echo "=== Git Status ==="
    git status --porcelain
    if [ -n "$(git status --porcelain)" ]; then
        echo "Warnung: Es gibt uncommittete Änderungen im Git-Repository"
        read -p "Git-Repository zurücksetzen? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            git reset --hard HEAD
            git clean -fd
            echo "Git-Repository zurückgesetzt"
        fi
    else
        echo "Git-Repository ist sauber"
    fi
fi

echo ""
echo "=== Cleanup abgeschlossen ==="
echo "Workspace-Status:"
echo "- Input-Directory: $([ -d input ] && echo "existiert" || echo "nicht vorhanden")"
echo "- Output1-Directory: $([ -d output1 ] && echo "existiert" || echo "nicht vorhanden")" 
echo "- Output2-Directory: $([ -d output2 ] && echo "existiert" || echo "nicht vorhanden")"
echo "- Laufende Prozesse: $(pgrep -f file-shifter | wc -l | tr -d ' ')"
echo "- Docker Container: $(docker ps --filter name=-q | wc -l | tr -d ' ')"

echo ""
echo "Workspace ist bereit für neue Tests! 🧹✨"