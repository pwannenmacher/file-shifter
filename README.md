# File Shifter

[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=pwannenmacher_file-shifter&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=pwannenmacher_file-shifter) [![Coverage](https://sonarcloud.io/api/project_badges/measure?project=pwannenmacher_file-shifter&metric=coverage)](https://sonarcloud.io/summary/new_code?id=pwannenmacher_file-shifter) [![Vulnerabilities](https://sonarcloud.io/api/project_badges/measure?project=pwannenmacher_file-shifter&metric=vulnerabilities)](https://sonarcloud.io/summary/new_code?id=pwannenmacher_file-shifter) [![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=pwannenmacher_file-shifter&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=pwannenmacher_file-shifter)

A robust file transfer service that automatically copies files from an input directory to multiple destinations while
preserving the original file structure.

## Overview

File Shifter monitors a defined input directory and automatically copies new files to any number of configured
destinations. After successful transfer to all destinations, the original file is automatically removed.

### ✨ Key Features

- **🎯 Multi-Target Support**: Simultaneous copying to multiple destinations
- **📁 Supported destination types**:
    - Local filesystem
    - S3-compatible storage (MinIO, AWS S3, etc.)
    - SFTP/FTP servers
- **🔄 Real-time processing**: File system watcher for immediate processing
- **📂 Path preservation**: Relative directory structure is maintained
- **⚡ Attribute preservation**: File permissions and timestamps (for filesystem)
- **🛡️ Robust error handling**: Atomic operations and rollback
- **🐳 Docker-ready**: Full container support
- **🔧 Zero-configuration**: Works without configuration with sensible defaults

## Quick Start

```bash
# Clone and build
git clone <repository-url>
cd file-shifter
go build -o file-shifter .
./file-shifter
```

Without configuration, files are copied from `./input` to `./output`.

## Configuration

File Shifter supports multiple configuration methods with the following priority:

1. Command line parameters (highest)
2. Environment variables
3. `env.yaml` file
4. Default values (lowest)

### Command Line Parameters

```bash
# Show help
./file-shifter --help
./file-shifter -h

# Set log level
./file-shifter --log-level DEBUG

# Set input directory
./file-shifter --input ./my-input

# Define output targets as JSON
./file-shifter --outputs '[{"path":"./backup","type":"filesystem"}]'
```

#### JSON Format for --outputs

**Filesystem:**

```json
[
  {
    "path": "./backup",
    "type": "filesystem"
  }
]
```

**S3:**

```json
[
  {
    "path": "s3://bucket/prefix",
    "type": "s3",
    "endpoint": "s3.amazonaws.com",
    "access-key": "ACCESS_KEY",
    "secret-key": "SECRET_KEY",
    "ssl": true,
    "region": "eu-central-1"
  }
]
```

**SFTP:**

```json
[
  {
    "path": "sftp://server/path",
    "type": "sftp",
    "host": "server.com",
    "username": "user",
    "password": "password"
  }
]
```

#### Examples

**Simple filesystem backup:**

```bash
./file-shifter --input ./data --outputs '[{"path":"./backup","type":"filesystem"}]'
```

**Multi-target with S3 and filesystem:**

```bash
./file-shifter --input ./uploads --outputs '[
  {"path":"./local-backup","type":"filesystem"},
  {"path":"s3://my-bucket/files","type":"s3","endpoint":"localhost:9000","access-key":"minioadmin","secret-key":"minioadmin","ssl":false,"region":"us-east-1"}
]'
```

### Environment Variables

**Flat structure:**

```bash
# Logging
LOG_LEVEL=INFO

# Input directory
INPUT=./input

# Output target 1: Filesystem
OUTPUT_1_PATH=./output1
OUTPUT_1_TYPE=filesystem

# Output target 2: Filesystem  
OUTPUT_2_PATH=./output2
OUTPUT_2_TYPE=filesystem

# Output target 3: S3/MinIO
OUTPUT_3_PATH=s3://my-bucket/uploads
OUTPUT_3_TYPE=s3
OUTPUT_3_ENDPOINT=localhost:9000
OUTPUT_3_ACCESS_KEY=minioadmin
OUTPUT_3_SECRET_KEY=minioadmin
OUTPUT_3_SSL=false
OUTPUT_3_REGION=eu-central-1

# Output target 4: SFTP
OUTPUT_4_PATH=sftp://server.example.com/uploads
OUTPUT_4_TYPE=sftp
OUTPUT_4_HOST=server.example.com
OUTPUT_4_USERNAME=ftpuser
OUTPUT_4_PASSWORD=secret123

# Output target 5: FTP
OUTPUT_5_PATH=ftp://ftp.example.com/files
OUTPUT_5_TYPE=ftp
OUTPUT_5_HOST=ftp.example.com
OUTPUT_5_USERNAME=ftpuser
OUTPUT_5_PASSWORD=secret123

# File Stability Configuration
FILE_STABILITY_MAX_RETRIES=30
FILE_STABILITY_CHECK_INTERVAL=100
FILE_STABILITY_PERIOD=200

# Worker pool configuration for parallel processing
WORKER_POOL_WORKERS=8
WORKER_POOL_QUEUE_SIZE=100
```

**JSON structure:**

```bash
# Logging
LOG_LEVEL=INFO

# Input directory
INPUT=./input

# Outputs
OUTPUTS=[{"path":"./output1","type":"filesystem"},{"path":"s3://bucket","type":"s3"}]

# Global S3 configuration (for all S3 targets)
S3_ENDPOINT=localhost:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_USE_SSL=false
S3_REGION=eu-central-1

# Global FTP configuration (for all FTP/SFTP targets)
FTP_HOST=server.example.com
FTP_USERNAME=ftpuser
FTP_PASSWORD=secret123

# File Stability Configuration
FILE_STABILITY_MAX_RETRIES=30
FILE_STABILITY_CHECK_INTERVAL=100
FILE_STABILITY_PERIOD=200

# Worker pool configuration for parallel processing
WORKER_POOL_WORKERS=8
WORKER_POOL_QUEUE_SIZE=200
```

**YAML Configuration (env.yaml):**

```yaml
log:
  level: INFO

# Input as direct string
input: ./input

# Output as direct array (without 'targets' wrapper)
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
    host: your-sftp-host
    username: your-username
    password: your-password
  - path: ftp://my-server2/output6
    type: ftp
    host: your-ftp-host
    username: your-username
    password: your-password

# File Stability Configuration
file-stability:
  max-retries: 30      # Maximum number of repetitions (default: 30)
  check-interval: 100  # Check interval in milliseconds (default: 1000 ms = 1 s)
  stability-period: 200  # Stability check in milliseconds (default: 1000 ms = 1 s)

# Worker pool configuration for parallel processing
worker-pool:
  workers: 8           # Number of parallel workers (default: 4)
  queue-size: 200      # Size of the file queue (default: 100)
```

#### Practical Examples

**Simple backup setup:**

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

**Multi-cloud setup:**

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

## Docker

### Demo Setup

```bash
cd demo
docker compose up -d
```

This starts File Shifter with MinIO S3, SFTP, and FTP servers for testing.

### Production

```yaml
services:
  file-shifter:
    image: pwannenmacher/file-shifter:latest
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
```

## Build & Installation

```bash
git clone <repository-url>
cd file-shifter

go mod download

go build -o file-shifter .

./file-shifter
```

### Testing

```bash
# Switch to test-scripts folder
cd test-scripts

# Simple test
./test-fs-env.sh

# Run all tests
./test-overview.sh --run-all

# Clean up
./clean-auto.sh
```

See [`test-scripts/README.md`](test-scripts/README.md) for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Open a pull request

## License

MIT License. See [LICENSE](LICENSE) for details.

## Support

For issues or questions:

1. Check [test-scripts/README.md](test-scripts/README.md) for examples
2. Review logs for errors
3. Create an issue with details

---

**File Shifter** - Reliable, automated file transfer for modern infrastructures.
