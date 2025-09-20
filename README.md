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

1. **Kommandozeilen-Parameter** (höchste Priorität)
2. **Environment-Variablen** (hohe Priorität)
3. **env.yaml** (mittlere Priorität)  
4. **Standard-Defaults** (niedrigste Priorität)

### 🖥️ Kommandozeilen-Parameter

File Shifter kann vollständig über Kommandozeilen-Parameter konfiguriert werden:

#### Grundlegende Parameter

```bash
# Hilfe anzeigen
./file-shifter --help
./file-shifter -h
./file-shifter ?
./file-shifter -?

# Log-Level setzen
./file-shifter --log-level DEBUG

# Input-Verzeichnis setzen
./file-shifter --input ./my-input

# Output-Targets als JSON definieren
./file-shifter --outputs '[{"path":"./backup","type":"filesystem"}]'
```

#### Vollständige Beispiele

**Einfaches Filesystem-Backup:**

```bash
./file-shifter \
  --input ./data \
  --outputs '[{"path":"./backup","type":"filesystem"}]'
```

**Multi-Target mit S3 und Filesystem:**

```bash
./file-shifter \
  --log-level INFO \
  --input ./uploads \
  --outputs '[
    {"path":"./local-backup","type":"filesystem"},
    {"path":"s3://my-bucket/files","type":"s3",
     "endpoint":"localhost:9000","access-key":"minioadmin",
     "secret-key":"minioadmin","ssl":false,"region":"us-east-1"}
  ]'
```

**Debug-Modus mit SFTP:**

```bash
./file-shifter \
  --log-level DEBUG \
  --input ./data \
  --outputs '[
    {"path":"sftp://server.com/backup","type":"sftp",
     "host":"server.com","username":"user","password":"pass"}
  ]'
```

#### Parameter-Referenz

| Parameter | Beschreibung | Format | Beispiel |
|-----------|--------------|--------|----------|
| `--log-level` | Log-Level festlegen | `DEBUG\|INFO\|WARN\|ERROR` | `--log-level INFO` |
| `--input` | Input-Verzeichnis | Pfad-String | `--input ./data` |
| `--outputs` | Output-Targets | JSON-Array | `--outputs '[{"path":"./out","type":"filesystem"}]'` |
| `--help`, `-h` | Hilfe anzeigen | - | `--help` |

#### JSON-Format für --outputs

Das `--outputs` Parameter erwartet ein JSON-Array mit Output-Target-Objekten:

**Filesystem:**

```json
[{"path":"./backup","type":"filesystem"}]
```

**S3/MinIO:**

```json
[{
  "path":"s3://bucket/prefix",
  "type":"s3",
  "endpoint":"s3.amazonaws.com",
  "access-key":"ACCESS_KEY",
  "secret-key":"SECRET_KEY",
  "ssl":true,
  "region":"eu-central-1"
}]
```

**SFTP:**

```json
[{
  "path":"sftp://server/path",
  "type":"sftp",
  "host":"server.com",
  "username":"user",
  "password":"password"
}]
```

**FTP:**

```json
[{
  "path":"ftp://server/path",
  "type":"ftp",
  "host":"ftp.server.com",
  "username":"user",
  "password":"password"
}]
```

### 🔧 Environment-Variablen (.env)

File Shifter unterstützt zwei ENV-Variable-Strukturen:

#### 🆕 Neue flache Struktur (empfohlen)

Die neue Struktur ist konsistent mit der YAML-Konfiguration und ermöglicht unterschiedliche S3-Konfigurationen pro Output-Ziel:

```bash
# Logging
LOG_LEVEL=INFO

# Input-Verzeichnis
INPUT=./input

# Output-Ziel 1: Filesystem
OUTPUT_1_PATH=./output1
OUTPUT_1_TYPE=filesystem

# Output-Ziel 2: Filesystem  
OUTPUT_2_PATH=./output2
OUTPUT_2_TYPE=filesystem

# Output-Ziel 3: S3/MinIO
OUTPUT_3_PATH=s3://my-bucket/uploads
OUTPUT_3_TYPE=s3
OUTPUT_3_ENDPOINT=localhost:9000
OUTPUT_3_ACCESS_KEY=minioadmin
OUTPUT_3_SECRET_KEY=minioadmin
OUTPUT_3_SSL=false
OUTPUT_3_REGION=eu-central-1

# Output-Ziel 4: SFTP
OUTPUT_4_PATH=sftp://server.example.com/uploads
OUTPUT_4_TYPE=sftp
OUTPUT_4_HOST=server.example.com
OUTPUT_4_USERNAME=ftpuser
OUTPUT_4_PASSWORD=secret123

# Output-Ziel 5: FTP
OUTPUT_5_PATH=ftp://ftp.example.com/files
OUTPUT_5_TYPE=ftp
OUTPUT_5_HOST=ftp.example.com
OUTPUT_5_USERNAME=ftpuser
OUTPUT_5_PASSWORD=secret123
```

