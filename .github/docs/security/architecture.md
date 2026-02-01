---
title: Security Architecture
description: Complete overview of Paste's security design
sidebar_position: 1
---

# Security Architecture

This document provides a comprehensive overview of Paste's security architecture for security professionals and auditors.

## Design Principles

### 1. Zero-Trust Server

The server is designed to be untrusted:
- Never receives encryption keys
- Stores only encrypted blobs
- Cannot decrypt any content
- Cannot read metadata (filename, type)

### 2. Client-Side Encryption

All cryptographic operations happen on the client:
- Key generation/derivation
- Encryption/decryption
- HMAC token generation

### 3. Minimal Data Retention

- Files deleted after first download
- No user accounts or tracking
- No logs of file contents

### 4. Defense in Depth

Multiple layers of protection:
- End-to-end encryption
- TLS in transit
- HMAC authentication
- Server-side validation

## System Components

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   CLI Client    │     │   Web Client    │     │   API Server    │
│                 │     │                 │     │                 │
│ • Key derivation│     │ • WASM crypto   │     │ • Blob storage  │
│ • AES-GCM       │     │ • AES-GCM       │     │ • HMAC verify   │
│ • HMAC tokens   │     │ • HMAC tokens   │     │ • Rate limiting │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         └───────────────────────┴───────────────────────┘
                                 │
                          TLS (HTTPS/WSS)
```

## Data Flow

### Upload Flow

```
1. User provides file
         │
         ▼
2. Generate/derive key (client-side)
   ├── Passphrase mode: HKDF from passphrase
   └── URL mode: Random 128/256 bits
         │
         ▼
3. Derive file ID (client-side)
   ├── Passphrase mode: HKDF from passphrase
   └── URL mode: Server-generated random ID
         │
         ▼
4. Encrypt file (client-side, AES-256-GCM)
   ├── Encrypt metadata (filename, type, size)
   └── Encrypt content in chunks
         │
         ▼
5. Generate HMAC token (client-side)
   └── HMAC-SHA256(derived_key, file_id)
         │
         ▼
6. Upload via WebSocket (TLS)
   ├── Send file ID
   ├── Send HMAC token
   └── Stream encrypted chunks
         │
         ▼
7. Server stores: {file_id}.{hmac_token}
   └── Server cannot decrypt
```

### Download Flow

```
1. User provides passphrase or URL
         │
         ▼
2. Derive/extract key (client-side)
         │
         ▼
3. Derive/extract file ID (client-side)
         │
         ▼
4. Generate HMAC token (client-side)
         │
         ▼
5. Request download with file ID + HMAC token
         │
         ▼
6. Server verifies HMAC token matches stored token
   ├── Match: Return encrypted blob
   └── No match: Reject (403)
         │
         ▼
7. Decrypt file (client-side)
         │
         ▼
8. Server deletes file
```

## Cryptographic Components

| Component | Algorithm | Purpose |
|-----------|-----------|---------|
| Encryption | AES-256-GCM | File confidentiality + integrity |
| Key Derivation | HKDF-SHA256 | Derive key from passphrase |
| Authentication | HMAC-SHA256 | Prove key possession |
| Random Generation | OS CSPRNG | Keys, IVs, suffixes |

## Server Security

### What Server Stores

```
/uploads/
  a7f3b2c1d4e5f6a8.Xk9fB2mPqRsT...  ← {file_id}.{hmac_token}
```

Contents of stored file:
```
[Encrypted Metadata Header]  ← Cannot read filename/type
[Content IV]
[Encrypted Chunk 1]          ← Cannot decrypt
[Encrypted Chunk 2]
...
```

### What Server Validates

1. **File size**: Within configured limits
2. **File ID format**: Valid hex string, correct length
3. **HMAC token format**: Valid base64, correct length
4. **HMAC token match**: Token must match stored value
5. **Collision check**: Reject if file ID exists (passphrase mode)

### What Server Cannot Do

- Decrypt file contents
- Read filenames or file types
- Determine which passphrases are in use
- Correlate files to users
- Recover deleted files

## Authentication Model

### HMAC Token System

```
Encryption Key
       │
       ▼
┌──────────────┐
│ HKDF-SHA256  │ ← Context: "paste:hmac-token"
│ Salt: fileID │
└──────┬───────┘
       │
       ▼
   HMAC Key
       │
       ▼
┌──────────────┐
│ HMAC-SHA256  │ ← Message: fileID
└──────┬───────┘
       │
       ▼
   HMAC Token
```

### Why This Design?

- **Proves key possession**: Only someone with the key can generate valid token
- **Doesn't reveal key**: HMAC is one-way
- **Bound to file**: Token only works for specific file ID
- **Single secret**: User only shares passphrase or URL, not separate token

## Threat Analysis

### Server Compromise

**Scenario**: Attacker gains full server access

| Attacker Capability | Attacker Cannot |
|---------------------|-----------------|
| Read encrypted blobs | Decrypt anything |
| See file IDs | Know passphrases |
| Delete files | Modify without detection |
| See HMAC tokens | Derive encryption keys |

**Mitigation**: Encryption keys never touch server

### Network Attack

**Scenario**: Attacker intercepts network traffic

| Attacker Capability | Attacker Cannot |
|---------------------|-----------------|
| See TLS handshake | Break TLS (without cert) |
| See encrypted payloads | Decrypt E2E encryption |
| Perform replay | Token changes per request |

**Mitigation**: TLS + E2E encryption

### Brute-Force Attack

**Scenario**: Attacker tries all possible passphrases

| Mode | Entropy | Time at 10⁹/sec |
|------|---------|-----------------|
| 4 words + suffix | ~57 bits | ~2,284 years |
| 8 words + suffix | ~95 bits | ~∞ |
| URL mode | 128 bits | ~∞ |

**Mitigation**: High entropy + rate limiting

## Implementation Security

### Memory Safety

- Go: Memory-safe language
- No buffer overflows
- Automatic bounds checking

### Cryptographic Implementation

- Uses Go standard library `crypto/*`
- No custom cryptographic primitives
- Constant-time comparisons where needed

### Input Validation

- All user input validated
- File ID: hex characters only
- Token: base64url characters only
- Size limits enforced

## Audit Checklist

For security auditors reviewing the codebase:

### Critical Files

| File | Purpose | What to Check |
|------|---------|---------------|
| `crypto/crypto.go` | Core crypto | Key derivation, encryption |
| `crypto/wordlist.go` | Passphrase gen | Randomness, entropy |
| `wasm/wasm.go` | Browser crypto | Same as crypto.go |
| `api/handlers/websocket.go` | Upload handler | Validation, auth |
| `api/handlers/files.go` | Download handler | Auth, access control |

## Compliance Considerations

### GDPR

- No personal data stored (no accounts)
- Files encrypted (pseudonymization)
- Automatic deletion (data minimization)

### HIPAA

- Encryption at rest: AES-256-GCM
- Encryption in transit: TLS 1.3
- Access controls: HMAC tokens
- Audit: Server logs (no PHI visible)

*Note: Self-hosters must ensure overall deployment compliance*

## Further Reading

- [Cryptography](cryptography.md) - Detailed algorithm analysis
- [Threat Model](threat-model.md) - Complete threat analysis
- [Audit Guide](audit-guide.md) - For security researchers
