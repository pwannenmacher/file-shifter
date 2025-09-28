# File Shifter Test Suite

Comprehensive tests for all configuration methods and destination types. Tests are isolated and non-destructive.

## Quick Start

```bash
./test-fs-env.sh               # Simple filesystem test
./test-overview.sh --run-all   # Run all tests
./clean-auto.sh                # Clean up
```

## Test Categories

### Standard & Filesystem Tests

| Script                | Description                  | Configuration             | Details                                              |
|-----------------------|------------------------------|---------------------------|------------------------------------------------------|
| `test-default.sh`     | Zero-Configuration Test      | None (Standard defaults)  | Tests ./input → ./output, checks standard defaults |
| `test-fs-env.sh`      | Filesystem with ENV variables | `.env`                    | Multi-target filesystem setup, ENV priority         |
| `test-fs-yaml.sh`     | Filesystem with YAML          | `env.yaml`                | Structured YAML configuration                        |
| `test-fs-env-json.sh` | Filesystem with JSON ENV      | `.env` (JSON format)      | Legacy JSON structure (backward compatibility)       |

### S3 Tests (MinIO erforderlich)

| Script            | Description               | Configuration | Details                                            |
|-------------------|----------------------------|---------------|----------------------------------------------------|
| `test-s3-env.sh`  | S3/MinIO mit ENV-Variablen | `.env`        | S3-Integration über ENV, MinIO-Client Verifikation |
| `test-s3-yaml.sh` | S3/MinIO mit YAML          | `env.yaml`    | S3-Integration über YAML, Bucket-Verifikation      |

### Combined Tests

| Script             | Description         | Configuration       | Details                                                |
|--------------------|---------------------|---------------------|--------------------------------------------------------|
| `test-combined.sh` | Multi-target (FS + S3) | `.env` + `env.yaml` | Configuration hierarchy (.env overrides env.yaml)     |

### Special Tests

| Script               | Description       | Purpose                                   | Details                                 |
|----------------------|-------------------|-------------------------------------------|-----------------------------------------|
| `test-yml-format.sh` | YAML format test  | Validation of different YAML structures  | env.yml vs env.yaml, conflict detection |

## 🔧 Utilities

| Script             | Description              | Purpose                  |
|--------------------|---------------------------|-----------------------------|
| `test-overview.sh` | Test-Übersicht und Runner | Interaktiv oder `--run-all` |
| `clean.sh`         | Interaktives Aufräumen    | Benutzergeführt             |
| `clean-auto.sh`    | Automatisches Aufräumen   | CI/CD, nach Tests           |

## Prerequisites

### Basic tests (always available)

- Go 1.19+ installed
- Write permissions in workspace

### S3 tests (optional)

```bash
# Start MinIO
docker run -d -p 9000:9000 -p 9001:9001 \
  --name minio \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio server /data --console-address ':9001'
```

## Test Features

### Test Philosophy

- **Isolation**
  - Each test runs in an isolated environment
  - No dependencies between tests
  - Clean state before and after each test
- **Non-destructive**
  - Original configuration files are backed up
  - Complete restoration after tests
  - Workspace remains unchanged
- **Self-contained**
  - Automatic dependency checking (e.g. MinIO)
  - Build integration without external dependencies
  - Clear error messages for missing prerequisites

## Workflows

**Development:**

```bash
./test-fs-env.sh
./clean-auto.sh
```

**CI/CD:**

```bash
./test-overview.sh --run-all
./clean-auto.sh
```

**Debug:**

```bash
LOG_LEVEL=DEBUG ./test-fs-env.sh
./clean.sh  # manual cleanup if needed
```

## Troubleshooting

**Test fails:** Check with `LOG_LEVEL=DEBUG ./test-name.sh`
**MinIO not available:** `docker ps | grep minio`
**Build errors:** `go mod tidy && go build -o file-shifter ..`

## Contributing

When adding new tests:

1. Follow the naming scheme `test-[category]-[type].sh`
2. Implement backup/restore mechanism
3. Add cleanup logic
4. Document the test in this README
5. Test integration with `test-overview.sh`
