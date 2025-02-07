package handlers

import (
	"encoding/binary"
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
		log.Println("WebSocket upgrade attempt")
		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer ws.Close()
		log.Println("WebSocket connection established")

		// Generate file ID
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

		// Read entire message
		_, data, err := ws.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			os.Remove(tmpPath)
			return
		}

		if len(data) < headerSize {
			log.Printf("Invalid data size: %d", len(data))
			os.Remove(tmpPath)
			ws.WriteJSON(gin.H{"error": "Invalid data size"})
			return
		}

		// Extract and validate header
		header := data[:headerSize]
		metadataLen := binary.LittleEndian.Uint32(header[12:16])
		log.Printf("Expected metadata length: %d", metadataLen)

		if metadataLen > maxMetadataSize {
			os.Remove(tmpPath)
			ws.WriteJSON(gin.H{"error": "Metadata too large"})
			return
		}

		totalBytes := int64(len(data))
		if totalBytes > maxFileSize {
			os.Remove(tmpPath)
			ws.WriteJSON(gin.H{"error": "File too large"})
			return
		}

		// Write file
		if _, err := file.Write(data); err != nil {
			log.Printf("Error writing file: %v", err)
			os.Remove(tmpPath)
			ws.WriteJSON(gin.H{"error": "Failed to write file"})
			return
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
