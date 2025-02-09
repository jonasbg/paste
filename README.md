# Paste (pɛjstə)

Zero-knowledge file sharing server with client-side encryption in Golang. The server never sees unencrypted file contents or metadata.

Paste is created to be an easy and secure way of sharing a file with someone for a short period of time.

![landing page](.github/docs/index.png)

## 🔒 Security Design

- AES-GCM encryption/decryption using WebAssembly in the browser
  - Supports 128-bit, 192-bit, and 256-bit keys (defaults to 128-bit)
  - Uses cryptographically secure random number generation
  - Implements authenticated encryption (AEAD) with integrity checks
  - Unique 96-bit IV (nonce) for each file and chunk
- Streaming encryption for large files
  - Processes files in 1MB chunks to avoid memory issues
  - Ensures unique nonces across chunks using counters
  - Maintains data integrity across chunk boundaries
- Zero server-side knowledge
  - Server stores only encrypted blobs
  - No unencrypted metadata or filenames stored server-side
  - Encryption keys never leave the client
  - Each file gets a unique identifier and encryption key

## 📸 Screenshots

### Upload
![encryption](.github/docs/encyption.png)
*Client-side WASM encryption before upload*

### Share
![sharesheet](.github/docs/sharesheet.png)
*Share encrypted file ID and key*

### Download
![download](.github/docs/download.png)
*Client-side decryption with provided key*

### Decryption
![encryption](.github/docs/decryption.png)
*Client-side WASM decryption with optional private key input*

## 🚀 Development

### Using Dev Container (Recommended)

1. Install [VS Code](https://code.visualstudio.com/) and [Dev Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers)

2. Clone and open:
```bash
git clone https://github.com/jonasbg/paste
code paste
```

3. When prompted, click "Reopen in Container"

The dev container provides:
- Go 1.23 with debugging
- Node.js for SvelteKit
- SQLite for access logging only
- Auto-built WASM encryption

### Manual Setup

```bash
# Build WASM
mkdir -p web/static
cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" web/static/
GOOS=js GOARCH=wasm go build -o web/static/encryption.wasm ./wasm/wasm.go

# Build and run backend
cd api && go build -o pastly
./pastly

# Run frontend
cd ../web
npm install
npm run dev
```

## 🔌 API Endpoints

```bash
POST   /api/upload            # Upload encrypted blob
GET    /api/download/:id      # Download encrypted blob
GET    /api/metadata/:id      # Get encrypted metadata
GET    /api/ws/upload         # WebSocket upload for large files
GET    /api/metrics/*         # Server stats (no file info)
```

## 🌍 Environment Variables

The application can be configured using the following environment variables:

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `UPLOAD_DIR` | Directory where uploaded files are stored | `./uploads` | `/data/uploads` |
| `DATABASE_DIR` | Directory where the SQLite database is stored | `./uploads` | `/data/db` |
| `WEB_DIR` | Directory containing static web files | `../web` | `/app/web` |
| `FILES_RETENTION_DAYS` | Number of days to keep uploaded files before deletion | `7` | `14` |
| `LOGS_RETENTION_DAYS` | Number of days to keep logs (negative for infinite) | `180` | `-1` |

## ❓ FAQ

### Why use WebAssembly for encryption?
While browsers provide the Web Crypto API, it requires loading the entire file into memory for encryption/decryption, which can freeze the browser for large files. WebAssembly allows us to process files in chunks, providing a smoother experience without memory issues.

### Why use WebSockets for file upload?
Many HTTP proxies and servers have file size limits when using traditional multipart form uploads. WebSockets allow us to chunk large files into 1MB blocks, bypassing these limitations while providing upload progress feedback.

### Does that means it violates ToS?
Probably. Most certain. While WebSocket chunking can technically bypass file size limits, this should only be implemented on your own infrastructure. Most cloud providers and CDNs like Cloudflare have file size limits (e.g. 100MB). You should read their Terms of Service.

### Is my data really secure?
Yes. All encryption happens in your browser using AES-GCM with 256-bit keys before upload. The server only sees encrypted data and never receives encryption keys or unencrypted metadata. Each file gets a unique identifier, encryption key, and IV (nonce), making it impossible to list or access files without having both the ID and key.

### What encryption algorithm is used?
We use AES-GCM (Galois/Counter Mode) which provides both confidentiality and authentication. For streaming large files, we implement chunked encryption with unique nonces per chunk. Keys are 256-bit by default and generated using cryptographically secure random number generation in the browser.

### How long are files stored?
Files are deleted after 7 days after uploading.

### Are there file size limits?
Files are processed in 1MB chunks, allowing for efficient handling of large files. While there's no hard size limit, browser memory constraints and network conditions may affect performance for extremely large files.

### Can I delete files after upload?
Yes, by downloading the blob.

## 📝 License

MIT License - see [LICENSE](LICENSE)