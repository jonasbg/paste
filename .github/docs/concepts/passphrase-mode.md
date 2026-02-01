---
title: Passphrase Mode
description: Share files using memorable word-based codes
sidebar_position: 2
---

# Passphrase Mode

Passphrase mode is the default sharing method in Paste. It generates memorable codes that can be easily shared verbally or through text.

## How It Works

When you upload a file, Paste generates a random passphrase:

```
happy-ocean-forest-moon-x7k3
└─────────────────────┘ └──┘
      4 words        suffix
```

This passphrase is used to derive both:
- **File ID**: Where the file is stored on the server
- **Encryption Key**: How the file is encrypted

The recipient uses the same passphrase to derive the same values and decrypt.

## Passphrase Format

### Structure

```
word-word-word-word-suffix
```

- **Words**: 4-8 random words from a 600-word list
- **Suffix**: 4 alphanumeric characters with at least one digit

### Examples

```
happy-ocean-forest-moon-x7k3     (4 words - default)
calm-river-sunset-peak-tree-a2b9 (5 words)
dawn-echo-wild-storm-peak-moon-f4m2 (6 words)
```

### Why This Format?

| Component | Purpose |
|-----------|---------|
| Words | Memorable, easy to speak |
| Hyphens | Clear word boundaries |
| Suffix | Prevents collision, adds entropy |
| Digit in suffix | Distinguishes suffix from words |

## Security Analysis

### Entropy Calculation

| Words | Calculation | Entropy | Equivalent Password |
|-------|-------------|---------|---------------------|
| 4 | 600⁴ × 36⁴ | ~57 bits | 10-char alphanumeric |
| 5 | 600⁵ × 36⁴ | ~67 bits | 11-char alphanumeric |
| 6 | 600⁶ × 36⁴ | ~76 bits | 13-char alphanumeric |
| 8 | 600⁸ × 36⁴ | ~95 bits | 16-char alphanumeric |

### Brute-Force Resistance

At 1 billion guesses per second (unrealistic for network service):

| Words | Time to Exhaustive Search |
|-------|---------------------------|
| 4 | ~2,284 years |
| 5 | ~1.4 million years |
| 6 | ~821 million years |
| 8 | ~12 trillion years |

In practice, network latency and rate limiting make attacks much slower.

### The Wordlist

The wordlist contains approximately 600 common English words:

- **Short**: 3-8 characters each
- **Memorable**: Common, everyday words
- **Distinct**: Phonetically different to avoid confusion
- **Public**: Security comes from randomness, not secrecy

Sample words: `ocean`, `forest`, `moon`, `river`, `storm`, `dawn`, `peak`, `calm`

The full list is in the source code at `/crypto/wordlist.go`.

## Key Derivation

The passphrase derives both File ID and encryption key using HKDF:

```
Passphrase: "happy-ocean-forest-moon-x7k3"
                    │
                    ▼
            ┌───────────────┐
            │   HKDF-SHA256 │
            └───────┬───────┘
                    │
        ┌───────────┴───────────┐
        ▼                       ▼
     Context:                Context:
   "paste-v1-file-id"     "paste-v1-encryption-key"
        │                       │
        ▼                       ▼
   File ID                 Encryption Key
   (16 bytes)              (32 bytes)
```

### Why HKDF?

- **Deterministic**: Same passphrase always produces same outputs
- **Independent**: File ID and key are cryptographically independent
- **Secure**: Based on HMAC-SHA256, well-analyzed

### Why No Salt?

Traditional password hashing uses random salt, but Paste intentionally omits it:

- **Requirement**: Recipient must derive same File ID to find the file
- **Trade-off**: Same passphrase = same File ID (mitigated by collision check)
- **Mitigation**: Random suffix ensures uniqueness

## Collision Prevention

### The Problem

If two users choose similar passphrases, they could collide:
- User A uploads with `happy-ocean-forest-moon-a1b2`
- User B uploads with `happy-ocean-forest-moon-a1b2` (same by chance)

### The Solution

1. **Random suffix**: 36⁴ = 1.68 million possibilities per word combination
2. **Server-side check**: Rejects if File ID already exists

```
Server: "Share code already in use, please try again"
```

The client automatically retries with a new passphrase.

## When to Use Passphrase Mode

### Ideal For

- Sharing files with colleagues or friends
- When you can communicate verbally or via chat
- General-purpose secure file sharing
- Situations where URLs are inconvenient

### Consider URL Mode When

- Sharing via email where clickable links work better
- You need maximum security (128-bit keys)
- You're embedding links in documents
- The recipient might mistype a passphrase

## Usage Examples

### Basic Upload

```bash
pastectl upload -f document.pdf
# Share code: calm-river-sunset-peak-a2b9
```

### Increased Security

```bash
# 6 words for sensitive files
pastectl upload -f financial-records.xlsx -p 6
# Share code: calm-river-sunset-peak-moon-tree-b4k9

# 8 words for highly confidential files
pastectl upload -f trade-secrets.pdf -p 8
# Share code: calm-river-sunset-peak-moon-tree-dawn-echo-x2m7
```

### Download

```bash
pastectl download calm-river-sunset-peak-a2b9
```

## Comparison with URL Mode

| Aspect | Passphrase Mode | URL Mode |
|--------|-----------------|----------|
| Entropy | 57-95 bits | 128 bits |
| Shareable verbally | Yes | No |
| Easy to type | Yes | No |
| Clickable | No | Yes |
| Risk of typos | Medium | Low (copy/paste) |

## Further Reading

- [URL Mode](url-mode.md) - Alternative sharing method
- [Encryption Details](encryption.md) - How encryption works
- [Cryptography](../security/cryptography.md) - Technical deep-dive
