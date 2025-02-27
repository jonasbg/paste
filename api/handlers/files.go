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

		// Get file info to determine size
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not access file"})
			return
		}
		fileSize := fileInfo.Size()

		// Open the file manually
		file, err := os.Open(filePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not open file"})
			return
		}
		defer file.Close()

		// Set headers
		c.Header("Content-Type", "application/octet-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Content-Length", fmt.Sprintf("%d", fileSize))
		c.Header("Connection", "keep-alive")

		// Use a custom writer that tracks completion
		downloadCompleted := false
		bytesWritten := int64(0)

		// Setup context cancellation monitoring
		ctx := c.Request.Context()
		doneCh := make(chan struct{})
		defer close(doneCh)

		// Monitor for client disconnection
		go func() {
			select {
			case <-ctx.Done():
				// Client disconnected before completion
				log.Printf("Client disconnected during download of file %s", id)
			case <-doneCh:
				// Download completed normally
			}
		}()

		// Stream the file to the client
		buf := make([]byte, 4096)
		for {
			n, err := file.Read(buf)
			if err != nil && err != io.EOF {
				log.Printf("Error reading file: %v", err)
				return
			}
			if n == 0 {
				break
			}

			if _, err := c.Writer.Write(buf[:n]); err != nil {
				log.Printf("Error writing to client: %v", err)
				return
			}

			c.Writer.Flush()
			bytesWritten += int64(n)
		}

		// Check if all bytes were sent
		downloadCompleted = (bytesWritten == fileSize)

		// Only delete the file if download was completed
		if downloadCompleted {
			log.Printf("Download completed successfully for file %s, deleting", id)
			go func() {
				// Small delay to ensure all buffers are flushed
				time.Sleep(1 * time.Second)
				if err := os.Remove(filePath); err != nil {
					log.Printf("Failed to remove file: %v", err)
				}
			}()
		} else {
			log.Printf("Download incomplete for file %s (%d of %d bytes), file preserved",
				id, bytesWritten, fileSize)
		}
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
