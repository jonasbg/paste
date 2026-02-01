---
title: Environment Variables
description: Configuration through environment variables
sidebar_position: 2
---

# Environment Variables

Paste can be configured through environment variables for both the CLI client and server.

## CLI Client

### PASTE_URL

The default server URL for uploads and downloads.

| Property | Value |
|----------|-------|
| Variable | `PASTE_URL` |
| Default | `https://paste.torden.tech` |
| Example | `https://my-paste-server.com` |

**Usage:**

```bash
# Set for current session
export PASTE_URL=https://my-paste-server.com
pastectl upload -f file.txt

# Set for single command
PASTE_URL=https://my-server.com pastectl upload -f file.txt

# Or use the --url flag
pastectl upload -f file.txt --url https://my-server.com
```

**Shell Configuration:**

```bash
# Add to ~/.bashrc or ~/.zshrc
export PASTE_URL=https://my-paste-server.com
```

## Server

### PORT

HTTP server port.

| Property | Value |
|----------|-------|
| Variable | `PORT` |
| Default | `8080` |
| Example | `3000` |

### UPLOAD_DIR

Directory for storing uploaded files.

| Property | Value |
|----------|-------|
| Variable | `UPLOAD_DIR` |
| Default | `./uploads` |
| Example | `/var/lib/paste/uploads` |

### MAX_FILE_SIZE

Maximum allowed file size. Supports raw bytes (number only), decimal units (KB, MB, GB, TB), or binary units (Ki, Mi, Gi, Ti, KiB, MiB, GiB, TiB).

| Property | Value |
|----------|-------|
| Variable | `MAX_FILE_SIZE` |
| Default | `100MB` |
| Examples | `5368709120` (raw bytes)<br/>`1073741824` (raw bytes)<br/>`1GB` (decimal)<br/>`5GiB` (binary)<br/>`500Mi` (binary) |

### CHUNK_SIZE

Upload chunk size in megabytes.

| Property | Value |
|----------|-------|
| Variable | `CHUNK_SIZE` |
| Default | `1` |
| Example | `2` |

### KEY_SIZE

Encryption key size in bits.

| Property | Value |
|----------|-------|
| Variable | `KEY_SIZE` |
| Default | `256` |
| Options | `128`, `192`, `256` |

### ID_SIZE

File ID size in bits.

| Property | Value |
|----------|-------|
| Variable | `ID_SIZE` |
| Default | `128` |
| Options | `64`, `128`, `192`, `256` |

### FILE_EXPIRY

File expiration time (if not downloaded).

| Property | Value |
|----------|-------|
| Variable | `FILE_EXPIRY` |
| Default | `24h` |
| Example | `48h`, `7d` |

### RATE_LIMIT

Request rate limit per IP.

| Property | Value |
|----------|-------|
| Variable | `RATE_LIMIT` |
| Default | `100` |
| Example | `50` |

### RATE_LIMIT_BURST

Rate limit burst allowance.

| Property | Value |
|----------|-------|
| Variable | `RATE_LIMIT_BURST` |
| Default | `10` |
| Example | `5` |

## Docker Configuration

### Using Environment Variables

```bash
docker run -d \
  -p 8080:8080 \
  -e PORT=8080 \
  -e UPLOAD_DIR=/data/uploads \
  -e MAX_FILE_SIZE=1GB \
  -e FILE_EXPIRY=48h \
  -v /host/uploads:/data/uploads \
  ghcr.io/jonasbg/paste:latest
```

### Using Docker Compose

```yaml
version: '3.8'
services:
  paste:
    image: ghcr.io/jonasbg/paste:latest
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - UPLOAD_DIR=/data/uploads
      - MAX_FILE_SIZE=5GB
      - CHUNK_SIZE=1
      - KEY_SIZE=256
      - ID_SIZE=128
      - FILE_EXPIRY=24h
      - RATE_LIMIT=100
    volumes:
      - paste-uploads:/data/uploads

volumes:
  paste-uploads:
```

## Kubernetes Configuration

### Using ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: paste-config
data:
  PORT: "8080"
  UPLOAD_DIR: "/data/uploads"
  MAX_FILE_SIZE: "5GB"
  FILE_EXPIRY: "24h"
```

### Using in Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: paste
spec:
  template:
    spec:
      containers:
        - name: paste
          image: ghcr.io/jonasbg/paste:latest
          envFrom:
            - configMapRef:
                name: paste-config
```

## Security Considerations

### Sensitive Variables

None of the environment variables contain secrets. Encryption keys are generated client-side and never configured on the server.

### Production Recommendations

| Variable | Recommendation |
|----------|----------------|
| `UPLOAD_DIR` | Dedicated volume, not in container |
| `MAX_FILE_SIZE` | Set based on available storage |
| `FILE_EXPIRY` | Balance security vs. convenience |
| `RATE_LIMIT` | Protect against abuse |

## Validation

The server validates environment variables at startup and uses defaults for invalid values. Check server logs for configuration warnings.

```bash
# View current configuration
docker logs paste-container | grep -i config
```
