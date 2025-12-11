<div align="center">

<h1>Paste (pɛjstə)</h1>
<p><strong>Zero-knowledge file sharing server with client-side encryption in Golang</strong></p>

![landing page](.github/docs/index.png)

<p>
<a href="#why">Why</a> ·
<a href="#quick-start">Quick Start</a> ·
<a href="#development">Development</a> ·
<a href="#api">API</a> ·
<a href="#security-implementation">Security</a> ·
<a href="#configuration">Configuration</a>
</p>

</div>

## Why
You need to share files securely but don't trust the server with your data. Traditional file sharing services can see your files, metadata, and encryption keys. This project provides:

- A lightweight Go server (single static binary + embedded SvelteKit UI)
- Client-side AES-GCM encryption using WebAssembly
- Zero server-side knowledge of file contents, names, or encryption keys
- Streaming encryption for large files without memory issues
- Automatic file deletion after configurable retention period

No database required for file storage. No server-side encryption keys. Upload and share with confidence.

## High-Level Architecture
```
                    ┌────────────────────┐
                    │   Web Browser      │
                    │ (SvelteKit + WASM) │
                    └─────────┬──────────┘
                              │ WebSocket/HTTP (encrypted data only)
                    ┌─────────▼────────┐
                    │  Paste Server    │
                    │  - REST API      │
                    │  - Static UI     │
                    │  - File Storage  │
                    │  - SQLite Logs   │
                    └──────────────────┘

Client-side encryption flow:
File → WASM Encryption → Chunked Upload → Server Storage
Server only sees: Encrypted blobs + File IDs + HMAC tokens
```

## Features
- **Client-side encryption**: AES-GCM encryption using WebAssembly in the browser
- **Zero server knowledge**: Server stores only encrypted blobs, never sees keys or metadata
- **Streaming encryption**: Process files in chunks to avoid memory issues with large files
- **WebSocket uploads**: Bypass HTTP proxy size limits with chunked transfers
- **Configurable security**: Support for 128-bit, 192-bit, and 256-bit encryption keys
- **Authenticated encryption**: AEAD with integrity checks and unique IVs per file
- **Automatic cleanup**: Files deleted after configurable retention period
- **Performance optimized**: Batched ACKs, early acknowledgments, and optimized buffers
- **Single binary deployment**: Go server with embedded SvelteKit UI
- **CLI tool (pastectl)**: Command-line interface for uploading and downloading files (named to avoid conflicts with Unix `paste` command)

## Quick Start

### 1. Using Dev Container (Recommended)
Prerequisites: VS Code with Dev Containers extension.

1. Clone and open:
```bash
git clone https://github.com/jonasbg/paste
code paste
```

2. When prompted, click "Reopen in Container"

The dev container provides Go 1.23, Node.js, SQLite, and auto-built WASM encryption.

### 2. Manual Setup
Prerequisites: Go 1.23+, Node.js 18+.

```bash
# Build WASM encryption module
mkdir -p web/static
cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" web/static/
GOOS=js GOARCH=wasm go build -o web/static/encryption.wasm ./wasm/wasm.go

# Build and run backend
cd api && go build -o pastly
./pastly

# Run frontend (separate terminal)
cd ../web
npm install
npm run dev
```

Visit http://localhost:5173 for development or http://localhost:8080 for production.

## Screenshots

### Upload Process
![encryption](.github/docs/encyption.png)
*Client-side WASM encryption before upload*

### Share Link
![sharesheet](.github/docs/sharesheet.png)
*Share encrypted file ID and key*

### Download & Decryption
![download](.github/docs/download.png)
*Client-side decryption with provided key*

![decryption](.github/docs/decryption.png)
*WASM decryption with optional private key input*

## Development

### Frontend (SvelteKit dev server)
```bash
cd web
npm run dev
```

### Backend (Go API serving built assets)
```bash
cd api
go run main.go
```

Hot reloading of frontend is via Vite dev server. Production build is embedded into the Go binary.

### Building for Production
```bash
# Build frontend
cd web
npm ci
npm run build

# Build server with embedded UI
cd ../api
go build -o pastly .
```

## API

Base path: `/api`

| Method | Path | Description |
|--------|------|-------------|
| GET | `/config` | Get server configuration |
| GET | `/download/:id` | Download encrypted blob |
| GET | `/metadata/:id` | Get encrypted metadata |
| DELETE | `/delete/:id` | Delete a file |
| GET | `/ws/upload` | WebSocket upload for large files |
| GET | `/ws/download` | WebSocket download for large files |
| GET | `/metrics/activity` | Server activity statistics |
| GET | `/metrics/storage` | Storage usage statistics |
| GET | `/metrics/requests` | Request statistics |
| GET | `/metrics/security` | Security-related metrics |
| GET | `/metrics/upload-history` | Upload history statistics |

Notes:
- All file data is encrypted client-side before reaching the server
- HMAC tokens provide proof of key possession without exposing keys
- WebSocket endpoints support chunked transfers for large files

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `UPLOAD_DIR` | `./uploads` | Directory where uploaded files are stored |
| `DATABASE_DIR` | `./uploads` | Directory where the SQLite database is stored |
| `WEB_DIR` | `../web` | Directory containing static web files |
| `FILES_RETENTION_DAYS` | `7` | Number of days to keep uploaded files before deletion |
| `LOGS_RETENTION_DAYS` | `180` | Number of days to keep logs (negative for infinite) |
| `MAX_FILE_SIZE` | `100MB` | Maximum allowed size for uploaded files |
| `ID_SIZE` | `64` | Size of the generated IDs (64, 128, 192, 256 bit) |
| `KEY_SIZE` | `128` | Size of the encryption keys (128, 192, 256 bit) |
| `CHUNK_SIZE` | `4` | Size of chunks in MB for transmission |
| `METRICS_ALLOWED_IPS` | `127.0.0.1/8,::1/128` | IP addresses allowed to access metrics endpoints |
| `TRUSTED_PROXIES` | `10.0.0.0/8` | IP ranges of trusted proxies for correct client IP detection |
| `LOG_HASH_SALT` | (empty) | Optional per-instance salt used when hashing client IPs. Setting it changes stored hashes; keep it secret.

