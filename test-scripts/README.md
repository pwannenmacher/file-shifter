# File Shifter - Test-Suite

## 🧪 Übersicht

Diese Test-Suite bietet umfassende Tests für alle Konfigurationsmethoden und Zieltypen von File Shifter. Alle Tests sind isoliert, non-destructive und self-contained.

## 🚀 Quick Start

```bash
# Einfacher Filesystem-Test
./test-fs-env.sh

# Alle Tests auf einmal ausführen
./test-overview.sh --run-all

# Nach Tests aufräumen
./clean-auto.sh
```

## 📂 Test-Kategorien

### Standard & Filesystem Tests

| Script | Beschreibung | Konfiguration |
|--------|-------------|---------------|
| `test-default.sh` | Zero-Configuration Test | Keine (Standard-Defaults) |
| `test-fs-env.sh` | Filesystem mit ENV-Variablen | `.env` |
| `test-fs-yaml.sh` | Filesystem mit YAML | `env.yaml` |
| `test-fs-env-json.sh` | Filesystem mit JSON ENV | `.env` (JSON-Format) |

### S3 Tests (MinIO erforderlich)

| Script | Beschreibung | Konfiguration |
|--------|-------------|---------------|
| `test-s3-env.sh` | S3/MinIO mit ENV-Variablen | `.env` |
| `test-s3-yaml.sh` | S3/MinIO mit YAML | `env.yaml` |

### Kombinierte Tests

| Script | Beschreibung | Konfiguration |
|--------|-------------|---------------|
| `test-combined.sh` | Multi-Target (FS + S3) | `.env` + `env.yaml` |

### Spezial-Tests

| Script | Beschreibung | Zweck |
|--------|-------------|-------|
| `test-yml-format.sh` | YAML-Format-Test | Validierung verschiedener YAML-Strukturen |

## 🔧 Utilities

| Script | Beschreibung | Verwendung |
|--------|-------------|-----------|
| `test-overview.sh` | Test-Übersicht und Runner | Interaktiv oder `--run-all` |
| `clean.sh` | Interaktives Aufräumen | Benutzergeführt |
| `clean-auto.sh` | Automatisches Aufräumen | CI/CD, nach Tests |

## ⚙️ Voraussetzungen

### Basis-Tests (immer verfügbar)
- Go 1.19+ installiert
- Schreibrechte im Workspace

### S3-Tests (optional)
```bash
# MinIO starten
docker run -d -p 9000:9000 -p 9001:9001 \
  --name minio \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio server /data --console-address ':9001'
```

## 🎯 Test-Features

### Automatisches Test-Management
- ✅ Fresh Build vor jedem Test
- ✅ Backup/Restore von Konfigurationsdateien
- ✅ Isolierte Test-Umgebung
- ✅ Vollständiges Cleanup nach Test

### Test-Isolation
- Jeder Test läuft unabhängig
- Original-Workspace bleibt unverändert
- Keine Abhängigkeiten zwischen Tests

## 📋 Typische Workflows

### Entwicklung
```bash
# Nach Code-Änderungen testen
./test-fs-env.sh
./clean-auto.sh
```

### CI/CD Pipeline
```bash
# Vollständige Test-Suite
./test-overview.sh --run-all
./clean-auto.sh
```

### S3-Integration testen
```bash
# MinIO starten (falls nicht aktiv)
docker run -d -p 9000:9000 --name minio \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio server /data

# S3-Tests ausführen
./test-s3-env.sh
./test-s3-yaml.sh
./clean-auto.sh
```

### Debug-Session
```bash
# Test mit Debug-Logs
LOG_LEVEL=DEBUG ./test-fs-env.sh

# Manual cleanup falls nötig
./clean.sh
```

## 🔍 Test-Details

### Ausgabe-Beispiel
```text
🧪 Testing File Shifter with Filesystem ENV configuration
✅ Build erfolgreich: file-shifter
✅ Konfigurationsdateien gesichert
✅ Test-Umgebung erstellt
📁 Input: ./input → Output: ./output1, ./output2
🚀 File Shifter gestartet (PID: 12345)
📄 Test-Datei erstellt: test-file-20250926-143052.txt
⏳ Warte auf Verarbeitung...
✅ Datei erfolgreich verarbeitet in output1
✅ Datei erfolgreich verarbeitet in output2
✅ Test erfolgreich abgeschlossen
✅ Cleanup abgeschlossen
```

### Fehlerbehebung

**Test schlägt fehl:**
```bash
# Logs prüfen
LOG_LEVEL=DEBUG ./test-fs-env.sh

# Manual cleanup
./clean.sh
```

**MinIO nicht verfügbar:**
```bash
# Status prüfen
docker ps | grep minio

# MinIO starten
docker run -d -p 9000:9000 --name minio ...
```

**Build-Fehler:**
```bash
# Dependencies aktualisieren
go mod tidy

# Manual build testen
go build -o file-shifter ..
```

## 📖 Weitere Informationen

- **Hauptdokumentation**: [`../README.md`](../README.md)
- **Docker-Demo**: [`../demo/README.md`](../demo/README.md) (falls vorhanden)
- **Konfiguration**: Siehe Hauptdokumentation für ENV/YAML-Formate

## 🤝 Beitrag leisten

Beim Hinzufügen neuer Tests:
1. Folgen Sie dem Naming-Schema `test-[kategorie]-[typ].sh`
2. Implementieren Sie Backup/Restore-Mechanismus
3. Fügen Sie Cleanup-Logik hinzu
4. Dokumentieren Sie den Test in dieser README
5. Testen Sie die Integration mit `test-overview.sh`