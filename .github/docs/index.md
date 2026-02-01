---
title: Paste Documentation
description: Zero-trust encrypted file sharing with memorable passphrases
slug: /
sidebar_position: 1
---

# Paste

**Zero-trust encrypted file sharing with memorable passphrases.**

Paste lets you share files securely without accounts, tracking, or trusting the server. Files are encrypted on your device before upload - the server only ever sees encrypted data it cannot read.

## Why Paste?

- **Zero-trust**: Server cannot decrypt your files - ever
- **Simple sharing**: Use memorable passphrases like `happy-ocean-forest-moon-x7k3`
- **No accounts**: Upload and share immediately
- **Self-hostable**: Run your own instance
- **Open source**: Fully auditable code

## Quick Example

```bash
# Upload a file
echo "secret message" | pastectl
# Output: Share code: happy-ocean-forest-moon-x7k3

# Download it
pastectl download happy-ocean-forest-moon-x7k3
```

That's it. Share the passphrase through any channel - the recipient can decrypt using only that passphrase.

## Documentation

### Getting Started

- [Installation](getting-started/installation.md) - Install the CLI or access the web interface
- [Quick Start](getting-started/quick-start.md) - Upload your first file in 60 seconds
- [CLI Reference](reference/cli.md) - Complete command reference

### Concepts

- [Zero-Trust Architecture](concepts/zero-trust.md) - How Paste protects your data
- [Passphrase Mode](concepts/passphrase-mode.md) - Sharing with memorable codes
- [URL Mode](concepts/url-mode.md) - Maximum security with link sharing
- [Encryption Details](concepts/encryption.md) - How files are encrypted

### Security

- [Security Architecture](security/architecture.md) - Technical security overview
- [Cryptography](security/cryptography.md) - Algorithms and implementation
- [Threat Model](security/threat-model.md) - What Paste protects against
- [Audit Guide](security/audit-guide.md) - For security researchers

### Self-Hosting

- [Docker Deployment](self-hosting/docker.md) - Run with Docker
- [Kubernetes](self-hosting/kubernetes.md) - Deploy with Helm

### Reference

- [CLI Reference](reference/cli.md) - All commands and flags
- [Environment Variables](reference/environment.md) - Configuration options
- [FAQ](reference/faq.md) - Frequently asked questions

## Support

- [GitHub Issues](https://github.com/jonasbg/paste/issues) - Bug reports and feature requests
- [Source Code](https://github.com/jonasbg/paste) - Full source code
