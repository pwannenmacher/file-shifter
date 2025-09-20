# File Shifter

Ein robuster File-Transfer-Service, der Dateien aus einem Input-Directory
automatisch zu mehreren Zielen kopiert und dabei die ursprüngliche
Dateistruktur erhält.

## 🎯 Übersicht

File Shifter überwacht ein definiertes Input-Verzeichnis und kopiert neue
Dateien automatisch zu beliebig vielen konfigurierten Zielen. Nach erfolgreichem
Transfer zu allen Zielen wird die Originaldatei automatisch entfernt.

### ✨ Key Features

- **🎯 Multi-Target-Support**: Gleichzeitiges Kopieren zu mehreren Zielen
- **📁 Unterstützte Zieltypen**:
  - Lokales Dateisystem
  - S3-kompatible Storage (MinIO, AWS S3, etc.)
  - SFTP/FTP-Server
- **🔄 Realtime-Processing**: File-System-Watcher für sofortige Verarbeitung
- **📂 Pfad-Erhaltung**: Relative Verzeichnisstruktur bleibt erhalten
- **⚡ Attribute-Erhaltung**: Dateiberechtigungen und Zeitstempel (bei Filesystem)
- **🛡️ Robuste Fehlerbehandlung**: Atomic Operations und Rollback
- **🐳 Docker-Ready**: Vollständige Container-Unterstützung
- **🔧 Zero-Configuration**: Funktioniert ohne Konfiguration mit sinnvollen Defaults

## 🚀 Quick Start

### Standard-Setup (Zero Configuration)

```bash
# Repository klonen
git clone <repository-url>
cd file-shifter

# Anwendung bauen und starten
go build -o file-shifter .
./file-shifter
```

**Standard-Verhalten ohne Konfiguration:**

- Input-Directory: `./input`
- Output-Directory: `./output`
- Typ: Filesystem

## ⚙️ Konfiguration

File Shifter unterstützt mehrere Konfigurationsmethoden mit folgender Priorität:

1. **Environment-Variablen** (höchste Priorität)
2. **env.yaml** (mittlere Priorität)
3. **Standard-Defaults** (niedrigste Priorität)

### 🔧 Environment-Variablen (.env)

#### Basis-Konfiguration

```bash
# Logging
LOG_LEVEL=INFO

# Input-Verzeichnis
INPUT_DIRECTORY=./input

# Output-Targets
OUTPUT_TARGETS_1_PATH=./output1
OUTPUT_TARGETS_1_TYPE=filesystem

OUTPUT_TARGETS_2_PATH=./output2
OUTPUT_TARGETS_2_TYPE=filesystem
```

#### S3-Targets

```bash
# S3-Ziel konfigurieren
OUTPUT_TARGETS_1_PATH=s3://my-bucket/uploads
OUTPUT_TARGETS_1_TYPE=s3

# S3-Verbindungsparameter
OUTPUT_CONFIG_S3_ENDPOINT=s3.amazonaws.com
OUTPUT_CONFIG_S3_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
OUTPUT_CONFIG_S3_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
OUTPUT_CONFIG_S3_REGION=eu-central-1
OUTPUT_CONFIG_S3_SSL=true
```

#### MinIO-Setup (S3-kompatibel)

```bash
# MinIO-Ziel (lokale S3-Alternative)
OUTPUT_TARGETS_1_PATH=s3://test-bucket/files
OUTPUT_TARGETS_1_TYPE=s3

# MinIO-Verbindungsparameter
OUTPUT_CONFIG_S3_ENDPOINT=localhost:9000
OUTPUT_CONFIG_S3_ACCESS_KEY=minioadmin
OUTPUT_CONFIG_S3_SECRET_KEY=minioadmin
OUTPUT_CONFIG_S3_REGION=us-east-1
OUTPUT_CONFIG_S3_SSL=false
```

#### FTP/SFTP-Targets

```bash
# SFTP-Ziel
OUTPUT_TARGETS_1_PATH=sftp://server.example.com/uploads
OUTPUT_TARGETS_1_TYPE=ftp

# FTP-Ziel
OUTPUT_TARGETS_2_PATH=ftp://ftp.example.com/files
OUTPUT_TARGETS_2_TYPE=ftp

# FTP-Verbindungsparameter
OUTPUT_CONFIG_FTP_HOST=server.example.com
OUTPUT_CONFIG_FTP_USERNAME=ftpuser
OUTPUT_CONFIG_FTP_PASSWORD=secret123
```

### 📄 YAML-Konfiguration (env.yaml)

#### Basis-Setup

```yaml
log:
  level: INFO

input:
  directory: ./input

output:
  targets:
    - path: ./output1
      type: filesystem
    - path: ./output2
      type: filesystem
```

#### Multi-Target-Setup mit S3

```yaml
log:
  level: INFO

input:
  directory: ./watch-folder

output:
  targets:
    # Lokale Backups
    - path: ./backup/local
      type: filesystem
    - path: /mnt/network-drive/backup
      type: filesystem
    
    # Cloud Storage
    - path: s3://production-bucket/files
      type: s3

  config:
    s3:
      endpoint: s3.amazonaws.com
      access-key: AKIAIOSFODNN7EXAMPLE
      secret-key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
      ssl: true
      region: eu-central-1
```

