package handlers

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jonasbg/paste/m/v2/db"
	"github.com/jonasbg/paste/m/v2/types"
	"github.com/jonasbg/paste/m/v2/utils"
)

var upgrader = websocket.Upgrader{
	// Larger buffers reduce syscall overhead for large binary frames (default 1KB -> 64KB)
	ReadBufferSize:  64 * 1024,
	WriteBufferSize: 64 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // TODO: tighten this with origin checks if exposed publicly
	},
}

func HandleWSDownload(uploadDir string, db *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		// Get initial request with fileId and token
		_, msg, err := ws.ReadMessage()
		if err != nil {
			sendError(ws, "Failed to read initial message")
			return
		}

		var request struct {
			Type   string `json:"type"`
			FileId string `json:"fileId"`
			Token  string `json:"token"`
		}

		if err := json.Unmarshal(msg, &request); err != nil {
			sendError(ws, "Invalid message format")
			return
		}

		if request.Type != "download_init" {
			sendError(ws, "Invalid message type: expected 'download_init'")
			return
		}

		// Validate fileId format
		if len(request.FileId) != 16 && len(request.FileId) != 24 && len(request.FileId) != 32 {
			sendError(ws, "Invalid file ID format")
			return
		}

		if !validateID(request.FileId) {
			sendError(ws, "Invalid file ID format")
			return
		}

		// Validate token
		if !validateToken(request.Token) {
			sendError(ws, "Invalid token")
			return
		}

		// Locate and open the file
		filePath := filepath.Join(uploadDir, request.FileId+"."+request.Token)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			sendError(ws, "File not found")
			return
		}

		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Error: Failed to open file: %v", err)
			sendError(ws, "Server error: Cannot open file")
			return
		}
		defer file.Close()

		// Get file info for size
		fileInfo, err := file.Stat()
		if err != nil {
			log.Printf("Error: Failed to get file info: %v", err)
			sendError(ws, "Server error: Cannot get file info")
			return
		}

		// Send file size info
		if err := ws.WriteJSON(gin.H{
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
					var ack struct {
						Type string `json:"type"`
					}
					if err := json.Unmarshal(ackMsg, &ack); err != nil || ack.Type != "ack" {
						log.Printf("Invalid final batch ack: %v", err)
						return
					}
				}

				if err := ws.WriteJSON(gin.H{"type": "complete", "size": totalSent}); err != nil {
					log.Printf("Failed to send complete message: %v", err)
					return
				}
				_, completeMsg, err := ws.ReadMessage()
				if err != nil {
					log.Printf("Error receiving final ack: %v", err)
					return
				}
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
				sendError(ws, "Error reading file")
				return
			}
			if n == 0 {
				break
			}
		}

		// Calculate duration of download
		duration := time.Since(start)

		// Only delete file if download was completed successfully
		if isComplete {
			if err := os.Remove(filePath); err != nil {
				log.Printf("Failed to remove file: %v", err)
			}

			// Log successful transaction
			tx := &types.TransactionLog{
				Timestamp:  start,
				Action:     "download",
				Method:     "websocket",
				IP:         utils.GetRealIP(c),
				UserAgent:  c.Request.UserAgent(),
				FileID:     request.FileId,
				Duration:   duration.Milliseconds(),
				Size:       totalSent,
				Success:    true,
				StatusCode: 200,
			}

			if err = db.LogTransaction(tx); err != nil {
				log.Printf("Failed to create transaction log: %v", err)
			}
		} else {
			// Log incomplete transaction
			tx := &types.TransactionLog{
				Timestamp:  start,
				Action:     "download_incomplete",
				Method:     "websocket",
				IP:         utils.GetRealIP(c),
				UserAgent:  c.Request.UserAgent(),
				FileID:     request.FileId,
				Duration:   duration.Milliseconds(),
				Size:       totalSent,
				Success:    false,
				StatusCode: 500,
			}

			if err = db.LogTransaction(tx); err != nil {
				log.Printf("Failed to create transaction log: %v", err)
			}
		}
	}
}