#### 🔄 Legacy JSON-Struktur (Rückwärtskompatibilität)

Die alte Struktur wird weiterhin unterstützt:

```bash
# Logging
LOG_LEVEL=INFO

# Input-Verzeichnis (alte Bezeichnung)
INPUT=./input

# Output-Targets als JSON-Array
OUTPUTS=[{"path":"./output1","type":"filesystem"},{"path":"./output2","type":"filesystem"},{"path":"s3://my-bucket/uploads","type":"s3"}]

# Globale S3-Konfiguration (für alle S3-Targets)
S3_ENDPOINT=localhost:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_USE_SSL=false
S3_REGION=eu-central-1

# Globale FTP-Konfiguration (für alle FTP/SFTP-Targets)
FTP_HOST=server.example.com
FTP_USERNAME=ftpuser
FTP_PASSWORD=secret123
```

### 📄 YAML-Konfiguration (env.yaml)

Die YAML-Konfiguration verwendet jetzt eine flache, einfache Struktur:

#### 🆕 Neue flache YAML-Struktur

```yaml
log:
  level: INFO

# Input als direkter String
input: ./input

# Output als direktes Array (ohne 'targets'-Wrapper)
output:
  - path: ./output1
    type: filesystem
  - path: ./output2
    type: filesystem
  - path: s3://my-bucket/output3
    type: s3
    endpoint: minio1:9000
    access-key: minioadmin
    secret-key: minioadmin
    ssl: false
    region: eu-central-1
  - path: s3://my-bucket/output4
    type: s3
    endpoint: minio2:9000
    access-key: minioadmin
    secret-key: minioadmin
    ssl: false
    region: eu-central-1
  - path: sftp://my-server1/output5
    type: sftp
    host: your-ftp-host
    username: your-username
    password: your-password
  - path: ftp://my-server2/output6
    type: ftp
    host: your-ftp-host
    username: your-username
    password: your-password
```

#### 💡 Vorteile der neuen Struktur

- **Einfacher**: Weniger Verschachtelung, direktere Konfiguration
- **Konsistent**: ENV- und YAML-Struktur sind analog aufgebaut
- **Flexibel**: Unterschiedliche S3-Endpoints pro Output möglich
- **Skalierbar**: Beliebig viele Output-Ziele einfach hinzufügbar

### 🔄 Konfigurationspriorität und Kompatibilität

#### Prioritäts-System

File Shifter lädt Konfigurationen in folgender Reihenfolge:

1. **YAML-Datei laden** (`env.yaml` oder `env.yml`)
2. **Standard-Werte setzen** (falls Werte fehlen)
3. **ENV-Variablen laden** (überschreibt YAML-Werte)

#### ENV-Variable Priorität

Bei ENV-Variablen gilt folgende Priorität:

1. **Neue flache Struktur** (`OUTPUT_X_*`) - wird zuerst versucht
2. **Legacy JSON-Struktur** (`OUTPUTS`) - als Fallback verwendet
3. **Input-Variablen**: `INPUT` hat Priorität vor `INPUT`

#### Beispiel: Kombinierte Konfiguration

**env.yaml (Basis-Konfiguration):**

```yaml
log:
  level: DEBUG
input: ./yaml-input
output:
  - path: ./yaml-output
    type: filesystem
```

**.env (Überschreibt YAML):**

```bash
LOG_LEVEL=INFO
INPUT=./env-input
OUTPUT_1_PATH=./env-output1
OUTPUT_1_TYPE=filesystem
OUTPUT_2_PATH=./env-output2
OUTPUT_2_TYPE=filesystem
```

**Resultierende Konfiguration:**

- Log-Level: `INFO` (ENV überschreibt YAML)
- Input: `./env-input` (ENV überschreibt YAML)
- Outputs: `./env-output1` und `./env-output2` (ENV überschreibt YAML komplett)


#### 🔧 Praktische Beispiele

**Einfaches Backup-Setup:**

