package handlers

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	CheckOrigin: func(r *http.Request) bool {
		return true // TODO: tighten this with origin checks if exposed publicly
	},
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

func HandleWSUpload(uploadDir string, metrics *telemetry.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		// Limit inbound frame size to one encrypted chunk + GCM tag + small headroom.
		// This prevents a malicious client from forcing gorilla to allocate a huge buffer
		// before the application-level chunk-size validation can fire.
		maxFrameBytes := int64(GlobalConfig.ChunkSize)*1024*1024 + 16 + 1024
		ws.SetReadLimit(maxFrameBytes)

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

		if init.Size > int64(GlobalConfig.MaxFileSizeBytes) {
			sendWSError(ws, "File too large")
			return
		}

		// 2. Generate or Use Provided ID
		var id string
		if init.FileID != "" {
			// Client provided a custom fileID (passphrase mode)
			// Validate format
			if len(init.FileID) != 16 && len(init.FileID) != 24 && len(init.FileID) != 32 {
				sendWSError(ws, "Invalid custom file ID length")
				return
			}
			if !validateID(init.FileID) {
				sendWSError(ws, "Invalid custom file ID format")
				return
			}

			// Check for collision: reject if any file with this ID already exists
			matches, err := filepath.Glob(filepath.Join(uploadDir, init.FileID+".*"))
			if err != nil {
				sendWSError(ws, "Failed to check for existing files")
				return
			}
			if len(matches) > 0 {
				sendWSError(ws, "Share code already in use, please try again with a different passphrase")
				return
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

		// 4. Create File (with token)
		finalPath := filepath.Join(uploadDir, id+"."+tokenData.Token)
		tmpPath := finalPath + ".tmp" // Use a temporary file
		// O_EXCL makes creation the atomic claim on this ID: two concurrent
		// passphrase uploads with the same derived fileId+token can both pass the
		// Glob collision check above, but only one can create the temp file.
		file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if errors.Is(err, fs.ErrExist) {
			sendWSError(ws, "Share code already in use, please try again with a different passphrase")
			return
		}
		if err != nil {
			sendWSError(ws, "Failed to create file")
			return
		}
		// 512 KB buffer is enough to smooth out syscall bursts; the OS page cache
		// handles larger sequential writes efficiently without a huge userspace buffer.
		bufWriter := bufio.NewWriterSize(file, 512*1024)
		defer func() {
			bufWriter.Flush()
			file.Close()
		}()

		// 5. Read and Validate Encrypted Metadata Header
		_, header, err := ws.ReadMessage()
		if err != nil || len(header) < headerSize {
			wsCleanup(ws, tmpPath, "Invalid header: incorrect size")
			return
		}
		ws.SetReadDeadline(time.Now().Add(pongWait))

		// Basic structural checks on the header (before we even try processing)
		metadataIV := header[:12] // first 12 is the IV
		metadataLength := binary.LittleEndian.Uint32(header[12:16])
		metadataEncryptedDataIV := header[16:28]

		if len(metadataIV) != 12 {
			wsCleanup(ws, tmpPath, "Invalid metadata IV size")
			return
		}

		if len(metadataEncryptedDataIV) != 12 {
			wsCleanup(ws, tmpPath, "Invalid metadata encrypted data IV size")
			return
		}

		if metadataLength > 65535 { // Example limit: 64KB metadata
			wsCleanup(ws, tmpPath, "Metadata size too large")
			return
		}

		if int(16+metadataLength) > len(header) {
			wsCleanup(ws, tmpPath, "Incomplete metadata in header")
			return
		}

		if _, err := bufWriter.Write(header); err != nil {
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
		_, iv, err := ws.ReadMessage()
		if err != nil || len(iv) != 12 {
			wsCleanup(ws, tmpPath, "Invalid IV: incorrect size")
			return
		}
		ws.SetReadDeadline(time.Now().Add(pongWait))

		if _, err := bufWriter.Write(iv); err != nil {
			wsCleanup(ws, tmpPath, "Failed to write IV")
			return
		}

		// 7. Chunk Processing Loop
		var totalBytes int64 = int64(len(header) + len(iv)) // Initialize with header + IV
		for {
			_, chunk, err := ws.ReadMessage()
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
			if len(chunk) > (GlobalConfig.ChunkSize*1024*1024 + 16) {
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

			// Persist chunk to disk BEFORE acknowledging.
			// An ACK sent before the write succeeds would make a disk error look like a
			// sudden connection drop to the client, because the server close frame races
			// the in-flight next chunk from the client.
			if _, err := bufWriter.Write(chunk); err != nil {
				wsCleanup(ws, tmpPath, "Failed to write chunk")
				return
			}
			totalBytes = projectedTotal

			// ACK only after the chunk is safely written to the buffer
			if err := wsWriteJSON(ws, gin.H{"type": "ack", "ack": chunkSize}); err != nil {
				log.Printf("Failed to send acknowledgement: %v", err)
				wsCleanup(ws, tmpPath, "Failed to send acknowledgement")
				return
			}
		}

		// 8. Finalization
		// Ensure all buffered data is flushed before closing/renaming
		if err := bufWriter.Flush(); err != nil {
			wsCleanup(ws, tmpPath, "Error flushing buffer")
			return
		}
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
