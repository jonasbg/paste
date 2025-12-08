# Paste CLI

A command-line tool for uploading and downloading files to/from paste.torden.tech with end-to-end encryption.

## Features

- Upload files or stdin with client-side encryption
- Download and decrypt files
- Support for pipes and redirects
- Configurable server URL (build-time or runtime)
- Zero-knowledge file sharing

## Installation

### Download Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/jonasbg/paste/releases).

### From Source

```bash
cd cli
go build -ldflags "-s -w" -o paste .
```

### Using Go Install

```bash
go install github.com/jonasbg/paste/cli@latest
```

## Usage

### Upload

Upload from stdin (no command needed when piping):
```bash
echo "Hello World" | paste
cat file.txt | paste
paste < myfile.txt
```

Upload a file:
```bash
paste upload -f document.pdf
```

Upload from stdin with explicit command:
```bash
echo "Hello World" | paste upload
```

Upload with custom filename:
```bash
echo "data" | paste upload -n "my-file.txt"
cat file.txt | paste -n "custom-name.txt"
```

Upload to custom server:
```bash
paste upload -f file.txt -url https://custom.paste.server
```

### Download

Download a file:
```bash
paste download -l "https://paste.torden.tech/abc123#key=xyz..."
```

Download to specific file:
```bash
paste download -l "https://paste.torden.tech/abc123#key=xyz..." -o output.txt
```

Download to stdout:
```bash
paste download -l "https://paste.torden.tech/abc123#key=xyz..." | grep pattern
```

### Other Commands

Show version:
```bash
paste version
```

Show help:
```bash
paste help
```

## Configuration

### Environment Variable

Set the default server URL:
```bash
export PASTE_URL=https://custom.paste.server
paste upload -f file.txt
```

### Build-Time Configuration

Override the default URL at build time:
```bash
go build -ldflags "-s -w -X main.pasteURL=https://custom.paste.server" -o paste .
```

### Runtime Flag

Override via command-line flag:
```bash
paste upload -f file.txt -url https://custom.paste.server
```

## Examples

### Quick Text Upload
```bash
echo "Quick note" | paste
# Output: https://paste.torden.tech/abc123#key=xyz...
```

### Secure File Sharing
```bash
paste upload -f sensitive-data.txt
# Share the URL with someone
# They can download with:
paste download -l "https://paste.torden.tech/abc123#key=xyz..."
```

### Pipeline Integration
```bash
# Compress and upload
tar czf - directory/ | paste -n "backup.tar.gz"

# Download and extract
paste download -l "URL" | tar xzf -
```

### Screenshot Sharing
```bash
# Take screenshot and upload (Linux/X11)
import png:- | paste -n "screenshot.png"

# macOS
screencapture -c && pbpaste | paste -n "screenshot.png"
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
