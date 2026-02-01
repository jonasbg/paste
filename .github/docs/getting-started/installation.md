---
title: Installation
description: Install the Paste CLI tool or access the web interface
sidebar_position: 1
---

# Installation

Paste can be used through the **web interface** or the **command-line tool (CLI)**. The CLI offers more features and is recommended for regular use.

## Web Interface

No installation required. Visit:

**[paste.torden.tech](https://paste.torden.tech)**

The web interface uses WebAssembly to perform all encryption in your browser. Your files and keys never leave your device unencrypted.

## CLI Installation

### macOS (Homebrew)

```bash
brew install jonasbg/tap/pastectl
```

### Linux

Download the latest binary:

```bash
# For x86_64
curl -L https://github.com/jonasbg/paste/releases/latest/download/pastectl-linux-amd64 \
  -o pastectl

# For ARM64
curl -L https://github.com/jonasbg/paste/releases/latest/download/pastectl-linux-arm64 \
  -o pastectl

# Make executable and move to PATH
chmod +x pastectl
sudo mv pastectl /usr/local/bin/
```

### Windows

Download `pastectl-windows-amd64.exe` from the [releases page](https://github.com/jonasbg/paste/releases) and add it to your PATH.

### From Source

Requires Go 1.21 or later:

```bash
go install github.com/jonasbg/paste/pastectl/cmd/paste@latest
```

## Verify Installation

```bash
pastectl version
```

You should see output like:

```
pastectl v1.0.0
```

## Shell Completion

Enable tab completion for your shell:

### Bash

```bash
# System-wide
pastectl completion bash | sudo tee /etc/bash_completion.d/pastectl

# Current user only
pastectl completion bash >> ~/.bashrc
source ~/.bashrc
```

### Zsh

```bash
pastectl completion zsh > "${fpath[1]}/_pastectl"
```

### Fish

```bash
pastectl completion fish > ~/.config/fish/completions/pastectl.fish
```

## Next Steps

- [Quick Start Guide](quick-start.md) - Upload your first file
- [CLI Reference](../reference/cli.md) - See all available commands
