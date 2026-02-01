---
title: Audit Guide
description: Guide for security researchers reviewing Paste
sidebar_position: 4
---

# Audit Guide

This guide helps security researchers and auditors review Paste's implementation.

## Repository Structure

```
paste/
├── api/                    # Server implementation
│   ├── handlers/           # HTTP/WebSocket handlers
│   ├── middleware/         # Auth, rate limiting
│   └── db/                 # Database operations
├── crypto/                 # Cryptographic library
│   ├── crypto.go           # Core crypto functions
│   └── wordlist.go         # Passphrase generation
├── pastectl/               # CLI client
│   └── internal/
│       ├── upload/         # Upload logic
│       ├── download/       # Download logic
│       └── cli/            # CLI interface
├── wasm/                   # WebAssembly module
│   └── wasm.go             # Browser crypto
└── web/                    # Web frontend
    └── src/lib/
        └── services/       # Encryption service
```

## Critical Files to Review

### Tier 1: Cryptographic Core

| File | Purpose | Review Focus |
|------|---------|--------------|
| `crypto/crypto.go` | Core crypto | Key derivation, encryption, HMAC |
| `crypto/wordlist.go` | Passphrase gen | Randomness, entropy |
| `wasm/wasm.go` | Browser crypto | Same as crypto.go |

### Tier 2: Protocol Implementation

| File | Purpose | Review Focus |
|------|---------|--------------|
| `api/handlers/websocket.go` | Upload handling | Auth, validation, storage |
| `api/handlers/files.go` | Download handling | Auth, access control |
| `pastectl/internal/upload/upload.go` | CLI upload | Key handling |
| `pastectl/internal/download/download.go` | CLI download | Key handling |

### Tier 3: Supporting Code

| File | Purpose | Review Focus |
|------|---------|--------------|
| `api/middleware/` | Server middleware | Rate limiting, logging |
| `web/src/lib/services/encryptionService.ts` | Web encryption | WASM integration |

## Key Questions

### Cryptographic Implementation

1. **Key Generation**
   - Is crypto/rand used for all key material?
   - Are key sizes correct (16/24/32 bytes)?
   - Is key material zeroed after use?

2. **Key Derivation**
   - Is HKDF used correctly?
   - Are context strings unique and appropriate?
   - Is the passphrase used as IKM without modification?

3. **Encryption**
   - Is AES-GCM used with 12-byte nonces?
   - Are nonces never reused?
   - Is chunk counter properly incremented?

4. **HMAC Tokens**
   - Is HMAC key derived from encryption key?
   - Is file ID used as additional input?
   - Is comparison constant-time?

### Server Security

1. **Input Validation**
   - Are file IDs validated (hex only)?
   - Are tokens validated (base64url only)?
   - Are size limits enforced?

2. **Access Control**
   - Is HMAC token required for download?
   - Is token verified before file access?
   - Is collision check performed for custom IDs?

3. **Information Disclosure**
   - Do error messages reveal sensitive info?
   - Are keys/passphrases ever logged?
   - Is file content ever processed server-side?

### Client Security

1. **Key Handling**
   - Are keys generated/derived client-side only?
   - Are keys transmitted to server? (Should be NO)
   - Are keys properly cleared from memory?

2. **URL Mode**
   - Is key in URL fragment only?
   - Is fragment sent to server? (Should be NO)
   - Is key extraction correct?

## Code Review Checklist

### crypto/crypto.go

```go
// ✓ Check: HKDF context strings
fileIDInfo := []byte("paste-v1-file-id")
keyInfo := []byte("paste-v1-encryption-key")

// ✓ Check: Key size validation
if keySize != 16 && keySize != 24 && keySize != 32 {
    return error
}

// ✓ Check: Random IV generation
if _, err := rand.Read(iv); err != nil {
    return error
}

// ✓ Check: Nonce derivation for chunks
binary.LittleEndian.PutUint32(nonce[8:], uint32(chunkNum))
```

### crypto/wordlist.go

```go
// ✓ Check: Uses crypto/rand
n, err := rand.Int(rand.Reader, max)

// ✓ Check: Suffix has digit
hasDigit := false
for _, c := range result {
    if c >= '0' && c <= '9' {
        hasDigit = true
    }
}
```

### api/handlers/websocket.go

```go
// ✓ Check: Collision detection
matches, err := filepath.Glob(filepath.Join(uploadDir, init.FileID+".*"))
if len(matches) > 0 {
    sendError(ws, "Share code already in use")
    return
}

// ✓ Check: Token validation
if !validateToken(tokenData.Token) {
    sendError(ws, "Invalid token")
    return
}
```

### api/handlers/files.go

```go
// ✓ Check: ID validation
func validateID(id string) bool {
    safeChars := "0123456789abcdefABCDEF"
    for _, char := range id {
        if !strings.ContainsRune(safeChars, char) {
            return false
        }
    }
    return true
}

// ✓ Check: Token in filename
filePath := filepath.Join(uploadDir, id+"."+token)
```

## Testing Procedures

### 1. Key Derivation Test

Verify same passphrase produces same outputs:

```bash
# Should produce identical results
echo "test passphrase" | ./derive_test
echo "test passphrase" | ./derive_test
```

### 2. Encryption Roundtrip Test

```bash
echo "test data" > test.txt
pastectl upload -f test.txt
# Note passphrase
pastectl download <passphrase> -o test2.txt
diff test.txt test2.txt  # Should be empty
```

### 3. HMAC Token Test

Verify invalid token is rejected:

```bash
curl -H "X-HMAC-Token: invalid" \
  https://paste.torden.tech/api/download/<file_id>
# Should return 403
```

### 4. Collision Test

```bash
# Upload with same passphrase twice
# Second should fail with collision error
```

### 5. Nonce Uniqueness Test

Upload large file and verify each chunk has unique nonce:

```bash
# Capture encrypted chunks
# Verify first 12 bytes differ for each chunk
```

## Common Vulnerabilities to Check

### Cryptographic

- [ ] Nonce reuse in GCM
- [ ] Weak key derivation
- [ ] Insufficient entropy in passphrase
- [ ] Predictable random numbers
- [ ] Key leakage in logs/errors

### Server-Side

- [ ] Path traversal in file ID
- [ ] Injection in file operations
- [ ] Race conditions in collision check
- [ ] Timing side-channels in token verification
- [ ] Denial of service via large uploads

### Client-Side

- [ ] XSS in web interface
- [ ] Key exposure in browser storage
- [ ] Insecure WebSocket connection
- [ ] Memory disclosure of keys

## Reporting Vulnerabilities

If you discover a security vulnerability:

1. **Do not** open a public issue
2. Email security findings to the maintainer
3. Include:
   - Description of vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

Response time: Within 48 hours for acknowledgment

## Test Environment

### Local Setup

```bash
# Clone repository
git clone https://github.com/jonasbg/paste
cd paste

# Run server locally
cd api && go run .

# Run CLI against local server
cd pastectl && go build ./...
./pastectl --url http://localhost:8080 upload -f test.txt
```

### Docker Setup

```bash
docker-compose up -d
# Server at http://localhost:8080
```

## Resources

- [Security Architecture](architecture.md)
- [Cryptography Details](cryptography.md)
- [Threat Model](threat-model.md)
- [Source Code](https://github.com/jonasbg/paste)
