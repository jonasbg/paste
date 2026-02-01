---
title: Kubernetes Deployment
description: Deploy Paste on Kubernetes using Helm
sidebar_position: 2
---

# Kubernetes Deployment

Deploy Paste on Kubernetes using the official Helm chart.

## Prerequisites

- Kubernetes cluster (1.19+)
- Helm 3.x
- kubectl configured

## Quick Start

```bash
# Add Helm repository
helm repo add paste https://jonasbg.github.io/paste/charts
helm repo update

# Install
helm install paste paste/paste
```

## Helm Chart

### Installation

```bash
# Basic installation
helm install paste paste/paste \
  --namespace paste \
  --create-namespace

# With custom values
helm install paste paste/paste \
  --namespace paste \
  --create-namespace \
  -f values.yaml
```

### values.yaml

```yaml
replicaCount: 1

image:
  repository: ghcr.io/jonasbg/paste
  tag: latest
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080

ingress:
  enabled: true
  className: nginx
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "5g"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
  hosts:
    - host: paste.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: paste-tls
      hosts:
        - paste.example.com

env:
  PORT: "8080"
  UPLOAD_DIR: "/data/uploads"
  MAX_FILE_SIZE: "5GB"
  CHUNK_SIZE: "1"
  FILE_EXPIRY: "24h"
  RATE_LIMIT: "100"

persistence:
  enabled: true
  storageClass: ""
  accessMode: ReadWriteOnce
  size: 100Gi

resources:
  limits:
    cpu: 2000m
    memory: 2Gi
  requests:
    cpu: 100m
    memory: 256Mi

nodeSelector: {}

tolerations: []

affinity: {}
```

## Manual Deployment

If not using Helm, apply these manifests:

### Namespace

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: paste
```

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: paste-config
  namespace: paste
data:
  PORT: "8080"
  UPLOAD_DIR: "/data/uploads"
  MAX_FILE_SIZE: "5GB"
  CHUNK_SIZE: "1"
  FILE_EXPIRY: "24h"
  RATE_LIMIT: "100"
```

### PersistentVolumeClaim

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: paste-uploads
  namespace: paste
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 100Gi
```

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: paste
  namespace: paste
spec:
  replicas: 1
  selector:
    matchLabels:
      app: paste
  template:
    metadata:
      labels:
        app: paste
    spec:
      containers:
        - name: paste
          image: ghcr.io/jonasbg/paste:latest
          ports:
            - containerPort: 8080
          envFrom:
            - configMapRef:
                name: paste-config
          volumeMounts:
            - name: uploads
              mountPath: /data/uploads
          resources:
            limits:
              cpu: 2000m
              memory: 2Gi
            requests:
              cpu: 100m
              memory: 256Mi
          livenessProbe:
            httpGet:
              path: /api/config
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /api/config
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
      volumes:
        - name: uploads
          persistentVolumeClaim:
            claimName: paste-uploads
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: paste
  namespace: paste
spec:
  selector:
    app: paste
  ports:
    - port: 8080
      targetPort: 8080
  type: ClusterIP
```

### Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: paste
  namespace: paste
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "5g"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - paste.example.com
      secretName: paste-tls
  rules:
    - host: paste.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: paste
                port:
                  number: 8080
```

## Configuration

### Storage Classes

For production, use a fast storage class:

```yaml
persistence:
  storageClass: fast-ssd
```

### Resource Limits

Adjust based on expected load:

```yaml
resources:
  limits:
    cpu: 4000m
    memory: 4Gi
  requests:
    cpu: 500m
    memory: 512Mi
```

### Horizontal Pod Autoscaling

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: paste
  namespace: paste
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: paste
  minReplicas: 1
  maxReplicas: 5
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
```

**Note**: Scaling requires shared storage (ReadWriteMany) for uploads.

## Production Considerations

### High Availability

For HA, you need:
1. ReadWriteMany storage (NFS, EFS, etc.)
2. Multiple replicas
3. Load balancer with session affinity (optional)

```yaml
replicaCount: 3

persistence:
  accessMode: ReadWriteMany
  storageClass: efs-sc  # AWS EFS example
```

### TLS Certificates

Using cert-manager:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: paste-tls
  namespace: paste
spec:
  secretName: paste-tls
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
    - paste.example.com
```

### Network Policies

Restrict traffic:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: paste-policy
  namespace: paste
spec:
  podSelector:
    matchLabels:
      app: paste
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - port: 8080
```

### Monitoring

Prometheus ServiceMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: paste
  namespace: paste
spec:
  selector:
    matchLabels:
      app: paste
  endpoints:
    - port: http
      path: /metrics
```

## Upgrading

```bash
# Update Helm repo
helm repo update

# Upgrade release
helm upgrade paste paste/paste \
  --namespace paste \
  -f values.yaml
```

## Troubleshooting

### Pod not starting

```bash
kubectl describe pod -n paste -l app=paste
kubectl logs -n paste -l app=paste
```

### Storage issues

```bash
kubectl get pvc -n paste
kubectl describe pvc paste-uploads -n paste
```

### Ingress not working

```bash
kubectl get ingress -n paste
kubectl describe ingress paste -n paste
```

## See Also

- [Docker Deployment](docker.md)
- [Environment Variables](../reference/environment.md)
- [Security Architecture](../security/architecture.md)
