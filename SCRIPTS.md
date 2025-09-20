# File Shifter - Scripts Übersicht

## 🧪 Test-Suite

### Systematische Test-Skripte

Die Test-Suite ist systematisch nach Konfigurationstypen und Zieltypen
organisiert. Jedes Script:

- ✅ Baut die Anwendung automatisch neu
- ✅ Sichert vorhandene Konfigurationsdateien
- ✅ Erstellt isolierte Test-Umgebung
- ✅ Stellt Original-Konfiguration nach dem Test wieder her
- ✅ Räumt alle temporären Dateien auf

---

## 📝 Standard & Filesystem Tests

### test-default.sh

#### Test ohne Konfigurationsdateien (Standard-Defaults)

```bash
Input:  ./input (automatisch erstellt)
Output: ./output (automatisch erstellt)
Typ:    filesystem
```

- Testet das Zero-Configuration-Verhalten
- Überprüft Standard-Defaults der Anwendung
- Keine .env oder env.yaml erforderlich

### test-fs-env.sh

#### Test mit .env für Filesystem-Ziele

```bash
Input:  ./input
Output: ./output1, ./output2
Typ:    filesystem
Config: .env (Environment-Variablen)
```

- Testet .env-basierte Konfiguration
- Multi-Target Filesystem-Setup
- Environment-Variable Priorität

### test-fs-yaml.sh

#### Test mit env.yaml für Filesystem-Ziele

```bash
Input:  ./input
Output: ./output1, ./output2  
Typ:    filesystem
Config: env.yaml (YAML-Konfiguration)
```

- Testet YAML-basierte Konfiguration
- Multi-Target Filesystem-Setup
- Strukturierte Konfiguration

---

## ☁️ S3 Tests (MinIO erforderlich)

### Voraussetzungen

```bash
# MinIO starten
docker run -d -p 9000:9000 -p 9001:9001 \
  --name minio \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio server /data --console-address ':9001'
```

### test-s3-env.sh

#### Test mit .env für S3-Ziele

```bash
Input:  ./input
Output: s3://test-bucket/output
Typ:    s3 (MinIO localhost:9000)
Config: .env (Environment-Variablen)
```

- Testet S3-Integration über Environment-Variablen
- MinIO-Client Bucket-Verifikation (falls verfügbar)
- S3-spezifische Konfiguration

### test-s3-yaml.sh

#### Test mit env.yaml für S3-Ziele

```bash
Input:  ./input
Output: s3://test-bucket/output
Typ:    s3 (MinIO localhost:9000)
Config: env.yaml (YAML-Konfiguration)
```

- Testet S3-Integration über YAML-Konfiguration
- MinIO-Client Bucket-Verifikation (falls verfügbar)
- Strukturierte S3-Konfiguration

---

## 🔄 Kombinierte Tests

### test-combined.sh

#### Test mit .env + env.yaml (Prioritäts-Logik)

```bash
Input:  ./input
Output: ./output1, ./output2, s3://combined-bucket/output
Typ:    filesystem + s3
Config: .env (hohe Priorität) + env.yaml (niedrige Priorität)
```

- Testet Konfigurationshierarchie (.env überschreibt env.yaml)
- Multi-Target-Setup (Filesystem + S3)
- Prioritäts-Validierung

---

## 🧹 Cleanup & Utilities

### clean.sh

#### Interaktives Aufräumen

- Interaktive Auswahl der zu entfernenden Komponenten
- Sicherheitsabfragen vor destructive Operationen
- Entwicklerfreundlich für manuelle Nutzung

### clean-auto.sh

#### Automatisches Aufräumen

- Vollständiges, nicht-interaktives Cleanup
- Entfernt alle Test-Artefakte und Prozesse
- Stellt Original-Konfiguration wieder her
- Ideal für CI/CD oder nach Test-Suites

### test-overview.sh

#### Test-Suite Übersicht & Runner

