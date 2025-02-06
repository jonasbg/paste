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
	"sync"
	"time"

	"github.com/jonasbg/paste/m/v2/spa"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/jonasbg/paste/m/v2/utils"
)

const (
	maxFileSize = 5 * 1024 * 1024 * 1024 // 5GB

	// Rate limiting constants
	requestsPerSecond = 5
	burstSize         = 20
)

type limiterInfo struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type IPRateLimiter struct {
	ips map[string]*limiterInfo
	mu  *sync.RWMutex
	r   rate.Limit
	b   int
}

// Create a new rate limiter
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		ips: make(map[string]*limiterInfo),
		mu:  &sync.RWMutex{},
		r:   r,
		b:   b,
	}

	// Start a goroutine to clean up old entries
	go i.cleanupLoop()
	return i
}
func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	info, exists := i.ips[ip]
	if !exists {
		info = &limiterInfo{
			limiter:  rate.NewLimiter(i.r, i.b),
			lastSeen: time.Now(),
		}
		i.ips[ip] = info
	} else {
		info.lastSeen = time.Now()
	}

	return info.limiter
}

// Cleanup old IP entries periodically
func (i *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	for range ticker.C {
		i.mu.Lock()
		for ip, info := range i.ips {
			if time.Since(info.lastSeen) > time.Hour {
				delete(i.ips, ip)
			}
		}
		i.mu.Unlock()
	}
}

// Rate limiting middleware
func rateLimitMiddleware(limiter *IPRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		l := limiter.GetLimiter(ip)
		if !l.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Too many requests",
				"retry_after": "1s",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

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

	limiter := NewIPRateLimiter(rate.Limit(requestsPerSecond), burstSize)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	api := r.Group("/api")
	api.Use(rateLimitMiddleware(limiter))
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
		// Check content length
		if c.Request.ContentLength > maxFileSize {
			c.String(http.StatusBadRequest, "File too large")
			return
		}

		// Generate ID first
		id, err := generateID()
		if err != nil {
			c.String(http.StatusInternalServerError, "Error generating ID")
			return
		}

		// Open destination file
		dst := filepath.Join(uploadDir, id)
		out, err := os.Create(dst)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error creating file")
			return
		}
		defer out.Close()

		// Get multipart reader
		reader, err := c.Request.MultipartReader()
		if err != nil {
			c.String(http.StatusBadRequest, "Error reading multipart form")
			return
		}

		// Stream the file
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
