---
title: Cryptography
description: Detailed analysis of cryptographic algorithms and implementation
sidebar_position: 2
---

# Cryptography

This document provides detailed information about the cryptographic algorithms and their implementation in Paste.

## Algorithm Summary

| Function | Algorithm | Standard |
|----------|-----------|----------|
| Symmetric Encryption | AES-256-GCM | NIST FIPS 197, SP 800-38D |
| Key Derivation | HKDF-SHA256 | RFC 5869 |
| Message Authentication | HMAC-SHA256 | RFC 2104 |
| Random Generation | OS CSPRNG | Platform-specific |

## AES-256-GCM

### Overview

AES-GCM (Galois/Counter Mode) is an authenticated encryption algorithm that provides both confidentiality and integrity.

### Parameters

| Parameter | Value | Notes |
|-----------|-------|-------|
| Key size | 256 bits | Maximum AES key size |
| Block size | 128 bits | Fixed for AES |
| Nonce size | 96 bits | Recommended by NIST |
| Tag size | 128 bits | Maximum security |

### Security Properties

- **IND-CPA**: Indistinguishable under chosen-plaintext attack
- **INT-CTXT**: Integrity of ciphertext
- **Nonce-misuse resistance**: None (catastrophic if nonce reused)

### Implementation

```go
import (
    "crypto/aes"
    "crypto/cipher"
)

func encrypt(key, plaintext, nonce []byte) ([]byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }

    aead, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    // Seal appends authentication tag
    return aead.Seal(nil, nonce, plaintext, nil), nil
}
```

### Nonce Handling

Each chunk uses a derived nonce to prevent reuse:

```go
func deriveNonce(baseIV []byte, chunkNumber uint32) []byte {
    nonce := make([]byte, 12)
    copy(nonce, baseIV)
    binary.LittleEndian.PutUint32(nonce[8:], chunkNumber)
    return nonce
}
```

**Nonce structure:**
```
[  Base IV (8 bytes)  ][Chunk # (4 bytes)]
```

Maximum chunks: 2³² - 1 = ~4 billion (far exceeds practical needs)

## HKDF (Key Derivation)

### Overview

HKDF (HMAC-based Key Derivation Function) extracts and expands keying material from a source.

### Parameters

| Parameter | Value |
|-----------|-------|
| Hash function | SHA-256 |
| Input key material | Passphrase bytes |
| Salt | None (empty) |
| Info | Context-specific strings |

### Why No Salt?

Traditional password hashing uses random salt to:
1. Prevent rainbow tables
2. Make identical passwords produce different hashes

Paste intentionally omits salt because:
1. **Determinism required**: Same passphrase must produce same File ID
2. **No stored hashes**: Server doesn't verify passwords
3. **Entropy elsewhere**: Passphrase + suffix provides entropy

### Context Separation

Different contexts ensure independent derived keys:

```go
// For File ID
fileIDKey := hkdf.New(sha256.New, passphrase, nil,
    []byte("paste-v1-file-id"))

// For encryption key
encKey := hkdf.New(sha256.New, passphrase, nil,
    []byte("paste-v1-encryption-key"))
```

This ensures that even with the same passphrase:
- File ID reveals nothing about encryption key
- Encryption key reveals nothing about File ID

### Security Analysis

HKDF security relies on:
1. **PRF assumption**: HMAC-SHA256 is a pseudorandom function
2. **Sufficient input entropy**: Passphrase must have enough entropy
3. **Unique contexts**: Different info strings produce independent outputs

## HMAC-SHA256 (Authentication)

### Overview

HMAC provides message authentication and is used for download tokens.

### Token Generation

```go
func generateToken(encKey []byte, fileID string) []byte {
    // Derive HMAC key from encryption key
    hmacKeyReader := hkdf.New(sha256.New, encKey,
        []byte(fileID), []byte("paste:hmac-token"))

    hmacKey := make([]byte, 32)
    hmacKeyReader.Read(hmacKey)

    // Generate HMAC
    h := hmac.New(sha256.New, hmacKey)
    h.Write([]byte(fileID))
    return h.Sum(nil)
}
```

