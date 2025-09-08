package middleware

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// cachedFile represents a file cached in memory
type cachedFile struct {
	data        []byte
	modTime     time.Time
	contentType string
	etag        string
	size        int64
}

// simple in-memory cache (no eviction yet)
var (
	spaCacheMu sync.RWMutex
	spaCache   = make(map[string]*cachedFile)
)

// getCachedFile retrieves a cached file or loads it from disk
func getCachedFile(root, relPath string) (*cachedFile, error) {
	key := root + "::" + relPath
	spaCacheMu.RLock()
	cf, ok := spaCache[key]
	spaCacheMu.RUnlock()
	if ok {
		return cf, nil
	}

	fullPath := filepath.Join(root, relPath)
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// Read entire file
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	// Derive content type
	ext := filepath.Ext(relPath)
	ctype := "application/octet-stream"
	if ext != "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			ctype = mt
		}
	}
	// Fallback sniffing (only if still default and size>0)
	if ctype == "application/octet-stream" && len(data) > 0 {
		if detected := http.DetectContentType(data); detected != "" {
			ctype = detected
		}
	}
	// ETag from size + modTime + sha1(data[:512])
	h := sha1.New()
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	h.Write([]byte(fi.ModTime().UTC().Format(http.TimeFormat)))
	h.Write([]byte("-"))
	h.Write([]byte(fullPath))
	h.Write(sample)
	etag := "W/\"" + hex.EncodeToString(h.Sum(nil)) + "\""

	cf = &cachedFile{
		data:        data,
		modTime:     fi.ModTime().UTC(),
		contentType: ctype,
		etag:        etag,
		size:        fi.Size(),
	}
	spaCacheMu.Lock()
	// Double-check if another goroutine inserted meanwhile
	if existing, exists := spaCache[key]; exists {
		spaCacheMu.Unlock()
		return existing, nil
	}
	spaCache[key] = cf
	spaCacheMu.Unlock()
	return cf, nil
}

func Middleware(urlPrefix, spaDirectory string) gin.HandlerFunc {
	fileserver := http.FileServer(http.Dir(spaDirectory))
	if urlPrefix != "" {
		fileserver = http.StripPrefix(urlPrefix, fileserver)
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		rel := strings.TrimPrefix(path, "/")

		// Try to serve a static file (not directory) from cache/disk first
		if rel != "" {
			if _, err := filepath.Rel(urlPrefix, path); err == nil {
				if cf, err := getCachedFile(spaDirectory, rel); err == nil {
					// Conditional GET handling
					if match := c.GetHeader("If-None-Match"); match != "" && match == cf.etag {
						c.Status(http.StatusNotModified)
						c.Header("ETag", cf.etag)
						c.Header("Cache-Control", "public, max-age=300")
						c.Header("Last-Modified", cf.modTime.Format(http.TimeFormat))
						c.Abort()
						return
					}
					if ims := c.GetHeader("If-Modified-Since"); ims != "" {
						if t, err := time.Parse(http.TimeFormat, ims); err == nil {
							if !cf.modTime.After(t) { // not modified
								c.Status(http.StatusNotModified)
								c.Header("ETag", cf.etag)
								c.Header("Cache-Control", "public, max-age=300")
								c.Header("Last-Modified", cf.modTime.Format(http.TimeFormat))
								c.Abort()
								return
							}
						}
					}

					// Write headers and body
					c.Status(http.StatusOK)
					c.Header("Content-Type", cf.contentType)
					c.Header("Content-Length", strconv.FormatInt(cf.size, 10))
					c.Header("ETag", cf.etag)
					c.Header("Last-Modified", cf.modTime.Format(http.TimeFormat))
					// Cache HTML shorter, assets longer
					if strings.HasSuffix(rel, ".html") || rel == "index.html" {
						c.Header("Cache-Control", "public, max-age=60")
					} else {
						c.Header("Cache-Control", "public, max-age=300")
					}
					c.Writer.Write(cf.data)
					c.Abort()
					return
				}
			}
		}

		// Not a cached static file; serve index.html (SPA fallback) via underlying file server
		c.Request.URL.Path = "/"
		c.Header("Content-Type", "text/html")
		fileserver.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}