## Security Implementation

This section provides a deeper dive into how Paste achieves its security goals.

### Client-Side Encryption
- **Key Generation**: Encryption keys are generated exclusively in the browser using WebAssembly and Go's `crypto/rand` package
- **AES-GCM**: Uses Advanced Encryption Standard in Galois/Counter Mode for authenticated encryption
- **Unique IVs**: Each file and chunk gets a unique 96-bit initialization vector to prevent nonce-reuse attacks
- **Streaming**: Large files are processed in chunks to avoid loading entire files into memory

### Zero-Knowledge Server
- **Encrypted Storage**: Server only stores encrypted blobs, never sees plaintext data
- **No Metadata**: Filenames, content types, and sizes are encrypted client-side
- **Key Isolation**: Encryption keys never leave the client browser
- **HMAC Verification**: Server verifies key possession through HMAC tokens without seeing actual keys

### Data Integrity
- **Authenticated Encryption**: AES-GCM provides both confidentiality and authenticity
- **Chunk Validation**: Each chunk includes authentication tags for tamper detection
- **Header Validation**: Server validates encryption headers before any decryption attempts
- **Unique Identifiers**: Each file gets a unique identifier and encryption key pair

### Performance & Security Balance
- **Chunked Processing**: 1MB chunks balance memory usage and performance
- **WebSocket Transfers**: Bypass HTTP size limits while maintaining security
- **Optimized Buffers**: 64KB WebSocket buffers reduce syscall overhead
- **Early ACKs**: Asynchronous disk operations improve upload throughput

## ❓ FAQ

### Why use WebAssembly for encryption?
While browsers provide the Web Crypto API, it requires loading the entire file into memory for encryption/decryption, which can freeze the browser for large files. WebAssembly allows us to process files in chunks, providing a smoother experience without memory issues.

### Why use WebSockets for file upload?
Many HTTP proxies and servers have file size limits when using traditional multipart form uploads. WebSockets allow us to chunk large files into 1MB blocks, bypassing these limitations while providing upload progress feedback.

### Does that means it violates ToS?
This should only be implemented on your own infrastructure. Most cloud providers and CDNs like Cloudflare have file size limits (e.g. 100MB). You should read their Terms of Service before deploying.

### Is my data really secure?
Yes. All encryption happens in your browser using AES-GCM with configurable key sizes before upload. The server only sees encrypted data and never receives encryption keys or unencrypted metadata. Each file gets a unique identifier, encryption key, and IV (nonce), making it impossible to list or access files without having both the ID and key.

### What encryption algorithm is used?
We use AES-GCM (Galois/Counter Mode) which provides both confidentiality and authentication. For streaming large files, we implement chunked encryption with unique nonces per chunk. Keys are configurable (128/192/256-bit) and generated using cryptographically secure random number generation in the browser.

### How long are files stored?
Files are deleted after 7 days by default. This can be configured with the `FILES_RETENTION_DAYS` environment variable.

### Are there file size limits?
Files are processed in 1MB chunks, allowing for efficient handling of large files. The default maximum file size is 100MB but can be configured. Browser memory constraints and network conditions may affect performance for extremely large files.

### Can I delete files after upload?
Yes, files can be deleted by accessing the download endpoint, which provides a deletion option.

## Performance Tuning

Several optimizations are in place to improve large file transfer throughput:

| Variable | Description | Suggested Values |
|----------|-------------|------------------|
| `CHUNK_SIZE` | Chunk size in MB for upload/download (encryption frames) | 4–8 (test 16 for LAN/high BW) |
| `MAX_FILE_SIZE` | Maximum accepted file size | Keep within infra limits |

Optimizations implemented:
- Increased WebSocket read/write buffers to 64KB (was 1KB) to reduce syscall overhead
- WebSocket download uses configured `CHUNK_SIZE` + 16 bytes (GCM tag) instead of fixed 32KB buffer
- ACKs for download are batched (every 8 chunks) to reduce round‑trip latency
- `/api/download` is excluded from gzip compression since encrypted data is already high entropy
- Upload path sends ACK before persisting chunk (early ack) for better pipeline performance

Further improvements you can try:
1. Raise `CHUNK_SIZE` gradually while monitoring memory and proxy limits
2. Increase ACK batch size or move to single final integrity check
3. Ensure TLS termination and reverse proxy aren't buffering entire WebSocket frames
4. Run benchmarks and monitor CPU, memory, and effective throughput

## Security Considerations
- Files are automatically deleted after retention period (default 7 days)
- Optional IP filtering for metrics endpoints
- No multi-tenant authentication; treat as single-user or trusted group tool
- Consider network policies and access restrictions in production deployments
- Encrypted data is effectively incompressible—avoid middleware that attempts compression

## License

MIT License - see [LICENSE](LICENSE)

---
Feel free to open an issue if you'd like an additional feature or have questions about the security implementation.

## 📝 License

MIT License - see [LICENSE](LICENSE)