```bash
# Übersicht anzeigen
./test-overview.sh

# Alle Tests automatisch ausführen
./test-overview.sh --run-all
```

- Zeigt alle verfügbaren Tests und deren Zweck
- Quick-Start Anweisungen
- Automatischer Test-Runner für alle Skripte
- Intelligente MinIO-Erkennung

---

## 🚀 Quick Start

### Einfacher Filesystem-Test

```bash
./test-fs-env.sh
```

### S3-Test (MinIO muss laufen)

```bash
# MinIO starten falls nicht aktiv
docker run -d -p 9000:9000 -p 9001:9001 --name minio \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio server /data --console-address ':9001'

# S3-Test ausführen  
./test-s3-env.sh
```

### Vollständige Test-Suite

```bash
./test-overview.sh --run-all
```

### Aufräumen nach Tests

```bash
./clean-auto.sh
```

---

## 🔨 Build-Integration

Alle Test-Skripte nutzen automatisches Build-Management:

1. **Fresh Build**: `go build -o file-shifter .`
2. **Build-Validierung**: Test stoppt bei Build-Fehlern
3. **Isolierte Binaries**: Jeder Test verwendet eigene Binary
4. **Automatisches Cleanup**: Binary wird nach Test entfernt

---

## 📋 Legacy Tests (deprecated)

Die folgenden Tests sind noch vorhanden, aber nicht mehr Teil der aktuellen Test-Suite:

- `test.sh` - Alter Filesystem-Test
- `test-s3.sh` - Alter S3-Test  
- `test-yaml-s3.sh` - Alter YAML+S3-Test

**Empfehlung**: Nutzen Sie die neue systematische Test-Suite für konsistente Ergebnisse.

---

## 🎯 Test-Philosophie

### Isolation

- Jeder Test läuft in isolierter Umgebung
- Keine Abhängigkeiten zwischen Tests
- Sauberer Zustand vor und nach jedem Test

### Non-destructive

- Original-Konfigurationsdateien werden gesichert
- Vollständige Wiederherstellung nach Tests
- Workspace bleibt unverändert

### Self-contained

- Automatische Dependency-Prüfung (z.B. MinIO)
- Build-Integration ohne externe Dependencies
- Klare Fehlermeldungen bei fehlenden Voraussetzungen

### Comprehensive

- Abdeckung aller Konfigurationsmethoden
- Test verschiedener Zieltypen
- Prioritäts- und Hierarchie-Tests

## Cleanup Scripts

### `./clean.sh`

#### Interaktives Cleanup

- Benutzerinteraktion für optionale Schritte
- Binärdateien und Docker-Resources optional
- Git-Repository-Reset optional
- Detaillierte Status-Ausgabe

### `./clean-auto.sh`

#### Automatisches Cleanup

- Keine Benutzerinteraktion
- Schnell für CI/CD-Pipelines
- Säubert nur Test-relevante Dateien

## Anwendungs-Scripts

### `./file-shifter`

#### Kompilierte Binärdatei

- Erstellt mit: `go build -o file-shifter .`
- Direkte Ausführung der Anwendung

## Docker Scripts

### `docker-compose.yaml`

#### Development-Setup

```bash
docker-compose up -d        # Alle Services starten
docker-compose up -d minio  # Nur MinIO starten
docker-compose down -v      # Services stoppen + Volumes
```

### `docker-compose.prod.yaml`

#### Production-Setup

```bash
docker-compose -f docker-compose.prod.yaml up -d
```

## Typischer Workflow

```bash
# 1. Basis-Test durchführen
./test.sh

# 2. Cleanup
./clean-auto.sh

# 3. S3-Test durchführen  
./test-s3.sh

# 4. Finales Cleanup
./clean.sh
```

## Debugging

```bash
# Anwendung mit Debug-Logs starten
LOG_LEVEL=DEBUG ./file-shifter

# Laufende Prozesse prüfen
ps aux | grep file-shifter

# Docker Services prüfen
docker-compose ps
```
