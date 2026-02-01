---
title: URL Mode
description: Maximum security file sharing with embedded keys
sidebar_position: 3
---

# URL Mode

URL mode provides maximum security by generating a cryptographically random 128-bit key embedded in the share URL. Use this when you can share links securely.

## How It Works

When you upload with `--url-mode`:

```bash
pastectl upload -f secret.pdf --url-mode
```

You receive a URL like:

```
https://paste.torden.tech/a1b2c3d4e5f6#key=Xk9fB2mPqRsT_uVwXyZ
└────────────────────────────────────┘└───────────────────────┘
        Sent to server                    URL fragment (never sent)
```

## The URL Fragment

The key is placed after the `#` character, making it a **URL fragment**.

### Why This Matters

URL fragments are **never sent to the server** by browsers:

```
Browser Request:
GET /a1b2c3d4e5f6 HTTP/1.1
Host: paste.torden.tech

(Note: #key=... is NOT included)
```

This is a fundamental property of HTTP - fragments are for client-side use only.

### Server Never Sees the Key

```
Your Browser                              Server
      │                                      │
      ├─── GET /a1b2c3d4e5f6 ───────────────→│
      │    (no key in request)               │
      │                                      │
      │←── Return encrypted blob ────────────┤
      │                                      │
      ├─── Extract key from URL fragment ────│
      │    #key=Xk9fB2mPqRsT_uVwXyZ          │
      │                                      │
      └─── Decrypt locally ──────────────────│
```

## Security Analysis

### Entropy

- **Key size**: 128 bits (or 256 bits for AES-256)
- **Combinations**: 2¹²⁸ ≈ 3.4 × 10³⁸
- **Brute-force time**: Computationally infeasible

For comparison:
- All Bitcoin mining power combined: ~2⁹⁰ hashes/year
- Time to crack 128-bit key: Longer than the age of the universe

### Comparison to Passphrase Mode

| Aspect | Passphrase (4 words) | URL Mode |
|--------|---------------------|----------|
| Entropy | ~57 bits | 128 bits |
| Combinations | 2.2 × 10¹⁷ | 3.4 × 10³⁸ |
| Brute-force | ~2,284 years* | Impossible |

* At 1 billion attempts/second, which is unrealistic for a network service.

## When to Use URL Mode

### Ideal For

- **Email sharing**: Recipients can click the link directly
- **Document embedding**: Links in PDFs, wikis, etc.
- **Maximum security**: When passphrase entropy isn't enough
- **Copy-paste workflows**: Where typing isn't needed

### Not Ideal For

- **Verbal sharing**: Too long to dictate
- **Phone conversations**: Error-prone to read aloud
- **Handwritten notes**: Complex characters

## Usage

### Upload with URL Mode

```bash
# From file
pastectl upload -f document.pdf --url-mode

# From stdin
echo "secret" | pastectl --url-mode

# From directory
pastectl upload -f ./folder/ --url-mode
```

### Download with URL

```bash
# Using -l flag
pastectl download -l "https://paste.torden.tech/a1b2c3...#key=Xk9f..."

# Save to specific file
pastectl download -l "https://paste.torden.tech/a1b2c3...#key=Xk9f..." -o output.pdf
```

### Web Interface

For the web interface, simply share the full URL. The recipient:
1. Opens the link in browser
2. Key is automatically extracted from fragment
3. File is decrypted in browser
4. Download starts

## How Keys Are Generated

URL mode uses the operating system's cryptographic random number generator:

```go
key := make([]byte, 16) // 128 bits
rand.Read(key)          // crypto/rand - OS CSPRNG
```

This provides:
- **True randomness**: From hardware entropy sources
- **Unpredictability**: Each key is independent
- **No patterns**: Unlike passphrase word selection

## URL Structure

```
https://paste.torden.tech/a1b2c3d4e5f6#key=Xk9fB2mPqRsT_uVwXyZ
│       │                  │           │    │
│       │                  │           │    └─ Base64URL-encoded key
│       │                  │           └─ Fragment delimiter
│       │                  └─ File ID (server-generated)
│       └─ Server hostname
└─ Protocol (always HTTPS)
```

### Key Encoding

The key is encoded using Base64URL (RFC 4648):
- Alphabet: `A-Z`, `a-z`, `0-9`, `-`, `_`
- No padding: `=` characters are omitted
- URL-safe: No characters need escaping

## Security Considerations

### Advantages Over Passphrase Mode

1. **Higher entropy**: 128 bits vs 57-95 bits
2. **No wordlist dependency**: Pure random bytes
3. **No collision possible**: Keys are independently random
4. **Copy-paste accuracy**: No transcription errors

### Risks to Consider

1. **URL logging**: Some systems log URLs - ensure the full URL isn't logged
2. **Browser history**: The URL (with key) may be saved in history
3. **Shared computers**: Others might access browser history
4. **Clipboard**: Key might remain in clipboard

### Mitigations

- Share URLs through encrypted channels (Signal, encrypted email)
- Use private/incognito browsing
- Clear clipboard after pasting
- URLs expire after first download

## Technical Details

### Key Derivation (None)

Unlike passphrase mode, URL mode doesn't derive keys:

```
Passphrase Mode:          URL Mode:
passphrase → HKDF → key   random bytes → key
```

The key is used directly for AES-GCM encryption.

### File ID Generation

In URL mode, the server generates the File ID:

```
Server: generateID() → random 64-128 bits → hex string
```

This differs from passphrase mode where the client derives the File ID.

## Comparison Summary

| Feature | Passphrase Mode | URL Mode |
|---------|-----------------|----------|
| Key source | Derived from words | Random bytes |
| Key size | 256 bits (derived) | 128/256 bits |
| Effective entropy | 57-95 bits | 128+ bits |
| File ID | Client-derived | Server-generated |
| Collision check | Required | Not needed |
| Verbal sharing | Easy | Impractical |
| Link sharing | Not applicable | Ideal |

## Further Reading

- [Passphrase Mode](passphrase-mode.md) - Alternative sharing method
- [Encryption Details](encryption.md) - AES-GCM implementation
- [Security Architecture](../security/architecture.md) - Full security overview
