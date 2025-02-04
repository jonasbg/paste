# Droply

A simple and secure file sharing server written in Go, supporting client-side encryption and large file uploads.

## Features

- Client-side file encryption using WebAssembly
- Large file support (up to 1GB)
- Configurable upload directory
- Fast and efficient file serving
- Docker support
- Minimal dependencies
- Error logging with request context

## Installation

### Prerequisites

- Go 1.23 or later
- Docker (optional)

### Local Setup

1. Clone the repository:
```bash
git clone https://github.com/yourusername/droply
cd droply
```

2. Build the project:
```bash
go build -o droply
```

3. Run the server:
```bash
./droply
```

### Docker Setup

1. Build the Docker image:
```bash
docker build -t droply .
```

2. Run the container:
```bash
docker run -d \
  --name droply \
  -p 8080:8080 \
  -v $(pwd)/uploads:/uploads \
  -e UPLOAD_DIR=/uploads \
  droply
```

## Configuration

The server can be configured using environment variables:

- `UPLOAD_DIR`: Directory where uploaded files will be stored (default: "./uploads")

## API Endpoints

### File Upload
```
POST /upload
Content-Type: multipart/form-data

Form field: file
```

Response:
```json
{
    "id": "generated-file-id"
}
```

### File Download
```
GET /download/{id}
```

### File Metadata
```
GET /metadata/{id}
```

## File Format

Files are stored with the following structure:
- First 12 bytes: Initialization Vector (IV)
- Next 4 bytes: Metadata length (little-endian uint32)
- Next N bytes: Encrypted metadata (JSON)
- Remaining bytes: Encrypted file content

## Security

- All file encryption is performed client-side using WebAssembly
- Server never receives unencrypted data
- Each file gets a unique 32-character hexadecimal identifier
- Files are stored with encrypted metadata separate from content
- Access requires the complete file ID

## Development

### Requirements

- Go 1.22+
- make (optional, for build scripts)
- Docker (optional, for containerization)

### Build Commands

Build the server:
```bash
go build
```

Run tests:
```bash
go test ./...
```

### Directory Structure

```
.
├── main.go          # Server entry point
├── Dockerfile       # Docker configuration
├── index.html       # Web client
├── wasm_exec.js     # Go WebAssembly support
└── encryption.wasm  # WebAssembly encryption module
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.