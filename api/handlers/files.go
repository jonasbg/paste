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

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
)

const maxFileSize = 5 * 1024 * 1024 * 1024 // 5GB

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
			c.String(http.StatusBadRequest, "Invalid file ID")
			return
		}

		file, err := os.Open(filepath.Join(uploadDir, id))
		if err != nil {
			if os.IsNotExist(err) {
				c.String(http.StatusNotFound, "File not found")
			} else {
				c.String(http.StatusInternalServerError, "Error opening file")
			}
			return
		}
		defer file.Close()

		header := make([]byte, 16)
		if _, err = io.ReadFull(file, header); err != nil {
			c.String(http.StatusInternalServerError, "Error reading file header")
			return
		}

		metadataLen := binary.LittleEndian.Uint32(header[12:16])
		if metadataLen > 1024*1024 {
			c.String(http.StatusInternalServerError, "Invalid metadata length")
			return
		}

		fullMetadata := make([]byte, 16+int(metadataLen))
		copy(fullMetadata[:16], header)

		if _, err = io.ReadFull(file, fullMetadata[16:]); err != nil {
			c.String(http.StatusInternalServerError, "Error reading metadata")
			return
		}

		c.Header("Content-Type", "application/octet-stream")
		c.Header("Cache-Control", "no-cache")
		c.Writer.Write(fullMetadata)
	}
}

func HandleDownload(uploadDir string, db *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if len(id) != 32 {
			c.String(http.StatusBadRequest, "Invalid file ID")
			return
		}

		filePath := filepath.Join(uploadDir, id)
		fi, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				c.String(http.StatusNotFound, "File not found")
			} else {
				c.String(http.StatusInternalServerError, "Error accessing file")
			}
			return
		}

		c.Header("Content-Type", "application/octet-stream")
		c.Header("Cache-Control", "no-cache")
		c.File(filePath)

		go func(size int64, fileID string) {

			if err := os.Remove(filePath); err != nil {
				log.Printf("Error removing file: %v", err)
			}

		}(fi.Size(), id)
	}
}

func HandleUpload(uploadDir string, db *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxFileSize {
			c.String(http.StatusBadRequest, "File too large")
			return
		}

		id, err := generateID()
		if err != nil {
			c.String(http.StatusInternalServerError, "Error generating ID")
			return
		}

		dst := filepath.Join(uploadDir, id)
		out, err := os.Create(dst)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error creating file")
			return
		}
		defer out.Close()

		reader, err := c.Request.MultipartReader()
		if err != nil {
			c.String(http.StatusBadRequest, "Error reading multipart form")
			return
		}

		part, err := reader.NextPart()
		if err != nil {
			c.String(http.StatusBadRequest, "Error reading file part")
			return
		}
		if part.FormName() != "file" {
			c.String(http.StatusBadRequest, "Invalid form field")
			return
		}

		written, err := io.Copy(out, part)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error saving file")
			return
		}
		if written > maxFileSize {
			os.Remove(dst)
			c.String(http.StatusBadRequest, "File too large")
			return
		}

		// Store response data in context for the logger
		response := gin.H{"id": id}
		c.Set("responseData", response)
		c.JSON(http.StatusOK, response)
	}
}
