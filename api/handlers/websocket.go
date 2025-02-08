package handlers

import (
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

type FileUpload struct {
	ID       string
	Size     int64
	FilePath string
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

		id, err := generateID()
		if err != nil {
			sendError(ws, "Failed to generate ID")
			return
		}

		tmpPath := filepath.Join(uploadDir, id+".tmp")
		finalPath := filepath.Join(uploadDir, id)

		file, err := os.Create(tmpPath)
		if err != nil {
			sendError(ws, "Failed to create file")
			return
		}
		defer file.Close() // Ensure file is closed even in error cases

		var totalBytes int64

		// Read first message (header)
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

        // Send "ready" signal after successfully processing the header
        if err := ws.WriteJSON(gin.H{"ready": true}); err != nil {
            log.Printf("Failed to send ready signal: %v", err)
			cleanup(ws, tmpPath, "Failed to send ready signal") //cleanup
            return
        }

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

			chunkSize := int64(len(chunk)) // Calculate chunk size *before* adding to totalBytes
			totalBytes += chunkSize

			if totalBytes > maxFileSize {
				cleanup(ws, tmpPath, "File too large")
				return
			}

			// Send acknowledgement
			if err := ws.WriteJSON(gin.H{"ack": chunkSize}); err != nil {
				log.Printf("Failed to send acknowledgement: %v", err)
				cleanup(ws, tmpPath, "Failed to send acknowledgement") //cleanup
				return
			}
		}


		// No need for file.Sync() here - close handles flushing
		if err := file.Close(); err != nil { // Explicitly close before rename
          cleanup(ws, tmpPath, "Error closing file") //cleanup
          return
        }

		if err := os.Rename(tmpPath, finalPath); err != nil {
			os.Remove(tmpPath) // Attempt cleanup, even if rename fails
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

		if err := ws.WriteJSON(gin.H{ // Use ws.WriteJSON for consistency
			"id":       id,
			"size":     totalBytes,
			"complete": true,
		}); err != nil {
            log.Printf("Failed to send complete message: %v", err) // Log error, but don't return - it's the last message
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
	sendError(ws, message) // Send the error message first
	if _, err := os.Stat(tmpPath); err == nil { // Check if file exists before removing
		if err := os.Remove(tmpPath); err != nil {
			log.Printf("Failed to remove temporary file: %v", err)
		}
	}
}