func HandleWSUpload(uploadDir string, db *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		// 1. Initial Message: Size Check
		_, msg, err := ws.ReadMessage()
		if err != nil {
			sendError(ws, "Failed to read initial message")
			return
		}

		var init struct {
			Type string `json:"type"`
			Size int64  `json:"size"`
		}
		if err := json.Unmarshal(msg, &init); err != nil {
			sendError(ws, "Invalid initial message format")
			return
		}

		if init.Type != "init" {
			sendError(ws, "Invalid message type: expected 'init'")
			return
		}

		if init.Size > int64(GlobalConfig.MaxFileSizeBytes) {
			sendError(ws, "File too large")
			return
		}

		// 2. Generate ID and Send
		id, err := generateID(GlobalConfig.IDSize) // Your existing ID generation
		if err != nil {
			sendError(ws, "Failed to generate ID")
			return
		}

		if err := ws.WriteJSON(gin.H{"id": id}); err != nil {
			sendError(ws, "Failed to send ID")
			return
		}

		// 3. Token Message and Validation
		_, tokenMsg, err := ws.ReadMessage()
		if err != nil {
			sendError(ws, "Failed to read token")
			return
		}

		var tokenData struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}

		if err := json.Unmarshal(tokenMsg, &tokenData); err != nil {
			sendError(ws, "Invalid token message")
			return
		}

		if tokenData.Type != "token" || !validateToken(tokenData.Token) {
			sendError(ws, "Invalid token")
			return
		}

		// Send token accepted
		if err := ws.WriteJSON(gin.H{"token_accepted": true}); err != nil {
			sendError(ws, "Failed to acknowledge token")
			return
		}

		// 4. Create File (with token)
		finalPath := filepath.Join(uploadDir, id+"."+tokenData.Token)
		tmpPath := finalPath + ".tmp" // Use a temporary file
		file, err := os.Create(tmpPath)
		if err != nil {
			sendError(ws, "Failed to create file")
			return
		}
		// Buffered writer to minimize syscalls; buffer ~2 chunks
		bufWriter := bufio.NewWriterSize(file, (GlobalConfig.ChunkSize*1024*1024+16)*2)
		defer func() {
			bufWriter.Flush()
			file.Close()
		}()

		// 5. Read and Validate Encrypted Metadata Header
		_, header, err := ws.ReadMessage()
		if err != nil || len(header) < headerSize {
			cleanup(ws, tmpPath, "Invalid header: incorrect size")
			return
		}

		// Basic structural checks on the header (before we even try processing)
		metadataIV := header[:12] // first 12 is the IV
		metadataLength := binary.LittleEndian.Uint32(header[12:16])
		metadataEncryptedDataIV := header[16:28]

		if len(metadataIV) != 12 {
			cleanup(ws, tmpPath, "Invalid metadata IV size") //Check Metadata IV size
			return
		}

		if len(metadataEncryptedDataIV) != 12 {
			cleanup(ws, tmpPath, "Invalid metadata encrypted data IV size") //Check Metadata IV size
			return
		}

		if metadataLength > 65535 { // Example limit: 64KB metadata
			cleanup(ws, tmpPath, "Metadata size too large")
			return
		}

		if int(16+metadataLength) > len(header) {
			cleanup(ws, tmpPath, "Incomplete metadata in header")
			return
		}

		if _, err := bufWriter.Write(header); err != nil {
			cleanup(ws, tmpPath, "Failed to write header")
			return
		}

		// Send ready signal after successfully processing the header
		if err := ws.WriteJSON(gin.H{"ready": true}); err != nil {
			log.Printf("Failed to send ready signal: %v", err)
			cleanup(ws, tmpPath, "Failed to send ready signal")
			return
		}

		// 6. Read and Validate IV
		_, iv, err := ws.ReadMessage()
		if err != nil || len(iv) != 12 {
			cleanup(ws, tmpPath, "Invalid IV: incorrect size")
			return
		}

		if _, err := bufWriter.Write(iv); err != nil {
			cleanup(ws, tmpPath, "Failed to write IV")
			return
		}

		// 7.  Chunk Processing Loop (Key Changes)
		var totalBytes int64 = int64(len(header) + len(iv)) // Initialize with header + IV
		for {
			_, chunk, err := ws.ReadMessage()
			if err != nil {
				cleanup(ws, tmpPath, "Failed to read chunk")
				return
			}

			// End signal (single byte 0)
			if len(chunk) == 1 && chunk[0] == 0 {
				break
			}

			// Validate size
			if len(chunk) > (GlobalConfig.ChunkSize*1024*1024 + 16) {
				cleanup(ws, tmpPath, "Chunk size exceeds maximum")
				return
			}
			if len(chunk) < 16 { // must at least contain GCM tag
				cleanup(ws, tmpPath, "Chunk size too small")
				return
			}

			chunkSize := int64(len(chunk))
			projectedTotal := totalBytes + chunkSize
			if projectedTotal > int64(GlobalConfig.MaxFileSizeBytes) {
				cleanup(ws, tmpPath, "File too large")
				return
			}

			// EARLY ACK: send acknowledgement BEFORE disk write to let client pipeline faster
			if err := ws.WriteJSON(gin.H{"ack": chunkSize}); err != nil {
				log.Printf("Failed to send acknowledgement: %v", err)
				cleanup(ws, tmpPath, "Failed to send acknowledgement")
				return
			}

			// Now persist chunk
			if _, err := bufWriter.Write(chunk); err != nil {
				cleanup(ws, tmpPath, "Failed to write chunk")
				return
			}
			totalBytes = projectedTotal
		}

		// 8. Finalization
		// Ensure all buffered data is flushed before closing/renaming
		if err := bufWriter.Flush(); err != nil {
			cleanup(ws, tmpPath, "Error flushing buffer")
			return
		}
		if err := file.Close(); err != nil { // Close before rename
			cleanup(ws, tmpPath, "Error closing file")
			return
		}

		if err := os.Rename(tmpPath, finalPath); err != nil {
			os.Remove(tmpPath) // Clean up temp file if rename fails
			sendError(ws, "Failed to save file")
			return
		}

		duration := time.Since(start)

		// 9. Log Transaction (Your existing code)
		tx := &types.TransactionLog{
			Timestamp:  start,
			Action:     "upload",
			Method:     "websocket",
			IP:         utils.GetRealIP(c),
			UserAgent:  c.Request.UserAgent(),
			FileID:     id,
			Duration:   duration.Milliseconds(),
			Size:       totalBytes,
			Success:    true,
			StatusCode: 200,
		}

		if err = db.LogTransaction(tx); err != nil {
			log.Printf("Failed to create transaction log: %v", err)
		}

		// 10.  Send Completion Message (Your existing code)
		if err := ws.WriteJSON(gin.H{
			"id":       id,
			"size":     totalBytes,
			"complete": true,
		}); err != nil {
			log.Printf("Failed to send complete message: %v", err)
		}
	}
}

// Helper functions (Modified for consistency)

func sendError(ws *websocket.Conn, message string) {
	log.Printf("Sending error: %s", message)     // Log the error
	err := ws.WriteJSON(gin.H{"error": message}) //Consistent JSON error
	if err != nil {
		log.Printf("Failed to send error message: %v", err)
	}
	ws.Close() // Always close the connection on error
}

func cleanup(ws *websocket.Conn, tmpPath string, message string) {
	sendError(ws, message)                      // Send the error message first
	if _, err := os.Stat(tmpPath); err == nil { // Check if file exists
		if err := os.Remove(tmpPath); err != nil {
			log.Printf("Failed to remove temporary file: %v", err)
		}
	}
}
