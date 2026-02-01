---
title: Threat Model
description: What Paste protects against and its limitations
sidebar_position: 3
---

# Threat Model

This document describes the threats Paste is designed to protect against and acknowledges its limitations.

## Assets

What we're protecting:

| Asset | Description |
|-------|-------------|
| File contents | The actual data being shared |
| File metadata | Filename, type, size |
| Passphrase | The secret used to encrypt |
| Encryption key | Derived from passphrase or random |

## Threat Actors

### Passive Network Attacker

**Capabilities**: Can observe network traffic
**Goal**: Read file contents or metadata

| Attack | Mitigation | Status |
|--------|------------|--------|
| Intercept HTTP | TLS required | Protected |
| Intercept ciphertext | E2E encryption | Protected |
| Traffic analysis | Encrypted size only visible | Partial |

### Active Network Attacker

**Capabilities**: Can intercept and modify traffic
**Goal**: Compromise files or obtain keys

| Attack | Mitigation | Status |
|--------|------------|--------|
| MITM attack | TLS certificate validation | Protected |
| Inject malicious data | GCM authentication | Protected |
| Replay upload | Server-side dedup | Protected |
| Replay download | File deleted after download | Protected |

### Malicious Server Operator

**Capabilities**: Full server access
**Goal**: Access file contents

| Attack | Mitigation | Status |
|--------|------------|--------|
| Read stored files | E2E encryption | Protected |
| Read metadata | Metadata encrypted | Protected |
| Brute-force passphrases | High entropy required | Protected |
| Modify encrypted files | GCM authentication | Protected |
| Log encryption keys | Keys never sent | Protected |

### External Attacker (Server Compromise)

**Capabilities**: Gained unauthorized server access
**Goal**: Mass data exfiltration

| Attack | Mitigation | Status |
|--------|------------|--------|
| Dump database | Only encrypted blobs | Protected |
| Read file contents | No keys available | Protected |
| Backdoor future uploads | Client-side encryption | Protected |
| Impersonate server | TLS certificate | Protected* |

*Requires attacker to not have obtained TLS private key

### Brute-Force Attacker

**Capabilities**: Computational resources
**Goal**: Guess passphrase to decrypt specific file

| Mode | Entropy | Resistance |
|------|---------|------------|
| 4 words + suffix | ~57 bits | Strong |
| 5 words + suffix | ~67 bits | Very strong |
| 6 words + suffix | ~76 bits | Excellent |
| 8 words + suffix | ~95 bits | Infeasible |
| URL mode | 128 bits | Computationally impossible |

**Analysis at 10⁹ guesses/second:**

- 57 bits: ~2,284 years average
- 67 bits: ~1.4 million years
- 76 bits: ~821 million years
- 128 bits: ~5.4 × 10²¹ years

Network rate limiting makes this much slower in practice.

## Trust Boundaries

### Trusted Components

| Component | Why Trusted |
|-----------|-------------|
| Client device | Where crypto happens |
| CLI binary | Performs encryption |
| Web browser | Executes WASM |
| WASM module | Performs encryption |
| Passphrase channel | How secret is shared |

### Untrusted Components

| Component | Why Untrusted |
|-----------|---------------|
| Server | By design - zero-trust |
| Network | Assumed hostile |
| Server operators | Cannot access data |
| Other users | Cannot access your files |

## Attack Scenarios

### Scenario 1: Insider Threat

**Threat**: Malicious server administrator
**Attack**: Attempts to read user files

```
Admin accesses: /uploads/a7f3b2c1.Xk9fB2mP...
                            └── Encrypted blob

Result: Cannot decrypt without passphrase
```

**Outcome**: Protected

### Scenario 2: Database Breach

**Threat**: Attacker dumps entire upload directory
**Attack**: Attempts offline analysis

```
Attacker obtains:
├── Encrypted file 1
├── Encrypted file 2
└── ...

Each file requires unique passphrase to decrypt
Brute-force each: ~2,284 years per file (4 words)
```

**Outcome**: Protected (impractical attack)

### Scenario 3: Targeted Attack

**Threat**: Nation-state targeting specific user
**Attack**: Focused resources on one passphrase

```
Assumptions:
- Attacker knows target uploaded a file
- Attacker has file ID (e.g., from logs)
- 10¹² guesses/second (supercomputer cluster)

4 words: ~83 days average
6 words: ~23,000 years average
8 words: ~40 billion years average
```

**Outcome**: Use more words for high-value targets

### Scenario 4: Passphrase Interception

**Threat**: Attacker observes passphrase sharing
**Attack**: Uses intercepted passphrase

```
User shares: "happy-ocean-forest-moon-x7k3"
Attacker sees passphrase
Attacker downloads and decrypts file
```

**Outcome**: NOT protected - passphrase is the secret

**Mitigation**: Share passphrases through secure channels

### Scenario 5: Client Compromise

**Threat**: Malware on user's device
**Attack**: Intercepts passphrase or decrypted file

```
Malware captures:
- Passphrase as user types
- Decrypted file in memory
- Clipboard contents
```

**Outcome**: NOT protected - client must be trusted

## Limitations

### What Paste Does NOT Protect Against

| Threat | Reason |
|--------|--------|
| Compromised client | Client must be trusted for crypto |
| Intercepted passphrase | Passphrase IS the secret |
| Weak custom passphrases | System-generated only |
| Coercion | User may be forced to reveal passphrase |
| Metadata leakage (timing) | Upload/download times visible |
| File size inference | Encrypted size ≈ original size |

### Residual Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Passphrase collision | Very low | Medium | Random suffix + server check |
| Nonce reuse | Extremely low | High | Proper implementation |
| Crypto breakthrough | Very low | Critical | Use standard algorithms |
| Implementation bug | Low | Variable | Code review, testing |

## Recommendations by Use Case

### General File Sharing

- **Mode**: Passphrase (4 words)
- **Risk level**: Low
- **Suitable for**: Documents, photos, general files

### Sensitive Documents

- **Mode**: Passphrase (6+ words)
- **Risk level**: Medium
- **Suitable for**: Financial, medical, legal documents

### High-Security Files

- **Mode**: URL mode (128 bits)
- **Risk level**: High
- **Suitable for**: Trade secrets, classified information

### Maximum Security

- **Mode**: URL mode + additional precautions
- **Additional**: Secure passphrase channel, verified recipients
- **Suitable for**: Nation-state threat model

## Security Assumptions

For Paste's security model to hold:

1. **AES-256-GCM remains secure**: No practical attacks
2. **SHA-256 remains collision-resistant**: For HKDF/HMAC
3. **OS CSPRNG provides good randomness**: For key generation
4. **TLS is properly configured**: Valid certificates, modern ciphers
5. **Client software is authentic**: Not backdoored
6. **Passphrase remains secret**: Shared only with intended recipient

## Further Reading

- [Security Architecture](architecture.md) - Technical implementation
- [Cryptography](cryptography.md) - Algorithm details
- [Audit Guide](audit-guide.md) - For security researchers
