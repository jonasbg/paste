package handlers

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jonasbg/paste/m/v2/telemetry"
)

const (
	// pingInterval is how often the server sends a WebSocket ping frame to keep
	// reverse proxies (nginx, caddy, …) from timing out idle connections.
	pingInterval = 25 * time.Second
	// pongWait is the maximum time we wait for the next client message or pong.
	// Any read deadline is extended to now+pongWait after each received message.
	pongWait = 60 * time.Second
	// writeWait is the maximum time we allow for a single write to complete.
	writeWait = 10 * time.Second
)

var upgrader = websocket.Upgrader{
	// Larger buffers reduce syscall overhead for large binary frames (default 1KB -> 64KB)
	ReadBufferSize:  64 * 1024,
	WriteBufferSize: 64 * 1024,
	CheckOrigin:     checkOrigin,
}

// checkOrigin blocks cross-site WebSocket use from other websites' pages.
// Browsers always send Origin; non-browser clients such as pastectl do not.
// Extra origins (e.g. when a proxy rewrites Host) come from ALLOWED_ORIGINS.
func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, allowed := range GlobalConfig.AllowedOrigins {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}

// startPingLoop sends periodic WebSocket pings so reverse proxies do not cut
// idle connections during large uploads/downloads. The goroutine exits when ctx
// is cancelled or a write fails. The caller must configure ws.SetPongHandler to
// reset the read deadline on each pong received.
//
// IMPORTANT: pings are sent via WriteControl, not WriteMessage.
// Gorilla WebSocket allows only one concurrent writer, but WriteControl is
// explicitly documented as safe to call concurrently with all other methods.
// Using WriteMessage here would race with the main goroutine's data writes and
// silently corrupt frames.
func startPingLoop(ctx context.Context, ws *websocket.Conn) {
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := ws.WriteControl(
					websocket.PingMessage,
					nil,
					time.Now().Add(writeWait),
				); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// wsWriteJSON sets a write deadline and then serialises v as JSON to ws.
func wsWriteJSON(ws *websocket.Conn, v any) error {
	ws.SetWriteDeadline(time.Now().Add(writeWait))
	return ws.WriteJSON(v)
}

func HandleWSDownload(uploadDir string, metrics *telemetry.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		// Download clients only send small JSON control messages.
		ws.SetReadLimit(64 * 1024)

		// Keepalive: extend read deadline on every pong.
		ws.SetReadDeadline(time.Now().Add(pongWait))
		ws.SetPongHandler(func(string) error {
			ws.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})

		ctx, cancelPing := context.WithCancel(context.Background())
		defer cancelPing()
		startPingLoop(ctx, ws)

		// Get initial request with fileId and token
		_, msg, err := ws.ReadMessage()
		if err != nil {
			sendWSError(ws, "Failed to read initial message")
			return
		}
		ws.SetReadDeadline(time.Now().Add(pongWait))

		var request struct {
			Type   string `json:"type"`
			FileId string `json:"fileId"`
			Token  string `json:"token"`
		}

		if err := json.Unmarshal(msg, &request); err != nil {
			sendWSError(ws, "Invalid message format")
			return
		}

		if request.Type != "download_init" {
			sendWSError(ws, "Invalid message type: expected 'download_init'")
			return
		}

		// Validate fileId format
		if !validateID(request.FileId) {
			sendWSError(ws, "Invalid file ID format")
			return
		}

		// Validate token format
		if !validateToken(request.Token) {
			sendWSError(ws, "Invalid token format")
			return
		}

		// Locate file with the exact token - this is the security check
		// The file name MUST match fileId.token exactly
		filePath := filepath.Join(uploadDir, request.FileId+"."+request.Token)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			// Return generic error to prevent token enumeration
			sendWSError(ws, "Access denied")
			return
		}

		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Error: Failed to open file: %v", err)
			sendWSError(ws, "Server error: Cannot open file")
			return
		}
		defer file.Close()

		// Get file info for size
		fileInfo, err := file.Stat()
		if err != nil {
			log.Printf("Error: Failed to get file info: %v", err)
			sendWSError(ws, "Server error: Cannot get file info")
			return
		}

		// Send file size info
		if err := wsWriteJSON(ws, gin.H{
			"type": "file_info",
			"size": fileInfo.Size(),
		}); err != nil {
			log.Printf("Failed to send file info: %v", err)
			return
		}

		// Read client ready confirmation
		_, readyMsg, err := ws.ReadMessage()
		if err != nil {
			log.Printf("Failed to read ready message: %v", err)
			return
		}
		ws.SetReadDeadline(time.Now().Add(pongWait))

		var readyResp struct {
			Type  string `json:"type"`
			Ready bool   `json:"ready"`
		}
		if err := json.Unmarshal(readyMsg, &readyResp); err != nil || readyResp.Type != "ready" || !readyResp.Ready {
			log.Printf("Client not ready: %v", err)
			return
		}

		// Stream file in chunks
		// Use the configured chunk size (+16 tag) to match upload pipeline; fall back to 1MB if unset
		configuredChunkBytes := GlobalConfig.ChunkSize * 1024 * 1024
		if configuredChunkBytes <= 0 {
			configuredChunkBytes = 1 * 1024 * 1024
		}
		buffer := make([]byte, configuredChunkBytes+16)
		var totalSent int64 = 0
		var isComplete = false
		// Ack batching: require client to ack every batchAckInterval chunks instead of every chunk
		const batchAckInterval = 8 // tuneable; higher reduces round trips
		chunksSinceAck := 0

		for {
			n, err := file.Read(buffer)
			if n > 0 {
				ws.SetWriteDeadline(time.Now().Add(writeWait))
				if err := ws.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
					log.Printf("Error sending chunk: %v", err)
					return
				}
				totalSent += int64(n)
				chunksSinceAck++

				// Only wait for an ACK every batchAckInterval chunks to improve throughput
				if chunksSinceAck >= batchAckInterval {
					_, ackMsg, err := ws.ReadMessage()
					if err != nil {
						log.Printf("Error receiving ack: %v", err)
						return
					}
					ws.SetReadDeadline(time.Now().Add(pongWait))
					var ack struct {
						Type string `json:"type"`
						Size int    `json:"size"`
					}
					if err := json.Unmarshal(ackMsg, &ack); err != nil || ack.Type != "ack" || ack.Size != n {
						log.Printf("Invalid ack: %v (expected size %d, got %d)", err, n, ack.Size)
						return
					}
					chunksSinceAck = 0
				}
			}

			if err == io.EOF {
				// Flush final ack if there are outstanding unacked chunks
				if chunksSinceAck > 0 {
					_, ackMsg, err := ws.ReadMessage()
					if err != nil {
						log.Printf("Error receiving final batch ack: %v", err)
						return
					}
					ws.SetReadDeadline(time.Now().Add(pongWait))
					var ack struct {
						Type string `json:"type"`
					}
					if err := json.Unmarshal(ackMsg, &ack); err != nil || ack.Type != "ack" {
						log.Printf("Invalid final batch ack: %v", err)
						return
					}
				}

				if err := wsWriteJSON(ws, gin.H{"type": "complete", "size": totalSent}); err != nil {
					log.Printf("Failed to send complete message: %v", err)
					return
				}
				_, completeMsg, err := ws.ReadMessage()
				if err != nil {
					log.Printf("Error receiving final ack: %v", err)
					return
				}
				ws.SetReadDeadline(time.Now().Add(pongWait))
				var complete struct {
					Type     string `json:"type"`
					Complete bool   `json:"complete"`
				}
				if err := json.Unmarshal(completeMsg, &complete); err != nil || complete.Type != "complete_ack" || !complete.Complete {
					log.Printf("Invalid complete ack")
					return
				}
				isComplete = true
				break
			}

			if err != nil {
				log.Printf("Error reading file: %v", err)
				sendWSError(ws, "Error reading file")
				return
			}
			if n == 0 {
				break
			}
		}

		// Calculate duration of download
		// Only delete file if download was completed successfully
		if isComplete {
			if err := os.Remove(filePath); err != nil {
				log.Printf("Failed to remove file: %v", err)
			}

			metrics.RecordTransfer(c.Request.Context(), "download", totalSent, true, "websocket")
		} else {
			metrics.RecordTransfer(c.Request.Context(), "download", totalSent, false, "websocket")
		}
	}
}

