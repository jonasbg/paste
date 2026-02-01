---
title: CLI Reference
description: Complete command reference for pastectl
sidebar_position: 1
---

# CLI Reference

Complete reference for the `pastectl` command-line tool.

## Synopsis

```bash
pastectl [command] [flags]
```

## Commands

| Command | Description |
|---------|-------------|
| `upload` | Upload a file or directory |
| `send` | Alias for upload |
| `download` | Download a file |
| `completion` | Generate shell completion |
| `version` | Show version information |
| `help` | Show help |

## Upload

Upload a file, directory, or stdin to the server.

### Usage

```bash
pastectl upload [flags]
pastectl upload -f <file> [flags]
pastectl send [flags]
pastectl send -f <file> [flags]
cat file | pastectl [flags]
```

### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--file` | `-f` | File or directory to upload | stdin |
| `--name` | `-n` | Override filename | auto-detected |
| `--passphrase` | `-p` | Number of words (4-8) | 4 |
| `--url-mode` | | Use URL mode with 128-bit key | false |
| `--url` | | Custom server URL | `$PASTE_URL` |

### Examples

#### Basic Upload

```bash
# Upload a file
pastectl upload -f document.pdf

# Upload from stdin
echo "Hello World" | pastectl

# Upload with custom name
cat data.json | pastectl -n "config.json"
```

#### Passphrase Options

```bash
# Default (4 words, ~57 bits)
pastectl upload -f file.txt
# → Share code: happy-ocean-forest-moon-x7k3

# More secure (6 words, ~76 bits)
pastectl upload -f sensitive.pdf -p 6
# → Share code: happy-ocean-forest-moon-peak-tree-a2b9

# Maximum (8 words, ~95 bits)
pastectl upload -f topsecret.pdf -p 8
# → Share code: happy-ocean-forest-moon-peak-tree-dawn-echo-x7k3
```

#### URL Mode

```bash
# Use URL mode for maximum security (128 bits)
pastectl upload -f secret.pdf --url-mode
# → https://paste.torden.tech/a1b2c3...#key=Xk9fB2mPqR...
```

#### Directory Upload

```bash
# Directories are automatically compressed
pastectl upload -f ./my-project/
# → Creates tar.gz archive

# Download extracts automatically
pastectl download <passphrase>
# → Extracts to ./my-project/
```

#### Custom Server

```bash
# Use a different server
pastectl upload -f file.txt --url https://my-paste-server.com
```

### Output

On success, displays:
```
Upload complete!

Share code: happy-ocean-forest-moon-x7k3

Download with: pastectl download happy-ocean-forest-moon-x7k3
```

Or for URL mode:
```
https://paste.torden.tech/a1b2c3d4e5f6#key=Xk9fB2mPqRsT...

Download with: pastectl download -l "https://paste.torden.tech/..."
```

## Download

Download and decrypt a file.

### Usage

```bash
pastectl download <passphrase> [flags]
pastectl download -l <url> [flags]
```

### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--link` | `-l` | URL with embedded key | |
| `--output` | `-o` | Output file path | original filename |
| `--url` | | Custom server URL | `$PASTE_URL` |

### Examples

#### Passphrase Download

```bash
# Download with passphrase
pastectl download happy-ocean-forest-moon-x7k3

# Save to specific file
pastectl download happy-ocean-forest-moon-x7k3 -o output.pdf

# Output to stdout
pastectl download happy-ocean-forest-moon-x7k3 -o -
```

#### URL Download

```bash
# Download from URL
pastectl download -l "https://paste.torden.tech/abc123#key=xyz..."

# Save to specific file
pastectl download -l "https://..." -o document.pdf
```

#### Custom Server

```bash
# Download from different server
pastectl download happy-ocean-forest-moon-x7k3 --url https://my-server.com
```

### Output

Shows download progress:
```
Downloading to: document.pdf
████████████████████████████████ 100%
Download complete: document.pdf
```

## Completion

Generate shell completion scripts.

### Usage

```bash
pastectl completion <shell>
```

### Supported Shells

- `bash`
- `zsh`
- `fish`

### Examples

#### Bash

```bash
# System-wide
sudo pastectl completion bash > /etc/bash_completion.d/pastectl

# Current user
pastectl completion bash >> ~/.bashrc
source ~/.bashrc
```

#### Zsh

```bash
# Add to fpath
pastectl completion zsh > "${fpath[1]}/_pastectl"

# Or to specific directory
pastectl completion zsh > ~/.zsh/completions/_pastectl
```

#### Fish

```bash
pastectl completion fish > ~/.config/fish/completions/pastectl.fish
```

## Version

Show version information.

### Usage

```bash
pastectl version
```

### Output

```
pastectl v1.0.0
```

## Help

Show help information.

### Usage

```bash
pastectl help
pastectl --help
pastectl -h
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (see stderr for details) |

## Error Messages

| Error | Cause | Solution |
|-------|-------|----------|
| "File too large" | Exceeds server limit | Use smaller file or different server |
| "Invalid passphrase" | Malformed passphrase | Check spelling, format |
| "Download failed: 403" | Invalid token | Passphrase may be wrong |
| "Share code already in use" | Collision | Retry (auto-generates new passphrase) |
| "Connection refused" | Server unreachable | Check URL, network |

## Stdin/Stdout Behavior

### Upload from Stdin

```bash
# Pipe data
echo "data" | pastectl

# Redirect file
pastectl < file.txt

# Here document
pastectl << EOF
Multiple
lines
of
data
EOF
```

When reading from stdin:
- Default filename: `stdin.txt`
- Use `-n` to override

### Download to Stdout

```bash
# Output to stdout
pastectl download <passphrase> -o -

# Pipe to another command
pastectl download <passphrase> -o - | grep pattern

# Redirect to file
pastectl download <passphrase> -o - > output.txt
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PASTE_URL` | Server URL | `https://paste.torden.tech` |

Example:
```bash
export PASTE_URL=https://my-paste-server.com
pastectl upload -f file.txt  # Uses custom server
```

## See Also

- [Quick Start](../getting-started/quick-start.md)
- [Passphrase Mode](../concepts/passphrase-mode.md)
- [URL Mode](../concepts/url-mode.md)
