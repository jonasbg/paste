package handlers

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxFileSize     = 5 * 1024 * 1024 * 1024 // 5GB
	headerSize      = 16                     // Size of metadata header
	maxMetadataSize = 1024 * 1024            // 1MB max metadata size
	expectedIVSize  = 12                     // Size of GCM IV
)

func generateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func HandleMetadata(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if len(id) != 32 {
			fmt.Println("Error: Invalid ID length")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		filePath := filepath.Join(uploadDir, id)

		// Get file info to get size
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("Error: File not found:", id)
				c.JSON(http.StatusNotFound, gin.H{"error": "Resource not found"})
			} else {
				fmt.Println("Error: Failed to get file info:", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			}
			return
		}

		file, err := os.Open(filePath)
		if err != nil {
			fmt.Println("Error: Failed to open file:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}
		defer file.Close()

		header := make([]byte, 16)
		if _, err = io.ReadFull(file, header); err != nil {
			fmt.Println("Error: Failed to read header:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		metadataLen := binary.LittleEndian.Uint32(header[12:16])
		if metadataLen > 1024*1024 {
			fmt.Println("Error: Metadata size exceeds limit")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		fullMetadata := make([]byte, 16+int(metadataLen))
		copy(fullMetadata[:16], header)

		if _, err = io.ReadFull(file, fullMetadata[16:]); err != nil {
			fmt.Println("Error: Failed to read metadata:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		// Get total file size
		fileSize := fileInfo.Size()

		// Add size to response headers
		c.Header("Content-Type", "application/octet-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("X-File-Size", strconv.FormatInt(fileSize, 10))

		c.Writer.Write(fullMetadata)
	}
}

func HandleDownload(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if len(id) != 32 {
			fmt.Println("Error: Invalid ID length")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		filePath := filepath.Join(uploadDir, id)
		fi, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("Error: File not found:", id)
				c.JSON(http.StatusNotFound, gin.H{"error": "Resource not found"})
			} else {
				fmt.Println("Error: Failed to access file:", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			}
			return
		}

		c.Header("Content-Type", "application/octet-stream")
		c.Header("Cache-Control", "no-cache")
		c.File(filePath)

		go func(size int64, fileID string) {
			if err := os.Remove(filePath); err != nil {
				fmt.Println("Error: Failed to remove file:", err)
			}
		}(fi.Size(), id)
	}
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

func HandleUpload(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check content type
		contentType := c.GetHeader("Content-Type")
		if !strings.HasPrefix(contentType, "multipart/form-data") {
			fmt.Println("Error: Invalid request format")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
			return
		}

		// Check content length
		if c.Request.ContentLength > maxFileSize {
			fmt.Println("Error: File size exceeds limit")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
			return
		}

		// Get the file from form
		file, fileHeader, err := c.Request.FormFile("file")
		if err != nil {
			fmt.Println("Error: Missing file in request")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}
		defer file.Close()

		// Verify file size from header
		if fileHeader.Size > maxFileSize {
			fmt.Println("Error: File size exceeds limit")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
			return
		}

		// Read and validate file structure
		header := make([]byte, headerSize)
		if _, err := io.ReadFull(file, header); err != nil {
			fmt.Println("Error: File structure validation failed")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
			return
		}

		metadataLen := binary.LittleEndian.Uint32(header[12:16])
		if metadataLen > maxMetadataSize {
			fmt.Println("Error: Metadata validation failed")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
			return
		}

		metadata := make([]byte, metadataLen)
		if _, err := io.ReadFull(file, metadata); err != nil {
			fmt.Println("Error: Metadata read failed")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
			return
		}

		if !validateWasmEncryption(header, metadata) {
			fmt.Println("Error: File validation failed")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
			return
		}

		// Generate unique ID
		id, err := generateID()
		if err != nil {
			fmt.Println("Error: ID generation failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		// Create destination file
		dst := filepath.Join(uploadDir, id)
		out, err := os.Create(dst)
		if err != nil {
			fmt.Println("Error: File creation failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}
		defer out.Close()

		// Write file data
		if _, err := out.Write(header); err != nil {
			os.Remove(dst)
			fmt.Println("Error: File write failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		if _, err := out.Write(metadata); err != nil {
			os.Remove(dst)
			fmt.Println("Error: File write failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		written, err := io.Copy(out, file)
		if err != nil {
			os.Remove(dst)
			fmt.Println("Error: File write failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		totalSize := written + int64(len(header)) + int64(len(metadata))
		if totalSize > maxFileSize {
			os.Remove(dst)
			fmt.Println("Error: File size exceeds limit")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
			return
		}

		response := gin.H{
			"id":        id,
			"size":      totalSize,
			"timestamp": time.Now().Unix(),
		}
		c.Set("responseData", response)

		c.JSON(http.StatusOK, response)
	}
}
