---
title: Docker Deployment
description: Deploy Paste using Docker
sidebar_position: 1
---

# Docker Deployment

Deploy your own Paste instance using Docker.

## Quick Start

```bash
docker run -d \
  --name paste \
  -p 8080:8080 \
  -v paste-uploads:/app/uploads \
  ghcr.io/jonasbg/paste:latest
```

Access at `http://localhost:8080`

## Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  paste:
    image: ghcr.io/jonasbg/paste:latest
    container_name: paste
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - UPLOAD_DIR=/app/uploads
      - MAX_FILE_SIZE=5GB
      - FILE_EXPIRY=24h
      - RATE_LIMIT=100
    volumes:
      - paste-uploads:/app/uploads
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/api/config"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  paste-uploads:
```

Start:

```bash
docker-compose up -d
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `UPLOAD_DIR` | `./uploads` | Upload storage directory |
| `MAX_FILE_SIZE` | `100MB` | Max file size (supports B, KB, MB, GB, TB, KiB, MiB, GiB, TiB, or raw bytes) |
| `CHUNK_SIZE` | `1` | Chunk size in MB |
| `FILE_EXPIRY` | `24h` | Expiry for undownloaded files |
| `RATE_LIMIT` | `100` | Requests per minute per IP |

See [Environment Variables](../reference/environment.md) for full list.

### Persistent Storage

Always mount the uploads directory to persist files:

```yaml
volumes:
  - /host/path/uploads:/app/uploads
  # Or use named volume
  - paste-uploads:/app/uploads
```

### Resource Limits

```yaml
services:
  paste:
    # ...
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '0.5'
          memory: 512M
```

## Reverse Proxy

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name paste.example.com;

    ssl_certificate /etc/ssl/certs/paste.crt;
    ssl_certificate_key /etc/ssl/private/paste.key;

    # WebSocket support
    location /api/ws {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400;
    }

    # API and static files
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Large file uploads
        client_max_body_size 5G;
        proxy_request_buffering off;
    }
}
```

### Traefik

```yaml
services:
  paste:
    image: ghcr.io/jonasbg/paste:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.paste.rule=Host(`paste.example.com`)"
      - "traefik.http.routers.paste.tls=true"
      - "traefik.http.routers.paste.tls.certresolver=letsencrypt"
      - "traefik.http.services.paste.loadbalancer.server.port=8080"
```

### Caddy

```
paste.example.com {
    reverse_proxy localhost:8080
}
```

## Production Checklist

### Security

- [ ] Use HTTPS (TLS termination at reverse proxy)
- [ ] Set appropriate `MAX_FILE_SIZE`
- [ ] Configure rate limiting
- [ ] Use non-root user in container
- [ ] Keep image updated

### Storage

- [ ] Use persistent volume
- [ ] Monitor disk usage
- [ ] Set up backup (encrypted blobs only)
- [ ] Configure `FILE_EXPIRY` appropriately

### Monitoring

- [ ] Health check endpoint: `/api/config`
- [ ] Monitor container logs
- [ ] Set up alerts for disk space

### Performance

- [ ] Place uploads on fast storage (SSD)
- [ ] Configure appropriate chunk size
- [ ] Consider geographic distribution

## Building from Source

```bash
git clone https://github.com/jonasbg/paste
cd paste

# Build image
docker build -t paste:local .

# Run
docker run -d -p 8080:8080 paste:local
```

## Updating

```bash
# Pull latest image
docker-compose pull

# Recreate container
docker-compose up -d
```

## Troubleshooting

### Container won't start

Check logs:
```bash
docker logs paste
```

Common issues:
- Port already in use: Change `PORT` or host port mapping
- Permission denied on uploads: Check volume permissions

### Upload failures

- Check `MAX_FILE_SIZE` setting
- Verify disk space
- Check reverse proxy timeout settings

### WebSocket errors

Ensure reverse proxy supports WebSocket:
- Nginx: Add upgrade headers
- Traefik: Works by default
- Caddy: Works by default

## See Also

- [Kubernetes Deployment](kubernetes.md)
- [Environment Variables](../reference/environment.md)
- [Security Architecture](../security/architecture.md)
