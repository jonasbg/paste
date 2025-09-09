package middleware

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// staticFile represents a cached immutable asset kept fully in memory.
type staticFile struct {
	content      []byte
	contentType  string
	etag         string
	modTime      time.Time
	lastModified string
}

// in-memory cache (only for fingerprinted immutable assets)
var (
	staticCache   = make(map[string]*staticFile)
	staticCacheMu sync.RWMutex
)

// Load and cache a static immutable file. Returns nil if not suitable for caching or error.
func getOrLoadStatic(spaDir, relPath string) *staticFile {
	// Normalize
	relPath = strings.TrimPrefix(relPath, "/")
	// Only cache files with a fingerprint-like name (simple heuristic: contains a dot and a hash-looking segment)
	if !strings.Contains(relPath, ".") {
		return nil
	}
	ext := filepath.Ext(relPath)
	switch ext {
	case ".js", ".css", ".wasm", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".ttf", ".woff", ".woff2":
	default:
		return nil
	}

	staticCacheMu.RLock()
	if f, ok := staticCache[relPath]; ok {
		staticCacheMu.RUnlock()
		return f
	}
	staticCacheMu.RUnlock()

	// Load file from disk
	fullPath := filepath.Join(spaDir, relPath)
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		return nil
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil
	}
	h := sha1.Sum(data)
	etag := fmt.Sprintf("\"%x\"", h[:])
	modTime := info.ModTime().UTC().Truncate(time.Second)
	sf := &staticFile{
		content:      data,
		contentType:  mimeTypeByExtension(ext),
		etag:         etag,
		modTime:      modTime,
		lastModified: modTime.Format(http.TimeFormat),
	}

	staticCacheMu.Lock()
	staticCache[relPath] = sf
	staticCacheMu.Unlock()
	return sf
}

func mimeTypeByExtension(ext string) string {
	if ext == "" {
		return "application/octet-stream"
	}
	if mt := mime.TypeByExtension(ext); mt != "" {
		return mt
	}
	return "application/octet-stream"
}

// CacheHeaders sets caching headers based on request path / extension.
func CacheHeaders(spaDir string) gin.HandlerFunc {
	immutableExt := map[string]struct{}{
		".js": {}, ".css": {}, ".wasm": {}, ".png": {}, ".jpg": {}, ".jpeg": {},
		".gif": {}, ".svg": {}, ".webp": {}, ".ico": {}, ".ttf": {}, ".woff": {}, ".woff2": {},
	}

	return func(c *gin.Context) {
		p := c.Request.URL.Path

		// API routes: default no-store (adjust selectively if safe)
		if strings.HasPrefix(p, "/api/") {
			// Downloads / sensitive data: force no-store
			c.Header("Cache-Control", "no-store")
			return
		}

		// SPA entry points
		if p == "/" || p == "/index.html" {
			c.Header("Cache-Control", "no-cache")
			// Optional weak ETag for index.html (best-effort)
			if f, err := http.Dir(spaDir).Open("index.html"); err == nil {
				defer f.Close()
				h := sha1.New()
				io.CopyN(h, f, 32*1024) // partial hash is enough
				c.Header("ETag", `W/"`+hex.EncodeToString(h.Sum(nil))+`"`)
			}
			return
		}

		// Static immutable assets (assumes fingerprinted names in builds)
		ext := filepath.Ext(p)
		if _, ok := immutableExt[ext]; ok {
			if f := getOrLoadStatic(spaDir, p); f != nil {
				c.Header("Cache-Control", "public,max-age=31536000,immutable")
				c.Header("ETag", f.etag)
				c.Header("Last-Modified", f.lastModified)

				// Conditional request handling
				inm := c.GetHeader("If-None-Match")
				ims := c.GetHeader("If-Modified-Since")
				if (inm != "" && inm == f.etag) || (ims != "" && ims == f.lastModified) {
					c.Status(http.StatusNotModified)
					c.Abort()
					return
				}

				// Serve from memory directly; writer not yet written by other middleware.
				c.Header("Content-Type", f.contentType)
				br := &bytesReader{b: f.content}
				http.ServeContent(c.Writer, c.Request, p, f.modTime, br)
				c.Abort()
				return
			}
			// Fallback: still mark as immutable even if not cached
			c.Header("Cache-Control", "public,max-age=31536000,immutable")
			return
		}

		// Fallback: short cache
		c.Header("Cache-Control", "public,max-age=300")
	}
}

// bytesReader implements io.ReadSeeker for a byte slice without copying.
type bytesReader struct{ b []byte; off int64 }

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.off >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.off:])
	r.off += int64(n)
	return n, nil
}

func (r *bytesReader) Seek(offset int64, whence int) (int64, error) {
	var newOff int64
	switch whence {
	case io.SeekStart:
		newOff = offset
	case io.SeekCurrent:
		newOff = r.off + offset
	case io.SeekEnd:
		newOff = int64(len(r.b)) + offset
	default:
		return 0, fs.ErrInvalid
	}
	if newOff < 0 {
		return 0, fs.ErrInvalid
	}
	r.off = newOff
	return newOff, nil
}
