package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Norskhelsenett/deling/m/v2/spa"
	"github.com/gin-gonic/gin"

	"github.com/Norskhelsenett/deling/m/v2/utils"
)

var magicNumberStr = "0x4E48464C"

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

		if c.Request.ContentLength > maxFileSize {
			c.String(http.StatusBadRequest, "File too large")
			return
		}

		id, err := generateID()
		if err != nil {
			c.String(http.StatusInternalServerError, "Error generating ID")
			return
		}

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

		// Buffer for reading header
		var buf bytes.Buffer
		tee := io.TeeReader(part, &buf)

		// Read and verify header
		header := make([]byte, 16)
		if _, err := io.ReadFull(tee, header); err != nil {
			c.String(http.StatusBadRequest, "Invalid file format")
			return
		}

		// Verify magic number
		magic := binary.LittleEndian.Uint32(header[0:4])
		parsed, err := strconv.ParseUint(magicNumberStr[2:], 16, 32) // Remove "0x" prefix
		if err != nil {
			panic("Invalid magic number: " + err.Error())
		}
		magicNumber := uint32(parsed)
		if magic != magicNumber {
			c.String(http.StatusBadRequest, "File not properly encrypted")
			return
		}

		// Verify metadata length
		metadataLen := binary.LittleEndian.Uint32(header[12:16])
		if metadataLen == 0 || metadataLen > 1024*1024 {
			c.String(http.StatusBadRequest, "Invalid metadata length")
			return
		}

		// Create and write to destination file
		dst := filepath.Join(uploadDir, id)
		out, err := os.Create(dst)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error creating file")
			return
		}
		defer out.Close()

		// Write the buffered header first
		if _, err := io.Copy(out, &buf); err != nil {
			os.Remove(dst)
			c.String(http.StatusInternalServerError, "Error writing file")
			return
		}

		// Write the rest of the file
		written, err := io.Copy(out, part)
		if err != nil {
			os.Remove(dst)
			c.String(http.StatusInternalServerError, "Error saving file")
			return
		}

		if written > maxFileSize {
			os.Remove(dst)
			c.String(http.StatusBadRequest, "File too large")
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
