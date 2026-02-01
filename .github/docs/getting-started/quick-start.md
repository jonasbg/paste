---
title: Quick Start
description: Upload and share your first file in 60 seconds
sidebar_position: 2
---

# Quick Start

This guide walks you through uploading and downloading your first encrypted file.

## Upload a File

### From a File

```bash
pastectl upload -f document.pdf
```

Output:
```
Upload complete!

Share code: calm-river-sunset-peak-a2b9

Download with: pastectl download calm-river-sunset-peak-a2b9
```

### From stdin

```bash
echo "Hello, World!" | pastectl
```

Or pipe any command:

```bash
cat secret.txt | pastectl
tar -czf - ./folder | pastectl -n "folder.tar.gz"
```

### Upload a Directory

Directories are automatically compressed:

```bash
pastectl upload -f ./my-project/
```

The recipient will receive a `.tar.gz` archive.

## Share the Passphrase

Share the passphrase with your recipient through any channel:

- Tell them verbally: *"The code is calm-river-sunset-peak-a2b9"*
- Send via chat or email
- Write it down

The passphrase is all they need to download and decrypt the file.

## Download a File

Your recipient runs:

```bash
pastectl download calm-river-sunset-peak-a2b9
```

Output:
```
Downloading to: document.pdf
Download complete: document.pdf
```

### Save to a Specific Location

```bash
pastectl download calm-river-sunset-peak-a2b9 -o ~/Downloads/mydoc.pdf
```

### Output to stdout

```bash
pastectl download calm-river-sunset-peak-a2b9 -o - | less
```

## What Just Happened?

1. **Upload**: Your file was encrypted on your device using a key derived from the passphrase
2. **Transfer**: Only the encrypted blob was sent to the server
3. **Storage**: The server stored the encrypted data (it cannot read it)
4. **Download**: The recipient used the passphrase to derive the same key and decrypt

The server **never** had access to:
- Your encryption key
- Your file contents
- Your filename or file type

## Increase Security

For sensitive files, use more words:

```bash
# 6 words (~76 bits entropy)
pastectl upload -f tax-returns.pdf -p 6
# Output: calm-river-sunset-peak-moon-tree-b4k9

# 8 words (~95 bits entropy)
pastectl upload -f top-secret.pdf -p 8
# Output: calm-river-sunset-peak-moon-tree-dawn-echo-x2m7
```

## Use URL Mode

For maximum security when you can share a link:

```bash
pastectl upload -f secret.pdf --url-mode
```

Output:
```
https://paste.torden.tech/a1b2c3d4...#key=Xk9fB2mPqRsT...
```

The key after `#` is never sent to the server (URL fragments stay client-side).

## Next Steps

- [Passphrase Mode](../concepts/passphrase-mode.md) - Understand how passphrases work
- [URL Mode](../concepts/url-mode.md) - When to use URL-based sharing
- [CLI Reference](../reference/cli.md) - All commands and options
