package middleware

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// CachedFile stores the content and metadata of a file
type CachedFile struct {
	Content     []byte
	ContentType string
	ModTime     time.Time
}

// FileCache is an in-memory cache for static files
type FileCache struct {
	files map[string]*CachedFile
	mutex sync.RWMutex
}

// NewFileCache creates a new file cache
func NewFileCache() *FileCache {
	return &FileCache{
		files: make(map[string]*CachedFile),
	}
}

func Middleware(urlPrefix, spaDirectory string) gin.HandlerFunc {
	fileserver := http.FileServer(http.Dir(spaDirectory))
	if urlPrefix != "" {
		fileserver = http.StripPrefix(urlPrefix, fileserver)
	}

	fileCache := NewFileCache()

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Serve the requested file if it exists
		if _, err := filepath.Rel(urlPrefix, path); err == nil {
			filePath := strings.TrimPrefix(path, "/")

			// Check if file exists in cache
			fileCache.mutex.RLock()
			cachedFile, exists := fileCache.files[filePath]
			fileCache.mutex.RUnlock()

			if exists {
				// Serve from cache
				c.Header("Content-Type", cachedFile.ContentType)
				c.Header("Last-Modified", cachedFile.ModTime.Format(http.TimeFormat))
				c.Data(http.StatusOK, cachedFile.ContentType, cachedFile.Content)
				c.Abort()
				return
			}

			// Check if file exists on disk
			file, err := http.Dir(spaDirectory).Open(filePath)
			if err == nil {
				defer file.Close()

				// Get file info for modification time
				fileInfo, err := file.Stat()
				if err == nil && !fileInfo.IsDir() {
					// Read file content
					content, err := os.ReadFile(filepath.Join(spaDirectory, filePath))
					if err == nil {
						// Determine content type
						ext := filepath.Ext(path)
						contentType := "application/octet-stream"
						if ext != "" {
							if mimeType := mime.TypeByExtension(ext); mimeType != "" {
								contentType = mimeType
							}
						}

						// Store in cache
						newCachedFile := &CachedFile{
							Content:     content,
							ContentType: contentType,
							ModTime:     fileInfo.ModTime(),
						}

						fileCache.mutex.Lock()
						fileCache.files[filePath] = newCachedFile
						fileCache.mutex.Unlock()

						// Serve the file
						c.Header("Content-Type", contentType)
						c.Header("Last-Modified", fileInfo.ModTime().Format(http.TimeFormat))
						c.Data(http.StatusOK, contentType, content)
						c.Abort()
						return
					}
				}
			}
		}

		// If the file doesn't exist, serve the index.html
		indexPath := "index.html"

		// Check if index.html exists in cache
		fileCache.mutex.RLock()
		cachedIndex, exists := fileCache.files[indexPath]
		fileCache.mutex.RUnlock()

		if exists {
			// Serve index.html from cache
			c.Header("Content-Type", cachedIndex.ContentType)
			c.Header("Last-Modified", cachedIndex.ModTime.Format(http.TimeFormat))
			c.Data(http.StatusOK, cachedIndex.ContentType, cachedIndex.Content)
			c.Abort()
			return
		}

		// Fallback to standard file server
		c.Request.URL.Path = "/"
		c.Header("Content-Type", "text/html")
		fileserver.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}
