package handlers

import (
	"encoding/json"
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
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Adjust based on your security needs
	},
}

type FileUploadInit struct {
	FileID string `json:"fileId"`
	Token  string `json:"token"`
	Type   string `json:"type"`
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

		// Read initial message
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
			sendError(ws, "Invalid message type")
			return
		}

		if init.Size > maxFileSize {
			sendError(ws, "File too large")
			return
		}

		// Generate and send file ID
		id, err := generateID()
		if err != nil {
			sendError(ws, "Failed to generate ID")
			return
		}

		if err := ws.WriteJSON(gin.H{"id": id}); err != nil {
			sendError(ws, "Failed to send ID")
			return
		}

		// Read token message
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

		// Create file with token in name
		finalPath := filepath.Join(uploadDir, id+"."+tokenData.Token)
		tmpPath := finalPath + ".tmp"

		file, err := os.Create(tmpPath)
		if err != nil {
			sendError(ws, "Failed to create file")
			return
		}
		defer file.Close()

		var totalBytes int64

		// Read encrypted metadata header
		_, header, err := ws.ReadMessage()
		if err != nil || len(header) < headerSize {
			cleanup(ws, tmpPath, "Invalid header")
			return
		}

		if _, err := file.Write(header); err != nil {
			cleanup(ws, tmpPath, "Failed to write header")
			return
		}
		totalBytes += int64(len(header))

		// Send ready signal after successfully processing the header
		if err := ws.WriteJSON(gin.H{"ready": true}); err != nil {
			log.Printf("Failed to send ready signal: %v", err)
			cleanup(ws, tmpPath, "Failed to send ready signal")
			return
		}

		// Read first message after ready (should be IV)
		_, iv, err := ws.ReadMessage()
		if err != nil || len(iv) != 12 { // GCM IV size
			cleanup(ws, tmpPath, "Invalid IV")
			return
		}

		if _, err := file.Write(iv); err != nil {
			cleanup(ws, tmpPath, "Failed to write IV")
			return
		}
		totalBytes += int64(len(iv))

		// Read chunks until end signal
		for {
			_, chunk, err := ws.ReadMessage()
			if err != nil {
				cleanup(ws, tmpPath, "Failed to read chunk")
				return
			}

			// Check for end signal (single byte 0)
			if len(chunk) == 1 && chunk[0] == 0 {
				break
			}

			if _, err := file.Write(chunk); err != nil {
				cleanup(ws, tmpPath, "Failed to write chunk")
				return
			}

			chunkSize := int64(len(chunk))
			totalBytes += chunkSize

			if totalBytes > maxFileSize {
				cleanup(ws, tmpPath, "File too large")
				return
			}

			// Send acknowledgement
			if err := ws.WriteJSON(gin.H{"ack": chunkSize}); err != nil {
				log.Printf("Failed to send acknowledgement: %v", err)
				cleanup(ws, tmpPath, "Failed to send acknowledgement")
				return
			}
		}

		// Close file before rename
		if err := file.Close(); err != nil {
			cleanup(ws, tmpPath, "Error closing file")
			return
		}

		if err := os.Rename(tmpPath, finalPath); err != nil {
			os.Remove(tmpPath)
			sendError(ws, "Failed to save file")
			return
		}

		duration := time.Since(start)

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

		if err := ws.WriteJSON(gin.H{
			"id":       id,
			"size":     totalBytes,
			"complete": true,
		}); err != nil {
			log.Printf("Failed to send complete message: %v", err)
		}
	}
}

// Helper functions for error handling and cleanup

func sendError(ws *websocket.Conn, message string) {
	err := ws.WriteJSON(gin.H{"error": message}) //Consistent JSON error
	if err != nil {
		log.Printf("Failed to send error message: %v", err)
	}
	ws.Close() // Always close the connection on error
}

func cleanup(ws *websocket.Conn, tmpPath string, message string) {
	sendError(ws, message)                      // Send the error message first
	if _, err := os.Stat(tmpPath); err == nil { // Check if file exists before removing
		if err := os.Remove(tmpPath); err != nil {
			log.Printf("Failed to remove temporary file: %v", err)
		}
	}
}
