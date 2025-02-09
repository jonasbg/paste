package handlers

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	maxFileSize     = 5 * 1024 * 1024 * 1024 // 5GB
	headerSize      = 16                     // Size of metadata header
	maxMetadataSize = 1024 * 1024            // 1MB max metadata size
	expectedIVSize  = 12                     // Size of GCM IV
	bufferSize      = 32 * 1024              // 32KB buffer for copying
)

func generateID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func HandleMetadata(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if len(id) != 32 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		token := c.GetHeader("X-HMAC-Token")
		if !validateToken(token) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid token"})
			return
		}

		// Look for file with token in name
		filePath := filepath.Join(uploadDir, id+"."+token)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Error: Failed to open file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}
		defer file.Close()

		// Get file info for size
		fileInfo, err := file.Stat()
		if err != nil {
			log.Printf("Error: Failed to get file info: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		// Read header to get metadata length
		header := make([]byte, 16)
		if _, err = io.ReadFull(file, header); err != nil {
			log.Printf("Error: Failed to read header: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		metadataLen := binary.LittleEndian.Uint32(header[12:16])
		if metadataLen > maxMetadataSize {
			log.Printf("Error: Metadata size exceeds limit")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		// Read full metadata including header
		fullMetadata := make([]byte, 16+int(metadataLen))
		copy(fullMetadata[:16], header)

		if _, err = io.ReadFull(file, fullMetadata[16:]); err != nil {
			log.Printf("Error: Failed to read metadata: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		// Add size to response headers
		c.Header("Content-Type", "application/octet-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("X-File-Size", strconv.FormatInt(fileInfo.Size(), 10))

		c.Writer.Write(fullMetadata)
	}
}

func HandleDownload(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if len(id) != 32 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		token := c.GetHeader("X-HMAC-Token")
		if !validateToken(token) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid token"})
			return
		}

		// Look for file with token
		filePath := filepath.Join(uploadDir, id+"."+token)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		// Serve file and delete after download
		c.Header("Content-Type", "application/octet-stream")
		c.Header("Cache-Control", "no-cache")
		c.File(filePath)

		go func() {
			if err := os.Remove(filePath); err != nil {
				log.Printf("Failed to remove file: %v", err)
			}
		}()
	}
}

func validateToken(token string) bool {
	// Ensure token only contains safe filename characters
	safeChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	for _, char := range token {
		if !strings.ContainsRune(safeChars, char) {
			return false
		}
	}
	return true
}

func validateWasmEncryption(header []byte, encryptedMetadata []byte) bool {
	// Check header size
	if len(header) != headerSize {
		return false
	}

	// Validate that first 12 bytes are a valid IV (non-zero)
	iv := header[:expectedIVSize]
	isZero := true
	for _, b := range iv {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		return false
	}

	// Check metadata length from header matches actual metadata
	metadataLen := binary.LittleEndian.Uint32(header[12:16])
	if metadataLen == 0 || metadataLen > maxMetadataSize {
		return false
	}

	// Verify metadata length matches what's in the header
	if uint32(len(encryptedMetadata)) != metadataLen {
		return false
	}

	// Verify metadata has minimum size for encrypted data
	// GCM adds 16 bytes of auth tag to encrypted data
	if len(encryptedMetadata) < 16 {
		return false
	}

	return true
}