func HandleWSUpload(uploadDir string, metrics *telemetry.Provider, limiter *UploadLimiter) gin.HandlerFunc {
	// Limit inbound frame size to one encrypted chunk + GCM tag + small headroom.
	// This prevents a malicious client from forcing gorilla to allocate a huge buffer
	// before the application-level chunk-size validation can fire.
	maxChunkBytes := GlobalConfig.ChunkSize*1024*1024 + 16
	maxFrameBytes := maxChunkBytes + 1024

	// Frames are read into pooled buffers instead of ws.ReadMessage, which
	// allocates and repeatedly regrows a fresh slice for every chunk.
	framePool := sync.Pool{New: func() any {
		b := make([]byte, maxFrameBytes)
		return &b
	}}

	return func(c *gin.Context) {
		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		ws.SetReadLimit(int64(maxFrameBytes))

		// Keepalive: extend read deadline whenever a pong arrives.
		ws.SetReadDeadline(time.Now().Add(pongWait))
		ws.SetPongHandler(func(string) error {
			ws.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})

		ctx, cancelPing := context.WithCancel(context.Background())
		defer cancelPing()
		startPingLoop(ctx, ws)

		// 1. Initial Message: Size Check
		_, msg, err := ws.ReadMessage()
		if err != nil {
			sendWSError(ws, "Failed to read initial message")
			return
		}
		ws.SetReadDeadline(time.Now().Add(pongWait))

		var init struct {
			Type   string `json:"type"`
			Size   int64  `json:"size"`
			FileID string `json:"fileId,omitempty"` // Optional: for passphrase-based uploads
		}
		if err := json.Unmarshal(msg, &init); err != nil {
			sendWSError(ws, "Invalid initial message format")
			return
		}

		if init.Type != "init" {
			sendWSError(ws, "Invalid message type: expected 'init'")
			return
		}

		if init.Size < 0 || init.Size > int64(GlobalConfig.MaxFileSizeBytes) {
			sendWSError(ws, "File too large")
			return
		}

		upload, reason := limiter.Begin(c.ClientIP(), init.Size)
		if upload == nil {
			sendWSError(ws, reason)
			return
		}
		defer upload.Done()

		// 2. Generate or Use Provided ID
		var id string
		if init.FileID != "" {
			// Client provided a custom fileID (passphrase mode)
			if !validateID(init.FileID) {
				sendWSError(ws, "Invalid custom file ID format")
				return
			}

			// Claim the ID atomically. A plain "does it exist" check races with
			// a concurrent upload of the same ID; O_EXCL does not.
			reservePath := filepath.Join(uploadDir, init.FileID+".reserve")
			reserve, err := os.OpenFile(reservePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if errors.Is(err, fs.ErrExist) {
				sendWSError(ws, "Share code already in use, please try again with a different passphrase")
				return
			}
			if err != nil {
				sendWSError(ws, "Failed to check for existing files")
				return
			}
			reserve.Close()
			defer os.Remove(reservePath)

			// Reject if a finished upload already uses this ID
			matches, err := filepath.Glob(filepath.Join(uploadDir, init.FileID+".*"))
			if err != nil {
				sendWSError(ws, "Failed to check for existing files")
				return
			}
			for _, m := range matches {
				if m != reservePath {
					sendWSError(ws, "Share code already in use, please try again with a different passphrase")
					return
				}
			}

			id = init.FileID
		} else {
			// Generate random ID as usual
			id, err = generateID(GlobalConfig.IDSize)
			if err != nil {
				sendWSError(ws, "Failed to generate ID")
				return
			}
		}

		if err := wsWriteJSON(ws, gin.H{"type": "id", "id": id}); err != nil {
			sendWSError(ws, "Failed to send ID")
			return
		}

		// 3. Token Message and Validation
		_, tokenMsg, err := ws.ReadMessage()
		if err != nil {
			sendWSError(ws, "Failed to read token")
			return
		}
		ws.SetReadDeadline(time.Now().Add(pongWait))

		var tokenData struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}

		if err := json.Unmarshal(tokenMsg, &tokenData); err != nil {
			sendWSError(ws, "Invalid token message")
			return
		}

		if tokenData.Type != "token" || !validateToken(tokenData.Token) {
			sendWSError(ws, "Invalid token")
			return
		}

		// Send token accepted
		if err := wsWriteJSON(ws, gin.H{"type": "token_accepted"}); err != nil {
			sendWSError(ws, "Failed to acknowledge token")
			return
		}

		// 4. Create File (with token). O_EXCL guarantees we never write into
		// a file another upload created.
		finalPath := filepath.Join(uploadDir, id+"."+tokenData.Token)
		tmpPath := finalPath + ".tmp"
		file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
		if err != nil {
			sendWSError(ws, "Failed to create file")
			return
		}
		defer file.Close()

		bufp := framePool.Get().(*[]byte)
		defer framePool.Put(bufp)
		buf := *bufp

		// 5. Read and Validate Encrypted Metadata Header
		// Layout: IV(12) | length(4 LE) | AES-GCM ciphertext+tag (length bytes)
		header, err := readFrame(ws, buf)
		if err != nil || len(header) < headerSize {
			wsCleanup(ws, tmpPath, "Invalid header: incorrect size")
			return
		}
		ws.SetReadDeadline(time.Now().Add(pongWait))

		metadataLength := binary.LittleEndian.Uint32(header[12:16])
		if metadataLength > 65535 {
			wsCleanup(ws, tmpPath, "Metadata size too large")
			return
		}
		if metadataLength < 16 || uint64(len(header)) != headerSize+uint64(metadataLength) {
			wsCleanup(ws, tmpPath, "Invalid metadata length in header")
			return
		}

		if _, err := file.Write(header); err != nil {
			wsCleanup(ws, tmpPath, "Failed to write header")
			return
		}

		// Send ready signal after successfully processing the header
		if err := wsWriteJSON(ws, gin.H{"type": "ready"}); err != nil {
			log.Printf("Failed to send ready signal: %v", err)
			wsCleanup(ws, tmpPath, "Failed to send ready signal")
			return
		}

		// 6. Read and Validate IV
		iv, err := readFrame(ws, buf)
		if err != nil || len(iv) != 12 {
			wsCleanup(ws, tmpPath, "Invalid IV: incorrect size")
			return
		}
		ws.SetReadDeadline(time.Now().Add(pongWait))

		if _, err := file.Write(iv); err != nil {
			wsCleanup(ws, tmpPath, "Failed to write IV")
			return
		}

		// 7. Chunk Processing Loop
		var totalBytes int64 = int64(headerSize) + int64(metadataLength) + 12 // header + IV
		if !upload.Add(totalBytes) {
			wsCleanup(ws, tmpPath, "Server storage is full, please try again later")
			return
		}
		for {
			chunk, err := readFrame(ws, buf)
			if err != nil {
				wsCleanup(ws, tmpPath, "Failed to read chunk")
				return
			}
			ws.SetReadDeadline(time.Now().Add(pongWait))

			// End signal (single byte 0)
			if len(chunk) == 1 && chunk[0] == 0 {
				break
			}

			// Validate size
			if len(chunk) > maxChunkBytes {
				wsCleanup(ws, tmpPath, "Chunk size exceeds maximum")
				return
			}
			if len(chunk) < 16 { // must at least contain GCM tag
				wsCleanup(ws, tmpPath, "Chunk size too small")
				return
			}

			chunkSize := int64(len(chunk))
			projectedTotal := totalBytes + chunkSize
			if projectedTotal > int64(GlobalConfig.MaxFileSizeBytes) {
				wsCleanup(ws, tmpPath, "File too large")
				return
			}
			if !upload.Add(chunkSize) {
				wsCleanup(ws, tmpPath, "Server storage is full, please try again later")
				return
			}

			// Persist chunk to disk BEFORE acknowledging.
			// An ACK sent before the write succeeds would make a disk error look like a
			// sudden connection drop to the client, because the server close frame races
			// the in-flight next chunk from the client.
			if _, err := file.Write(chunk); err != nil {
				wsCleanup(ws, tmpPath, "Failed to write chunk")
				return
			}
			totalBytes = projectedTotal

			// ACK only after the chunk is written
			if err := wsWriteJSON(ws, gin.H{"type": "ack", "ack": chunkSize}); err != nil {
				log.Printf("Failed to send acknowledgement: %v", err)
				wsCleanup(ws, tmpPath, "Failed to send acknowledgement")
				return
			}
		}

		// 8. Finalization
		if err := file.Close(); err != nil { // Close before rename
			wsCleanup(ws, tmpPath, "Error closing file")
			return
		}

		if err := os.Rename(tmpPath, finalPath); err != nil {
			os.Remove(tmpPath) // Clean up temp file if rename fails
			sendWSError(ws, "Failed to save file")
			return
		}

		metrics.RecordTransfer(c.Request.Context(), "upload", totalBytes, true, "websocket")
		metrics.RecordUpload(c.Request.Context(), totalBytes, true, "websocket")

		// 10. Send Completion Message
		if err := wsWriteJSON(ws, gin.H{
			"type": "complete",
			"id":   id,
			"size": totalBytes,
		}); err != nil {
			log.Printf("Failed to send complete message: %v", err)
		}
	}
}

// readFrame reads the next WebSocket message into buf and returns the filled
// prefix. The connection's read limit must not exceed len(buf).
func readFrame(ws *websocket.Conn, buf []byte) ([]byte, error) {
	_, r, err := ws.NextReader()
	if err != nil {
		return nil, err
	}
	n, err := io.ReadFull(r, buf)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// sendWSError sends a typed error JSON frame and closes the connection.
func sendWSError(ws *websocket.Conn, message string) {
	log.Printf("WebSocket error: %s", message)
	_ = wsWriteJSON(ws, gin.H{"type": "error", "error": message})
	ws.Close()
}

// wsCleanup sends an error, closes the connection, and removes any partial temp file.
func wsCleanup(ws *websocket.Conn, tmpPath string, message string) {
	sendWSError(ws, message)
	if _, err := os.Stat(tmpPath); err == nil {
		if err := os.Remove(tmpPath); err != nil {
			log.Printf("Failed to remove temporary file: %v", err)
		}
	}
}