```yaml
log:
  level: INFO
input: ./incoming
output:
  - path: ./backup/local
    type: filesystem
  - path: s3://backup-bucket/files
    type: s3
    endpoint: s3.amazonaws.com
    access-key: YOUR_ACCESS_KEY
    secret-key: YOUR_SECRET_KEY
    ssl: true
    region: eu-central-1
```

**Multi-Cloud-Setup:**

```yaml
log:
  level: INFO
input: ./data
output:
  - path: s3://aws-bucket/data
    type: s3
    endpoint: s3.amazonaws.com
    access-key: AWS_ACCESS_KEY
    secret-key: AWS_SECRET_KEY
    ssl: true
    region: eu-central-1
  - path: s3://minio-bucket/data
    type: s3
    endpoint: minio.company.com:9000
    access-key: MINIO_ACCESS_KEY
    secret-key: MINIO_SECRET_KEY
    ssl: false
    region: us-east-1
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

#### Beispiel env.yaml für Docker

```yaml
log:
  level: INFO

input: ./input

output:
  # Lokales Backup
  - path: ./output
    type: filesystem
  
  # MinIO S3-kompatibles Storage
  - path: s3://docker-bucket/files
    type: s3
    endpoint: minio:9000
    access-key: minioadmin
    secret-key: minioadmin
    ssl: false
    region: us-east-1
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

#### Mit ENV-Variablen

```yaml
version: '3.8'

services:
  file-shifter:
    image: file-shifter:latest
    container_name: file-shifter-prod
    volumes:
      - /data/input:/app/input
      - /data/backup:/app/backup
    environment:
      - LOG_LEVEL=INFO
      - INPUT=/app/input
      - OUTPUT_1_PATH=/app/backup
      - OUTPUT_1_TYPE=filesystem
      - OUTPUT_2_PATH=s3://prod-bucket/files
      - OUTPUT_2_TYPE=s3
      - OUTPUT_2_ENDPOINT=s3.amazonaws.com
      - OUTPUT_2_ACCESS_KEY=${AWS_ACCESS_KEY}
      - OUTPUT_2_SECRET_KEY=${AWS_SECRET_KEY}
      - OUTPUT_2_SSL=true
      - OUTPUT_2_REGION=eu-central-1
    restart: always
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

#### Mit YAML-Konfiguration

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
log:
  level: INFO

input: /data/incoming

output:
  - path: /backup/local/daily
    type: filesystem
  - path: s3://backup-bucket/daily
    type: s3
    endpoint: s3.amazonaws.com
    access-key: YOUR_ACCESS_KEY
    secret-key: YOUR_SECRET_KEY
    ssl: true
    region: eu-central-1
  - path: sftp://offsite-server.com/backup
    type: sftp
    host: offsite-server.com
    username: backup-user
    password: secure-password
```

### Development-Workflow

#### Mit neuer ENV-Struktur

```bash
# 1. .env-Datei erstellen
cat > .env << 'EOF'
LOG_LEVEL=DEBUG
INPUT=./input
OUTPUT_1_PATH=./output
OUTPUT_1_TYPE=filesystem
OUTPUT_2_PATH=s3://dev-bucket/test
OUTPUT_2_TYPE=s3
OUTPUT_2_ENDPOINT=localhost:9000
OUTPUT_2_ACCESS_KEY=minioadmin
OUTPUT_2_SECRET_KEY=minioadmin
OUTPUT_2_SSL=false
OUTPUT_2_REGION=us-east-1
EOF

# 2. Entwicklungsumgebung starten
docker-compose up -d

# 3. Test-Dateien erstellen
mkdir -p input
echo "Test content" > input/test.txt

# 4. Verarbeitung überwachen
docker-compose logs -f file-shifter

# 5. Ergebnis prüfen
ls -la output/
```

### Produktions-Workflows

#### Legacy-Migration

Falls Sie eine bestehende Konfiguration migrieren möchten:

**Alte Struktur:**

```bash
INPUT=./input
OUTPUTS=[{"path":"./output","type":"filesystem"},{"path":"s3://bucket/files","type":"s3"}]
S3_ENDPOINT=s3.amazonaws.com
S3_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
S3_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

**Neue Struktur:**

```bash
INPUT=./input
OUTPUT_1_PATH=./output
OUTPUT_1_TYPE=filesystem
OUTPUT_2_PATH=s3://bucket/files
OUTPUT_2_TYPE=s3
OUTPUT_2_ENDPOINT=s3.amazonaws.com
OUTPUT_2_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
OUTPUT_2_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
OUTPUT_2_SSL=true
OUTPUT_2_REGION=eu-central-1
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
