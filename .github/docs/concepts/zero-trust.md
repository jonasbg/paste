---
title: Zero-Trust Architecture
description: How Paste ensures the server can never access your data
sidebar_position: 1
---

# Zero-Trust Architecture

Paste is built on a **zero-trust** principle: the server is never trusted with access to your data. Even if the server is compromised, your files remain encrypted and unreadable.

## What Zero-Trust Means

In a zero-trust architecture:

- **The server has no keys**: Encryption keys are generated and used only on client devices
- **The server stores only ciphertext**: Encrypted blobs that are meaningless without the key
- **The server cannot identify files**: Even metadata (filename, type, size) is encrypted

This is fundamentally different from services that encrypt "at rest" but have access to keys.

## How Paste Achieves Zero-Trust

### Client-Side Key Generation

Keys are generated on your device, never on the server:

```
Your Device                           Server
    │                                   │
    ├─── Generate passphrase ───────────│
    │    "happy-ocean-forest-moon-x7k3" │
    │                                   │
    ├─── Derive encryption key ─────────│
    │    (HKDF-SHA256)                  │
    │                                   │
    ├─── Derive file ID ────────────────│
    │    (HKDF-SHA256)                  │
    │                                   │
    ├─── Encrypt file ──────────────────│
    │    (AES-256-GCM)                  │
    │                                   │
    └─── Send encrypted blob ──────────→ Store blob
                                        (cannot decrypt)
```

### What the Server Sees

| Data | Server's View |
|------|---------------|
| File contents | Encrypted blob (random bytes) |
| Filename | Encrypted within blob |
| File type | Encrypted within blob |
| File size | Encrypted size (slightly larger) |
| Encryption key | Never received |
| Passphrase | Never received |

### What the Server Stores

```
/uploads/
  a7f3b2c1d4e5f6a8.Xk9fB2mPqRsT...
  │                 │
  │                 └─ HMAC token (proves key possession)
  └─ File ID (derived from passphrase)
```

The HMAC token proves the downloader has the encryption key without revealing it.

## Server Compromise Scenario

If an attacker gains full access to the server:

| Attacker Can | Attacker Cannot |
|--------------|-----------------|
| Read encrypted blobs | Decrypt any file |
| See file IDs | Know which passphrases are in use |
| See HMAC tokens | Derive encryption keys |
| Delete files | Read file contents or metadata |

The attacker would need to:
1. Identify a target file ID
2. Brute-force all possible passphrases (~57-95 bits)
3. For each guess, derive the key and attempt decryption

With 4 words + suffix, this would take thousands of years.

## Comparison with Other Approaches

### "Encrypted at Rest"

Many services claim encryption but hold the keys:

```
Your Device                         Server
    │                                  │
    └─── Send plaintext ──────────────→ Encrypt with server key
                                        Store encrypted
                                        (server can decrypt anytime)
```

**Risk**: Server compromise = total data breach.

### End-to-End with Server Key Exchange

Some services do E2E but exchange keys through the server:

```
Your Device                         Server
    │                                  │
    ├─── Generate key ─────────────────│
    └─── Send key to server ──────────→ Store key
                                        (server has access)
```

**Risk**: Server can intercept keys during exchange.

### Paste's Approach

```
Your Device                         Server
    │                                  │
    ├─── Generate key locally          │
    └─── Send only ciphertext ────────→ Store ciphertext
                                        (no key ever received)
```

**Risk**: Only client-side compromise or passphrase interception.

## Trust Boundaries

### You Must Trust

- **Your device**: Where encryption/decryption happens
- **The code**: CLI tool, web app, or WASM module
- **Your passphrase channel**: How you share the passphrase

### You Don't Need to Trust

- **The server**: Handles only encrypted data
- **The network**: TLS + E2E encryption
- **Server operators**: Cannot access your data
- **Law enforcement requests**: Server has nothing useful to provide

## Verifying Zero-Trust

The entire codebase is open source for verification:

- **Crypto library**: `/crypto/` - all cryptographic operations
- **WASM module**: `/wasm/` - browser-side encryption
- **CLI tool**: `/pastectl/` - command-line encryption
- **Server**: `/api/` - verify server never receives keys

See the [Audit Guide](../security/audit-guide.md) for detailed review guidance.

## Limitations

Zero-trust protects against server compromise but not:

- **Compromised clients**: Malware on your device
- **Weak passphrases**: Though minimums are enforced
- **Passphrase interception**: If attacker sees your passphrase
- **Targeted attacks**: Nation-state actors with unlimited resources

For the highest security needs, use [URL Mode](url-mode.md) with 128-bit random keys.

## Further Reading

- [Passphrase Mode](passphrase-mode.md) - How passphrases derive keys
- [Encryption Details](encryption.md) - Cryptographic implementation
- [Threat Model](../security/threat-model.md) - Complete threat analysis