### Security Properties

- **Unforgeability**: Cannot create valid token without key
- **Key binding**: Token bound to specific file ID
- **Non-reversibility**: Cannot extract key from token

## Random Number Generation

### Source

All randomness comes from the operating system's CSPRNG:

```go
import "crypto/rand"

key := make([]byte, 32)
rand.Read(key)  // Uses OS entropy source
```

### Platform Implementation

| OS | Source |
|----|--------|
| Linux | `/dev/urandom` (getrandom syscall) |
| macOS | `SecRandomCopyBytes` |
| Windows | `CryptGenRandom` / BCrypt |

### What Uses Randomness

| Component | Bytes | Purpose |
|-----------|-------|---------|
| URL mode key | 16-32 | Encryption key |
| Content IV | 12 | Nonce for content |
| Metadata IV | 12 | Nonce for metadata |
| Passphrase words | ~2 per word | Word selection |
| Passphrase suffix | ~2 | Suffix characters |

## Entropy Analysis

### Passphrase Entropy

```
Wordlist size: ~600 words
Suffix charset: 36 characters (a-z, 0-9)
Suffix length: 4 characters

Per word: log₂(600) ≈ 9.23 bits
Per suffix char: log₂(36) ≈ 5.17 bits

4 words + suffix: 4 × 9.23 + 4 × 5.17 ≈ 57.6 bits
5 words + suffix: 5 × 9.23 + 4 × 5.17 ≈ 66.8 bits
6 words + suffix: 6 × 9.23 + 4 × 5.17 ≈ 76.0 bits
8 words + suffix: 8 × 9.23 + 4 × 5.17 ≈ 94.5 bits
```

### URL Mode Entropy

```
Key size: 128 bits (or 256 bits)
Source: OS CSPRNG

Entropy: 128 bits (full key size)
```

## Constant-Time Operations

### Where Required

Timing attacks are mitigated in security-critical comparisons:

```go
import "crypto/subtle"

// Token verification
if subtle.ConstantTimeCompare(provided, stored) != 1 {
    return ErrInvalidToken
}
```

### Not Required

- Key derivation (not timing-sensitive)
- Encryption/decryption (AES-NI is constant-time)
- File operations (I/O dominates timing)

## Algorithm Agility

### Version Prefix

Context strings include version for future algorithm changes:

```
"paste-v1-file-id"
"paste-v1-encryption-key"
```

If algorithms need updating, "v2" contexts can be introduced without breaking existing files.

### Migration Path

1. Add new algorithm with "v2" contexts
2. Client detects version from server config
3. Old files remain readable
4. New uploads use new algorithm

## Standards Compliance

| Standard | Compliance |
|----------|------------|
| NIST SP 800-38D (GCM) | Full |
| RFC 5869 (HKDF) | Full |
| RFC 2104 (HMAC) | Full |
| NIST SP 800-90A (RNG) | OS-dependent |

## Security Margins

| Primitive | Attack Complexity | Quantum Resistance |
|-----------|-------------------|-------------------|
| AES-256 | 2²⁵⁶ | 2¹²⁸ (Grover's) |
| SHA-256 | 2²⁵⁶ (preimage) | 2¹²⁸ (Grover's) |
| HMAC-SHA256 | 2²⁵⁶ | 2¹²⁸ (Grover's) |

All primitives have substantial security margins against both classical and quantum attacks.

## Further Reading

- [Security Architecture](architecture.md) - Overall security design
- [Threat Model](threat-model.md) - Attack analysis
- [Audit Guide](audit-guide.md) - Review guidance
- [NIST Recommendations](https://csrc.nist.gov/publications/detail/sp/800-38d/final) - GCM specification
