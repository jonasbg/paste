# Paste (pɛjstə)

Zero-knowledge file sharing server with client-side encryption in Golang. The server never sees unencrypted file contents or metadata.

![landing page](.github/docs/index.png)

## 🔒 Security Design

- All encryption/decryption happens in the browser using WebAssembly
- Server stores only encrypted blobs
- No metadata or filenames stored server-side
- Encryption keys never leave the client
- Each file gets a unique identifier

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

## 📝 License

MIT License - see [LICENSE](LICENSE)