# Pastectl

A command-line tool for uploading and downloading files to/from paste.torden.tech with end-to-end encryption. Named `pastectl` to avoid conflicts with the Unix `paste` command.

## Features

- Upload files or stdin with client-side encryption
- Download and decrypt files
- Support for pipes and redirects
- Configurable server URL (build-time or runtime)
- Zero-knowledge file sharing

## Installation

### Download Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/jonasbg/paste/releases).

Archives are named `pastectl-{platform}.tar.gz` (or `.zip` for Windows), and contain a binary named `pastectl` (or `pastectl.exe`).

### From Source

```bash
cd pastectl
go build -ldflags "-s -w" -o pastectl ./cmd/paste
```

### Using Go Install

```bash
go install github.com/jonasbg/paste/pastectl/cmd/paste@latest
```

## Usage

### Upload

Upload from stdin (no command needed when piping):
```bash
echo "Hello World" | pastectl
cat file.txt | pastectl
pastectl < myfile.txt
```

Upload a file:
```bash
pastectl upload -f document.pdf
```

Upload from stdin with explicit command:
```bash
echo "Hello World" | pastectl upload
```

Upload with custom filename:
```bash
echo "data" | pastectl upload -n "my-file.txt"
cat file.txt | pastectl -n "custom-name.txt"
```

Upload to custom server:
```bash
pastectl upload -f file.txt -url https://custom.paste.server
```

### Download

Download a file:
```bash
pastectl download -l "https://paste.torden.tech/abc123#key=xyz..."
```

Download to specific file:
```bash
pastectl download -l "https://paste.torden.tech/abc123#key=xyz..." -o output.txt
```

Download to stdout:
```bash
pastectl download -l "https://paste.torden.tech/abc123#key=xyz..." | grep pattern
```

### Other Commands

Show version:
```bash
pastectl version
```

Show help:
```bash
pastectl help
```

## Configuration

### Environment Variable

Set the default server URL:
```bash
export PASTE_URL=https://custom.paste.server
pastectl upload -f file.txt
```

### Build-Time Configuration

Override the default URL at build time:
```bash
go build -ldflags "-s -w -X main.pasteURL=https://custom.paste.server" -o pastectl .
```

### Runtime Flag

Override via command-line flag:
```bash
pastectl upload -f file.txt -url https://custom.paste.server
```

## Examples

### Quick Text Upload
```bash
echo "Quick note" | pastectl
# Output: https://paste.torden.tech/abc123#key=xyz...
```

### Secure File Sharing
```bash
pastectl upload -f sensitive-data.txt
# Share the URL with someone
# They can download with:
pastectl download -l "https://paste.torden.tech/abc123#key=xyz..."
```

### Pipeline Integration
```bash
# Compress and upload
tar czf - directory/ | pastectl -n "backup.tar.gz"

# Download and extract
pastectl download -l "URL" | tar xzf -
```

### Screenshot Sharing
```bash
# Take screenshot and upload (Linux/X11)
import png:- | pastectl -n "screenshot.png"

# macOS
screencapture -c && pbpaste | pastectl -n "screenshot.png"
```

## Security

- All files are encrypted client-side using AES-GCM-256
- Encryption keys never leave your device
- Server only stores encrypted data
- Keys are included in the URL fragment (not sent to server)

## Technical Details

- **Encryption**: AES-GCM with 256-bit keys (configurable)
- **Upload Protocol**: WebSocket with chunked streaming
- **Download Protocol**: HTTP with streaming
- **Authentication**: HMAC-SHA256 tokens derived from encryption key
- **Chunk Size**: 4MB (configurable server-side)

## License

Same as the main paste repository.
