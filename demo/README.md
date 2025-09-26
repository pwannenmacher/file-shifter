# File Shifter - Demo Setup

## 🚀 Quick Start

Diese Demo zeigt File Shifter mit allen unterstützten Zieltypen: lokales Filesystem, MinIO S3, SFTP und FTP.

```bash
# Demo starten
docker compose up -d

# Test-Datei erstellen
echo "Demo Test $(date)" > input/demo-test.txt

# Verarbeitung beobachten
docker compose logs -f shifter
```

## 📁 Verzeichnisberechtigungen

**Wichtig**: Die Input- und Output-Verzeichnisse müssen die richtigen Berechtigungen haben:

```bash
# Berechtigungen für Demo-Verzeichnisse setzen
chmod 777 input output1 output2

# Falls Verzeichnisse nicht existieren, erstellen
mkdir -p input output1 output2
chmod 777 input output1 output2
```

## 🔧 Service-Konfiguration

### MinIO S3 Storage

- **Web-UI**: <http://localhost:9000>
- **Credentials**: `minioadmin` / `minioadmin`
- **Buckets**: `bucket1`, `bucket2` werden automatisch erstellt

### SFTP Server (SFTPGo)

- **Port**: 2022
- **Admin-UI**: <http://localhost:8080>
- **Admin-Login**: `admin` / `admin123`

**Benutzer konfigurieren:**

1. Admin-UI öffnen: <http://localhost:8080>
2. Mit `admin` / `admin123` anmelden
3. **Users** → **Add User**
4. Benutzerdaten eingeben:
   - **Username**: `sftp`
   - **Password**: `sftp`
   - **Home Directory**: `/srv/sftpgo/data`
   - **Permissions**: Alle aktivieren (Read, Write, Create dirs, etc.)
5. Speichern

### FTP Server (SFTPGo)

- **Port**: 2021
- **Admin-UI**: <http://localhost:8081>
- **Admin-Login**: `admin` / `admin123`

**Benutzer konfigurieren:**

1. Admin-UI öffnen: <http://localhost:8081>
2. Mit `admin` / `admin123` anmelden
3. **Users** → **Add User**
4. Benutzerdaten eingeben:
   - **Username**: `ftp`
   - **Password**: `ftp`
   - **Home Directory**: `/srv/sftpgo/data`
   - **Permissions**: Alle aktivieren (Read, Write, Create dirs, etc.)
5. Speichern

## 🎯 Demo-Targets

Die Demo ist konfiguriert für folgende Ziele:

```yaml
# Lokale Dateisysteme
- ./output1 (Volume-Mount)
- ./output2 (Volume-Mount)

# S3-Storage (MinIO)
- s3://bucket1 (automatisch erstellt)
- s3://bucket2 (automatisch erstellt)

# SFTP-Server
- sftp://sftp:2022/uploads (Benutzer muss angelegt werden)

# FTP-Server  
- ftp://ftp:2121/uploads (Benutzer muss angelegt werden)
```

## ✅ Funktionstest

```bash
# 1. Demo starten
docker compose up -d

# 2. Services initialisieren (warten bis alle gesund sind)
docker compose ps

# 3. FTP/SFTP-Benutzer über Web-UIs anlegen (siehe oben)

# 4. Test-Datei erstellen
echo "Funktionstest $(date)" > input/test-$(date +%s).txt

# 5. Verarbeitung beobachten
docker compose logs -f shifter

# 6. Ergebnisse prüfen
ls -la output1/ output2/
# MinIO: http://localhost:9000 (Browse bucket1, bucket2)
# SFTP/FTP: Über Admin-UIs oder File-Browser
```

## 🐛 Troubleshooting

### Berechtigungsfehler

```bash
# Alle Demo-Verzeichnisse zurücksetzen
sudo rm -rf output1 output2 input
mkdir -p input output1 output2
chmod 777 input output1 output2
```

### SFTP/FTP-Verbindungsfehler

- Überprüfen ob Benutzer in den Admin-UIs angelegt wurden
- Home-Directory muss existieren (`/srv/sftpgo/data`)
- Alle Berechtigungen aktiviert (Read, Write, Create directories, etc.)

### MinIO-Verbindungsfehler

```bash
# MinIO-Status prüfen
docker compose logs minio

# MinIO neustarten
docker compose restart minio
```

### File Shifter Debug

```bash
# Debug-Logs aktivieren
# In docker-compose.yaml: LOG_LEVEL: DEBUG

# Container-Logs anzeigen
docker compose logs shifter

# Container neu starten
docker compose restart shifter
```

## 📊 Monitoring

```bash
# Alle Services anzeigen
docker compose ps

# Logs verfolgen
docker compose logs -f

# Einzelne Service-Logs
docker compose logs minio
docker compose logs sftp
docker compose logs ftp
docker compose logs shifter
```

## 🧹 Aufräumen

```bash
# Services stoppen und Volumes entfernen
docker compose down -v

# Lokale Test-Dateien entfernen
rm -rf input/* output1/* output2/*
```

## 🔧 Konfiguration anpassen

Die Demo-Konfiguration ist in `docker-compose.yaml` definiert. Wichtige Environment-Variablen:

```yaml
environment:
  LOG_LEVEL: INFO                    # DEBUG für mehr Details
  INPUT: /app/input
  FILE_STABILITY_MAX_RETRIES: 3      # Anzahl Wiederholungen
  FILE_STABILITY_CHECK_INTERVAL: 1   # Prüf-Intervall in Sekunden
  FILE_STABILITY_PERIOD: 1          # Stabilität-Periode in Sekunden
  OUTPUTS: |                        # Multi-Target-Konfiguration
    - path: "/app/output1"
      type: "filesystem"
    # ... weitere Targets
```

Für Produktions-Setup siehe die Hauptdokumentation: [`../README.md`](../README.md)
