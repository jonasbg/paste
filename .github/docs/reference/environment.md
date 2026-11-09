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

### UPLOAD_DIR

Directory for storing uploaded files.

| Property | Value |
|----------|-------|
| Variable | `UPLOAD_DIR` |
| Default | `./uploads` |
| Example | `/var/lib/paste/uploads` |

### MAX_FILE_SIZE

Maximum allowed file size. Supports raw bytes (number only) or a number followed by a unit: KB, MB, GB, TB, the IEC forms KiB/MiB/GiB/TiB, or Ki/Mi/Gi/Ti. All units are binary (1 KB = 1024 bytes).

| Property | Value |
|----------|-------|
| Variable | `MAX_FILE_SIZE` |
| Default | `100MB` |
| Examples | `1073741824` (raw bytes)<br/>`1GB`<br/>`5GiB`<br/>`500Mi` |

### CHUNK_SIZE

Upload chunk size in megabytes, between 1 and 64.

| Property | Value |
|----------|-------|
| Variable | `CHUNK_SIZE` |
| Default | `4` |
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

### FILES_RETENTION_DAYS

Days to keep an uploaded file before it is deleted, whether or not it was downloaded. Expired files are swept at startup and then hourly.

| Property | Value |
|----------|-------|
| Variable | `FILES_RETENTION_DAYS` |
| Default | `7` |
| Example | `1` |

### MAX_UPLOADS_PER_IP

Maximum number of concurrent uploads from one client IP; `0` disables the limit. Only uploads in progress count, and an upload over the limit is rejected rather than queued. Users behind one NAT or corporate proxy share this limit, and if `TRUSTED_PROXIES` does not include your reverse proxy, *all* users share it (see below).

| Property | Value |
|----------|-------|
| Variable | `MAX_UPLOADS_PER_IP` |
| Default | `5` |
| Example | `10`, `0` (unlimited) |

### MAX_STORAGE_SIZE

Optional cap on the total size of stored files, using the same format as `MAX_FILE_SIZE`. New uploads are rejected while the cap is reached. Unset means unlimited.

| Property | Value |
|----------|-------|
| Variable | `MAX_STORAGE_SIZE` |
| Default | (unlimited) |
| Example | `50GB` |

### TRUSTED_PROXIES

Comma-separated IPs or CIDR ranges of reverse proxies. `X-Forwarded-For` / `X-Real-IP` are only honoured when the direct peer is in this list; this is what the per-IP rate limit and upload limit key on.

If your ingress or load balancer is not in this list, every user appears as the proxy's IP and shares a single rate limit and upload limit. The server logs a warning when a private-network peer sends forwarding headers without being trusted:

```
WARNING: request from 172.18.0.5 carries X-Forwarded-For but 172.18.0.5 is not in TRUSTED_PROXIES ...
```

| Property | Value |
|----------|-------|
| Variable | `TRUSTED_PROXIES` |
| Default | `10.0.0.0/8` |
| Example | `10.0.0.0/8,192.168.1.10` |

### ALLOWED_ORIGINS

Comma-separated extra browser origins allowed to open WebSocket connections. Browser requests are otherwise only accepted when `Origin` matches the request `Host`. Only needed when a proxy rewrites `Host`.

| Property | Value |
|----------|-------|
| Variable | `ALLOWED_ORIGINS` |
| Default | (none) |
| Example | `https://paste.example.com` |

The server listens on port `8080` and rate-limits API requests to 60/s per IP (burst 120); these are not configurable.

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
| `FILES_RETENTION_DAYS` | Balance security vs. convenience |
| `MAX_STORAGE_SIZE` | Set below the volume size to avoid filling the disk |
| `TRUSTED_PROXIES` | Only your ingress/proxy addresses |

## Validation

The server validates environment variables at startup and refuses to start if `MAX_FILE_SIZE`, `MAX_STORAGE_SIZE`, `ID_SIZE`, `KEY_SIZE`, `CHUNK_SIZE`, `MAX_UPLOADS_PER_IP` or `TRUSTED_PROXIES` is invalid. `PASSPHRASE_WORDS` is clamped to 4-8 with a warning.

```bash
# View current configuration
docker logs paste-container | grep -i config
```
