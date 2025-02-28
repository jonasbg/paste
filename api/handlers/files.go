package handlers

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	headerSize      = 16          // Size of metadata header
	maxMetadataSize = 1024 * 1024 // 1MB max metadata size
	expectedIVSize  = 12          // Size of GCM IV
	bufferSize      = 32 * 1024   // 32KB buffer for copying
)

// generateID generates a cryptographically secure random ID with a specified bit length.
// It supports bit lengths of 64, 128, 192, and 256.  If no length is provided, it defaults to 64 bits.
// The generated ID is returned as a hexadecimal string.
//
// Args:
//
//	bitLength (optional): An integer representing the desired bit length of the ID.
//	                      Must be one of 64, 128, 192, or 256. Defaults to 64.
//
// Returns:
//
//	(string, error): The generated ID as a hexadecimal string, or an error if the
//	                specified bit length is invalid or if random byte generation fails.
func generateID(bitLength ...int) (string, error) {
	var length int
	if len(bitLength) > 0 {
		length = bitLength[0]
	} else {
		length = 64 // Default to 64 bits
	}

	switch length {
	case 64, 128, 192, 256:
		byteLength := length / 8
		bytes := make([]byte, byteLength)
		if _, err := rand.Read(bytes); err != nil {
			return "", fmt.Errorf("failed to generate random bytes: %w", err)
		}
		return hex.EncodeToString(bytes), nil
	default:
		return "", fmt.Errorf("invalid ID length: %d.  Must be 128, 192, or 256", length)
	}
}

func HandleMetadata(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if len(id) != 16 && len(id) != 24 && len(id) != 32 {
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

func HandleDelete(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if len(id) != 16 && len(id) != 24 && len(id) != 32 {
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

		// Delete the file
		if err := os.Remove(filePath); err != nil {
			log.Printf("Error: Failed to delete file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
	}
}

func HandleDownload(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if len(id) != 16 && len(id) != 24 && len(id) != 32 {
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
		file, err := os.Stat(filePath)
		if err != nil {
			log.Printf("Error: Failed to get file info: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		c.Header("Content-Type", "application/octet-stream")
		c.Header("Content-Length", strconv.FormatInt(file.Size(), 10))
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		c.File(filePath)

		go func() {
			time.Sleep(30 * time.Minute)
			if _, err := os.Stat(filePath); err == nil {
				if err := os.Remove(filePath); err != nil {
					log.Printf("Failed to remove file: %v", err)
				}
			}
		}()
	}
}

func HandleDelete(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if len(id) != 16 && len(id) != 24 && len(id) != 32 {
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

		// Delete the file
		if err := os.Remove(filePath); err != nil {
			log.Printf("Error: Failed to delete file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
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
