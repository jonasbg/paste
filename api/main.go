package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Norskhelsenett/deling/m/v2/spa"
	"github.com/gin-gonic/gin"

	"github.com/Norskhelsenett/deling/m/v2/utils"
)

const (
	maxFileSize = 5 * 1024 * 1024 * 1024 // 5GB
)

func getUploadDir() string {
	if dir := os.Getenv("UPLOAD_DIR"); dir != "" {
		return dir
	}
	return "./uploads"
}

func generateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func main() {
	uploadDir := getUploadDir()
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	api := r.Group("/api")
	{
		// API routes
		api.POST("/upload", handleUpload(uploadDir))
		api.GET("/download/:id", handleDownload(uploadDir))
		api.GET("/metadata/:id", handleMetadata(uploadDir))
	}
	spaDirectory := utils.GetEnv("WEB_DIR", "../web")

	spaDirectory = filepath.Clean(spaDirectory)

	// Ensure static directory exists
	if _, err := os.Stat(spaDirectory); os.IsNotExist(err) {
		log.Fatalf("Static files directory does not exist: %s", spaDirectory)
	}

	r.Use(spa.Middleware("/", spaDirectory))

	log.Printf("Starting server on :8080 with upload directory: %s", uploadDir)
	log.Fatal(r.Run(":8080"))
}

func handleMetadata(uploadDir string) gin.HandlerFunc {
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

func handleUpload(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.String(http.StatusBadRequest, "Error retrieving file")
			return
		}

		if file.Size > maxFileSize {
			c.String(http.StatusBadRequest, "File too large")
			return
		}

		id, err := generateID()
		if err != nil {
			c.String(http.StatusInternalServerError, "Error generating ID")
			return
		}

		dst := filepath.Join(uploadDir, id)
		if err := c.SaveUploadedFile(file, dst); err != nil {
			c.String(http.StatusInternalServerError, "Error saving file")
			return
		}

		c.JSON(http.StatusOK, gin.H{"id": id})
	}
}

func handleDownload(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if len(id) != 32 {
			c.String(http.StatusBadRequest, "Invalid file ID")
			return
		}

		filePath := filepath.Join(uploadDir, id)
		if _, err := os.Stat(filePath); err != nil {
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

		// Delete after sending
		go func() {
			os.Remove(filePath)
		}()
	}
}
