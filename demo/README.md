# File Shifter Demo

## Quick Start

This demo shows File Shifter with all supported destination types.

```bash
docker compose up -d
echo "Demo Test $(date)" > input/demo-test.txt
docker compose logs -f shifter
```

## Directory Permissions

The input and output directories need proper permissions:

```bash
chmod 777 input output1 output2
# Or create if missing:
mkdir -p input output1 output2 && chmod 777 input output1 output2
```

## Services

### MinIO S3 Storage

- Web UI: <http://localhost:9000>
- Credentials: `minioadmin` / `minioadmin`

### SFTP Server (SFTPGo)

- Port: 2022
- Admin UI: <http://localhost:8080>
- Admin Login: `admin` / `admin123`

Create user: Username `sftp`, Password `sftp`, Home `/srv/sftpgo/data`

### FTP Server (SFTPGo)

- Port: 2021  
- Admin UI: <http://localhost:8081>
- Admin Login: `admin` / `admin123`

Create user: Username `ftp`, Password `ftp`, Home `/srv/sftpgo/data`

## Demo Targets

- Local: `./output1`, `./output2`
- S3: `s3://bucket1`, `s3://bucket2` (auto-created)
- SFTP: `sftp://sftp:2022/uploads` (create user first)
- FTP: `ftp://ftp:2121/uploads` (create user first)

## Testing

```bash
# Start demo
docker compose up -d

# Create users in SFTP/FTP admin UIs

# Test file transfer
echo "Test $(date)" > input/test-$(date +%s).txt
docker compose logs -f shifter

# Check results
ls -la output1/ output2/
# MinIO: http://localhost:9000
```

## Troubleshooting

**Permission errors:**

```bash
sudo rm -rf output1 output2 input
mkdir -p input output1 output2
chmod 777 input output1 output2
```

**Connection issues:**

- Ensure users are created in admin UIs
- Check container logs: `docker compose logs [service]`
- Restart services: `docker compose restart [service]`

