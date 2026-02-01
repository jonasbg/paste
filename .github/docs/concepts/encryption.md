---
title: Encryption Details
description: How files are encrypted and decrypted
sidebar_position: 4
---

# Encryption Details

This document explains how Paste encrypts and decrypts files at a technical level.

## Overview

Paste uses **AES-256-GCM** (Galois/Counter Mode) for all file encryption. This provides:

- **Confidentiality**: File contents are unreadable without the key
- **Authenticity**: Any tampering is detected
- **Integrity**: Corrupted data is rejected

## Encryption Algorithm

### AES-256-GCM

| Property | Value |
|----------|-------|
| Algorithm | AES (Advanced Encryption Standard) |
| Key Size | 256 bits (32 bytes) |
| Mode | GCM (Galois/Counter Mode) |
| IV/Nonce Size | 96 bits (12 bytes) |
| Tag Size | 128 bits (16 bytes) |

### Why AES-GCM?

- **Industry standard**: Used by TLS, SSH, and most secure protocols
- **Hardware acceleration**: AES-NI instructions on modern CPUs
- **AEAD**: Authenticated Encryption with Associated Data
- **Streaming**: Can encrypt/decrypt in chunks

## File Format

Encrypted files have this structure:

```
┌──────────────────────────────────────────┐
│ Header                                   │
│ ├── Metadata IV (12 bytes)               │
│ ├── Metadata Length (4 bytes)            │
│ └── Encrypted Metadata (variable)        │
├──────────────────────────────────────────┤
│ Content IV (12 bytes)                    │
├──────────────────────────────────────────┤
│ Encrypted Chunk 1 + GCM Tag (16 bytes)   │
├──────────────────────────────────────────┤
│ Encrypted Chunk 2 + GCM Tag (16 bytes)   │
├──────────────────────────────────────────┤
│ ...                                      │
├──────────────────────────────────────────┤
│ Encrypted Chunk N + GCM Tag (16 bytes)   │
└──────────────────────────────────────────┘
```

### Metadata

The encrypted metadata contains:
- Original filename
- Content type (MIME type)
- Original file size

This information is encrypted with the same key, so the server cannot read it.

### Chunked Encryption

Large files are split into chunks for:
- **Memory efficiency**: Don't load entire file into memory
- **Streaming**: Start uploading before encryption completes
- **Progress tracking**: Report upload/download progress

Default chunk size: 1 MB (configurable by server)

## Nonce Handling

### Why Nonces Matter

GCM mode requires a unique nonce (number used once) for each encryption operation. Reusing a nonce with the same key is catastrophic - it can reveal plaintext.

### Nonce Generation

For each file, Paste generates a random base IV:

```go
iv := make([]byte, 12)
rand.Read(iv) // Cryptographically random
```

### Per-Chunk Nonces

Each chunk uses a unique nonce derived from the base IV:

```go
chunkNonce := baseIV
chunkNonce[8:12] = littleEndian(chunkNumber)
```

This ensures:
- Each chunk has a unique nonce
- Nonces are deterministic for decryption
- No nonce reuse even with same key

### Maximum Chunks

To prevent nonce overflow, there's a limit of 2²⁰ (~1 million) chunks per file. With 1 MB chunks, this allows files up to ~1 TB.

## Key Derivation

### Passphrase Mode

Keys are derived using HKDF (HMAC-based Key Derivation Function):

```go
// Derive 256-bit encryption key
keyReader := hkdf.New(sha256.New,
    []byte(passphrase),           // Input key material
    nil,                          // No salt (intentional)
    []byte("paste-v1-encryption-key")) // Context

key := make([]byte, 32)
keyReader.Read(key)
```

### URL Mode

Keys are generated directly from the OS CSPRNG:

```go
key := make([]byte, 32)
rand.Read(key) // crypto/rand
```

## Authentication

### HMAC Tokens

To download a file, clients must prove they have the key without revealing it:

```go
// Derive HMAC key from encryption key
hmacKey := hkdf.New(sha256.New,
    encryptionKey,
    []byte(fileID),
    []byte("paste:hmac-token"))

// Generate token
token := HMAC-SHA256(hmacKey, fileID)
```

The server stores `fileID.token` and verifies the token on download requests.

### Why HMAC?

- Proves key possession without revealing key
- Derived from encryption key (one secret to share)
- Computationally infeasible to forge

## Implementation

### Go (Server & CLI)

```go
import (
    "crypto/aes"
    "crypto/cipher"
)

// Create cipher
block, _ := aes.NewCipher(key)
aead, _ := cipher.NewGCM(block)

// Encrypt
ciphertext := aead.Seal(nil, nonce, plaintext, nil)

// Decrypt
plaintext, _ := aead.Open(nil, nonce, ciphertext, nil)
```

### WebAssembly (Browser)

The same Go code is compiled to WebAssembly for browser use:

```bash
GOOS=js GOARCH=wasm tinygo build -o encryption.wasm
```

This ensures identical encryption behavior across CLI and web.

## Security Properties

### What's Protected

| Threat | Protection |
|--------|------------|
| Eavesdropping | AES-256 encryption |
| Tampering | GCM authentication tag |
| Replay | One-time download + deletion |
| Key extraction | Key never sent to server |

### Assumptions

| Assumption | Consequence if Violated |
|------------|------------------------|
| AES-256 is secure | Encryption broken (unlikely) |
| Random IV is unique | Nonce reuse (catastrophic) |
| Key remains secret | Full compromise |
| GCM tag verified | Tampering undetected |

## Decryption Process

1. **Download encrypted blob** from server
2. **Parse header**: Extract metadata IV and length
3. **Decrypt metadata**: Get filename, type, size
4. **Read content IV**: 12-byte nonce for content
5. **For each chunk**:
   - Derive chunk nonce from base IV
   - Decrypt chunk with GCM
   - Verify authentication tag
   - Output plaintext
6. **Verify completion**: All chunks decrypted

If any tag verification fails, the entire decryption is aborted.

## Performance

### Encryption Speed

On modern hardware with AES-NI:
- ~1-4 GB/s for AES-256-GCM
- Bottleneck is usually network, not crypto

### Memory Usage

- Streaming encryption: ~2× chunk size
- No need to load entire file

## Further Reading

- [Cryptography](../security/cryptography.md) - Algorithm details
- [Security Architecture](../security/architecture.md) - Overall security design
- [NIST SP 800-38D](https://csrc.nist.gov/publications/detail/sp/800-38d/final) - GCM specification
