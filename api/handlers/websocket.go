package handlers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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

func HandleWSUpload(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		id, err := generateID()
		if err != nil {
			ws.WriteJSON(gin.H{"error": "Failed to generate ID"})
			return
		}

		tmpPath := filepath.Join(uploadDir, id+".tmp")
		finalPath := filepath.Join(uploadDir, id)

		file, err := os.Create(tmpPath)
		if err != nil {
			ws.WriteJSON(gin.H{"error": "Failed to create file"})
			return
		}
		defer file.Close()

		var totalBytes int64

		// Read first message (header)
		_, header, err := ws.ReadMessage()
		if err != nil || len(header) < headerSize {
			os.Remove(tmpPath)
			ws.WriteJSON(gin.H{"error": "Invalid header"})
			return
		}

		if _, err := file.Write(header); err != nil {
			os.Remove(tmpPath)
			ws.WriteJSON(gin.H{"error": "Failed to write header"})
			return
		}
		totalBytes += int64(len(header))

		// Read chunks until end signal
		for {
			_, chunk, err := ws.ReadMessage()
			if err != nil {
				os.Remove(tmpPath)
				ws.WriteJSON(gin.H{"error": "Failed to read chunk"})
				return
			}

			// Check for end signal (single byte 0)
			if len(chunk) == 1 && chunk[0] == 0 {
				break
			}

			totalBytes += int64(len(chunk))
			if totalBytes > maxFileSize {
				os.Remove(tmpPath)
				ws.WriteJSON(gin.H{"error": "File too large"})
				return
			}

			if _, err := file.Write(chunk); err != nil {
				os.Remove(tmpPath)
				ws.WriteJSON(gin.H{"error": "Failed to write chunk"})
				return
			}
		}

		file.Close()

		if err := os.Rename(tmpPath, finalPath); err != nil {
			os.Remove(tmpPath)
			ws.WriteJSON(gin.H{"error": "Failed to save file"})
			return
		}

		ws.WriteJSON(gin.H{
			"id":       id,
			"size":     totalBytes,
			"complete": true,
		})
	}
}
