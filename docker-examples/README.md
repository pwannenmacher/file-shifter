# Docker Compose Examples

✅ **Diese Dateien verwenden die aktuelle Konfigurationsstruktur!**

**Für vollständige Demo-Umgebung**: Siehe [`../demo/docker-compose.yaml`](../demo/docker-compose.yaml) mit MinIO, SFTP
und FTP-Servern.

## 📁 Dateien in diesem Ordner

| Datei                              | Status    | Beschreibung                              |
|------------------------------------|-----------|-------------------------------------------|
| `docker-compose.modern-env.yaml`   | ✅ Aktuell | YAML-basierte OUTPUTS-Konfiguration       |
| `docker-compose.env-file.yaml`     | ✅ Aktuell | Externe .env-Datei Konfiguration          |
| `docker-compose.env-included.yaml` | ✅ Aktuell | Flache ENV-Variable-Struktur (OUTPUT_X_*) |
| `docker-compose.prod.yaml`         | ✅ Aktuell | Produktions-Setup mit Pre-built Image     |

## ✅ Moderne Konfigurationsstrukturen

Diese Dateien verwenden die aktuelle Konfiguration:

- `OUTPUT_X_PATH` / `OUTPUT_X_TYPE` statt veraltete `OUTPUTS_X_*` Namen
- Direkte S3-Konfiguration pro Output (`OUTPUT_X_ENDPOINT`, etc.)
- Moderne YAML-Syntax und Container-Images
- Korrekte FILE_STABILITY-Parameter
- Sichere Port-Bindings (127.0.0.1)

## 🎯 Verwendungszwecke

### `docker-compose.modern-env.yaml`

**Entwicklung** - YAML-basierte OUTPUTS-Konfiguration

```bash
cd docker-examples
docker compose -f docker-compose.modern-env.yaml up -d
```

### `docker-compose.env-file.yaml`

**Externe Konfiguration** - Für .env-Dateien

```bash
# .env-Datei erstellen (siehe Kommentare in Datei)
docker compose -f docker-compose.env-file.yaml up -d
```

### `docker-compose.env-included.yaml`

**Flache ENV-Struktur** - Direkte ENV-Variablen (OUTPUT_X_ENDPOINT, etc.)

```bash
docker compose -f docker-compose.env-included.yaml up -d
```

### `docker-compose.prod.yaml`

**Produktion** - Mit Pre-built Image und Resource-Limits

```bash
# Produktions-.env erstellen (siehe Kommentare in Datei)
docker compose -f docker-compose.prod.yaml up -d
```

## 🆚 Unterschied zur Demo

**Demo-Setup** (`../demo/`) bietet:

- ✅ Vollständige Service-Integration (MinIO + SFTP + FTP)
- ✅ Vorkonfigurierte Multi-Target-Tests
- ✅ Detaillierte Setup-Anleitung

**Diese Examples** bieten:

- ✅ Verschiedene Konfigurationsmuster
- ✅ Produktions-orientierte Setups
- ✅ Modulare, anpassbare Vorlagen

## 💡 Empfehlung

- **Für schnelle Tests**: Verwenden Sie [`../demo/docker-compose.yaml`](../demo/docker-compose.yaml)
- **Für eigene Setups**: Verwenden Sie diese Examples als Vorlage
