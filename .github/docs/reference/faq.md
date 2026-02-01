---
title: FAQ
description: Frequently asked questions about Paste
sidebar_position: 3
---

# Frequently Asked Questions

## General

### What is Paste?

Paste is a zero-trust, end-to-end encrypted file sharing service. It lets you share files using simple, memorable passphrases without needing an account.

### How is it different from Dropbox/Google Drive?

| Feature | Paste | Dropbox/Drive |
|---------|-------|---------------|
| End-to-end encryption | Yes | No |
| Account required | No | Yes |
| Server can read files | No | Yes |
| Passphrase sharing | Yes | No |
| Self-hostable | Yes | No |

### Is it really secure?

Yes. The encryption key is derived from your passphrase on your device. The server only receives encrypted data it cannot decrypt. Even if the server is compromised, your files remain protected.

### Who can see my files?

Only someone with the passphrase can decrypt your files. The server cannot read them, the server operators cannot read them, and attackers who compromise the server cannot read them.

## Passphrases

### How do passphrases work?

Your passphrase (like `happy-ocean-forest-moon-x7k3`) is used to derive two things:
1. **File ID**: Where the encrypted file is stored
2. **Encryption key**: How the file is encrypted

The recipient uses the same passphrase to derive the same values.

### How secure is a 4-word passphrase?

A 4-word passphrase with suffix has approximately 57 bits of entropy. At 1 billion guesses per second, it would take over 2,000 years to try all combinations. In practice, network latency makes attacks much slower.

### Can I choose my own passphrase?

No. Passphrases are randomly generated to ensure sufficient entropy. User-chosen passphrases tend to be predictable (common phrases, personal information).

### What if two people pick the same passphrase?

The random suffix (like `x7k3`) makes this extremely unlikely. Additionally, the server rejects uploads if the file ID already exists, preventing collisions.

### Is the wordlist secret?

No, and it doesn't need to be. Security comes from the randomness of word selection, not from hiding the list. This is the same principle used by BIP39 (cryptocurrency wallets).

## Security

### Can the server read my files?

No. The server only stores encrypted blobs. The encryption key never leaves your device.

### What if the server is hacked?

Attackers would only get encrypted data they cannot decrypt. Your files remain protected by your passphrase.

### Should I use passphrase mode or URL mode?

| Use Case | Recommended Mode |
|----------|------------------|
| Sharing verbally | Passphrase |
| Sharing via chat | Passphrase |
| Maximum security | URL mode |
| Clickable links | URL mode |
| Phone conversations | Passphrase |

### How do I increase security?

Use more words:
```bash
pastectl upload -f file.txt -p 6  # 6 words, ~76 bits
pastectl upload -f file.txt -p 8  # 8 words, ~95 bits
```

Or use URL mode for 128-bit keys:
```bash
pastectl upload -f file.txt --url-mode
```

### What encryption is used?

AES-256-GCM (Galois/Counter Mode) with keys derived using HKDF-SHA256. These are industry-standard algorithms used by TLS, SSH, and other secure protocols.

## Files

### What's the maximum file size?

Depends on the server configuration. The default limit is 5 GB. Self-hosted instances can configure different limits.

### What happens after download?

Files are automatically deleted from the server after the first successful download. This is a security feature to limit exposure.

### Can I download a file multiple times?

No. Files are deleted after the first download. If you need to share with multiple people, upload the file multiple times or share the passphrase with everyone before anyone downloads.

### Can I cancel an upload?

Yes. Close the CLI or browser tab. Incomplete uploads are automatically cleaned up by the server.

### Are directories supported?

Yes. Directories are automatically compressed into a tar.gz archive:
```bash
pastectl upload -f ./my-folder/
```

The recipient receives the archive, which can be extracted:
```bash
tar -xzf my-folder.tar.gz
```

## Usage

### How do I upload from stdin?

Pipe data to pastectl:
```bash
echo "secret" | pastectl
cat file.txt | pastectl
tar -czf - ./folder | pastectl -n "folder.tar.gz"
```

### How do I specify the filename?

Use the `-n` flag:
```bash
cat data | pastectl -n "important.txt"
```

### Can I use a different server?

Yes:
```bash
pastectl upload -f file.txt --url https://my-server.com
```

Or set the environment variable:
```bash
export PASTE_URL=https://my-server.com
```

### How do I self-host?

```bash
docker run -p 8080:8080 ghcr.io/jonasbg/paste:latest
```

See [Self-Hosting](../self-hosting/docker.md) for details.

## Troubleshooting

### "Invalid passphrase"

- Check spelling carefully
- Ensure all lowercase
- Verify hyphen placement
- Confirm suffix is correct

### "Download failed: 403"

- Passphrase may be incorrect
- File may have already been downloaded (files are one-time)
- File may have expired

### "Share code already in use"

This is rare but can happen. The client automatically retries with a new passphrase. If it persists, try again.

### "File too large"

The file exceeds the server's size limit. Options:
- Compress the file
- Split into multiple parts
- Use a server with higher limits

### "Connection refused"

- Check internet connection
- Verify server URL is correct
- Server may be down

## Privacy

### What data is collected?

The hosted service (paste.torden.tech) collects minimal operational data:
- Request timestamps
- Encrypted file sizes
- IP addresses (for rate limiting)

It does NOT have access to:
- File contents
- Filenames
- Passphrases

### Are there logs?

Server logs contain only:
- Timestamps
- File IDs (not passphrases)
- IP addresses
- Error messages

Logs do not contain any decryptable content.

### How long are files stored?

Files are deleted:
1. After first download (immediately)
2. After expiration period (configurable, default varies)

Self-hosted instances can configure retention policies.

## Contributing

### How can I contribute?

- Report bugs on [GitHub Issues](https://github.com/jonasbg/paste/issues)
- Submit pull requests
- Improve documentation
- Spread the word

### Is it open source?

Yes, fully open source under the MIT license.

### Can I audit the code?

Absolutely. Security audits are welcome. See the [Audit Guide](../security/audit-guide.md).