#### FTP/SFTP-Setup

```yaml
log:
  level: INFO

input:
  directory: ./uploads

output:
  targets:
    - path: sftp://secure-server.com/incoming
      type: ftp
    - path: ftp://backup-server.com/files
      type: ftp

  config:
    ftp:
      host: secure-server.com
      username: transfer-user
      password: secure-password
```

## 🐳 Docker Setup

### MinIO + File Shifter

#### docker-compose.yaml (Development)

```yaml
version: '3.8'

services:
  minio:
    image: quay.io/minio/minio
    container_name: minio
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    command: server /data --console-address ":9001"
    volumes:
      - minio-data:/data

  file-shifter:
    build: .
    container_name: file-shifter
    depends_on:
      - minio
    volumes:
      - ./input:/app/input
      - ./output:/app/output
      - ./env.yaml:/app/env.yaml:ro
    environment:
      - LOG_LEVEL=INFO
    restart: unless-stopped

volumes:
  minio-data:
```

#### Entwicklung starten

```bash
# Services starten
docker-compose up -d

# MinIO Web-UI öffnen
open http://localhost:9001
# Login: minioadmin / minioadmin

# Logs verfolgen
docker-compose logs -f file-shifter
```

### Produktions-Setup

```yaml
version: '3.8'

services:
  file-shifter:
    image: file-shifter:latest
    container_name: file-shifter-prod
    volumes:
      - /data/input:/app/input
      - /data/backup:/app/backup
      - ./env.yaml:/app/env.yaml:ro
    environment:
      - LOG_LEVEL=INFO
    restart: always
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

## 🔧 Build & Installation

### Lokale Entwicklung

```bash
# Dependencies installieren
go mod download

# Anwendung bauen
go build -o file-shifter .

# Tests ausführen (siehe SCRIPTS.md)
./test-overview.sh --run-all

# Aufräumen
./clean-auto.sh
```

### Binary-Installation

```bash
# Release-Build erstellen
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o file-shifter .

# Nach /usr/local/bin kopieren
sudo cp file-shifter /usr/local/bin/
```

## 📊 Monitoring & Logging

### Log-Level

```bash
# Environment-Variable
export LOG_LEVEL=DEBUG  # DEBUG, INFO, WARN, ERROR
```

### Beispiel-Logs

```text
time=2025-09-19T22:30:18.767+02:00 level=INFO msg="Worker gestartet"
time=2025-09-19T22:30:18.767+02:00 level=INFO msg="File-Watcher gestartet"
time=2025-09-19T22:30:19.269+02:00 level=INFO msg="Neue Datei erkannt"
time=2025-09-19T22:30:19.270+02:00 level=INFO msg="Datei erfolgreich kopiert"
time=2025-09-19T22:30:19.287+02:00 level=INFO msg="Datei erfolgreich hochgeladen"
time=2025-09-19T22:30:19.288+02:00 level=INFO msg="Datei erfolgreich verarbeitet"
```

## 🧪 Testing

Für umfassende Tests und Beispiele siehe **[SCRIPTS.md](SCRIPTS.md)**

### Quick-Tests

```bash
# Filesystem-Test
./test-fs-env.sh

# S3-Test (MinIO erforderlich)
./test-s3-env.sh

# Alle Tests
./test-overview.sh --run-all

# Aufräumen
./clean-auto.sh
```

## 🔒 Sicherheit

### Produktions-Überlegungen

- **Credentials**: Verwenden Sie sichere Passwörter und Access-Keys
- **Network**: Beschränken Sie Netzwerkzugriff auf notwendige Ports
- **File Permissions**: Setzen Sie restriktive Dateiberechtigungen
- **Monitoring**: Überwachen Sie Logs auf Anomalien

## 📝 Beispiel-Workflows

### Backup-Workflow

```yaml
# Automatisches Backup zu mehreren Zielen
input:
  directory: /data/incoming

output:
  targets:
    - path: /backup/local/daily
      type: filesystem
    - path: s3://backup-bucket/daily
      type: s3
    - path: sftp://offsite-server.com/backup
      type: ftp
```

### Development-Workflow

```bash
# 1. Entwicklungsumgebung starten
docker-compose up -d

# 2. Test-Dateien erstellen
mkdir -p input
echo "Test content" > input/test.txt

# 3. Verarbeitung überwachen
docker-compose logs -f file-shifter

# 4. Ergebnis prüfen
ls -la output/
```

## 🤝 Contributing

1. Fork das Repository
2. Feature-Branch erstellen (`git checkout -b feature/amazing-feature`)
3. Änderungen committen (`git commit -m 'Add amazing feature'`)
4. Branch pushen (`git push origin feature/amazing-feature`)
5. Pull Request öffnen

## 📄 License

Dieses Projekt steht unter der [MIT License](LICENSE).

## 🙋‍♂️ Support

Bei Fragen oder Problemen:

1. Überprüfen Sie die [SCRIPTS.md](SCRIPTS.md) für Test-Beispiele
2. Prüfen Sie die Logs auf Fehlermeldungen
3. Erstellen Sie ein Issue mit detaillierter Beschreibung

---

**File Shifter** - Zuverlässiger, automatisierter File-Transfer für moderne Infrastrukturen.
