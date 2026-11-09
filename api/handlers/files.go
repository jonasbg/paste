package handlers

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	headerSize      = 16          // Size of metadata header
	maxMetadataSize = 1024 * 1024 // 1MB max metadata size

	// Tokens are base64url HMACs truncated to the key length, so a 128-bit key
	// gives 22 characters and a 256-bit key 43. Accepting the whole range keeps
	// existing files reachable when an operator changes KEY_SIZE.
	minTokenLength = 22
	maxTokenLength = 43
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

// resolveFile validates the file ID and the X-HMAC-Token header and opens the
// matching upload. On failure it writes the error response and returns nil.
func resolveFile(c *gin.Context, uploadDir string) (*os.File, string) {
	id := c.Param("id")
	if !validateID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return nil, ""
	}

	token := c.GetHeader("X-HMAC-Token")
	if !validateToken(token) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid token"})
		return nil, ""
	}

	filePath := filepath.Join(uploadDir, id+"."+token)
	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		} else {
			log.Printf("Error: Failed to open file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
		}
		return nil, ""
	}
	return file, filePath
}

func HandleMetadata(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, _ := resolveFile(c, uploadDir)
		if file == nil {
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
		header := make([]byte, headerSize)
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
		fullMetadata := make([]byte, headerSize+int(metadataLen))
		copy(fullMetadata, header)

		if _, err = io.ReadFull(file, fullMetadata[headerSize:]); err != nil {
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
		file, filePath := resolveFile(c, uploadDir)
		if file == nil {
			return
		}
		file.Close()

		if err := os.Remove(filePath); err != nil {
			log.Printf("Error: Failed to delete file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
	}
}

// HandleDownload streams the stored ciphertext. It does not delete the file:
// the web client fetches it more than once (previews, then the download) and
// deletes it explicitly afterwards via DELETE /api/delete/:id.
func HandleDownload(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, _ := resolveFile(c, uploadDir)
		if file == nil {
			return
		}
		defer file.Close()

		c.Header("Content-Type", "application/octet-stream")
		c.Header("Cache-Control", "no-cache")

		// ServeContent sets Content-Length and supports Range requests.
		http.ServeContent(c.Writer, c.Request, "", time.Time{}, file)
	}
}

func validateToken(token string) bool {
	if len(token) < minTokenLength || len(token) > maxTokenLength {
		return false
	}

	// Ensure token only contains safe filename characters
	for i := 0; i < len(token); i++ {
		ch := token[i]
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// validateID checks that id is a hex file ID of a supported length and is
// therefore safe to use in a file name. Generated IDs are ID_SIZE/4 characters
// (64-256 bit); passphrase IDs are 32. 24 is kept for older clients.
func validateID(id string) bool {
	switch len(id) {
	case 16, 24, 32, 48, 64:
	default:
		return false
	}
	for i := 0; i < len(id); i++ {
		ch := id[i]
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F') {
			return false
		}
	}
	return true
}
