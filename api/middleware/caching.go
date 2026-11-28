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

const (
	maxStaticCacheEntries = 200
	maxStaticCacheBytes   = 64 * 1024 * 1024 // 64 MiB
)

// staticFile represents a cached immutable asset kept fully in memory.
type staticFile struct {
	content      []byte
	contentType  string
	etag         string
	modTime      time.Time
	lastModified string
}

// in-memory cache (bounded, only for fingerprinted assets under /assets/)
var (
	staticCache      = make(map[string]*staticFile)
	staticCacheBytes int64
	staticCacheMu    sync.RWMutex
)

// APINoStore forces no-store caching on all API routes so browsers and proxies
// never persist encrypted blobs, metadata, or config responses.
func APINoStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

// Load and cache a static immutable file under /assets/. Returns nil if not
// suitable for caching, exceeds cache budget, or on error.
func getOrLoadStatic(spaDir, relPath string) *staticFile {
	relPath, fullPath := resolveInDir(spaDir, relPath)
	if !strings.HasPrefix(relPath, "assets/") || !strings.Contains(relPath, ".") {
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

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() || info.Size() > maxStaticCacheBytes {
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
	if len(staticCache) < maxStaticCacheEntries && staticCacheBytes+int64(len(data)) <= maxStaticCacheBytes {
		if _, exists := staticCache[relPath]; !exists {
			staticCache[relPath] = sf
			staticCacheBytes += int64(len(data))
		}
	}
	staticCacheMu.Unlock()
	return sf
}

func mimeTypeByExtension(ext string) string {
	if ext == ".wasm" {
		return "application/wasm"
	}
	if ext == "" {
		return "application/octet-stream"
	}
	if mt := mime.TypeByExtension(ext); mt != "" {
		return mt
	}
	return "application/octet-stream"
}

// CacheHeaders sets caching headers for static SPA assets.
// Only content-hashed /assets/* files (and explicitly ?v=-versioned static
// resources) receive immutable 1-year caching; unversioned files and SPA HTML
// routes use no-cache so deployments take effect immediately.
func CacheHeaders(spaDir string) gin.HandlerFunc {
	staticExt := map[string]struct{}{
		".js": {}, ".css": {}, ".wasm": {}, ".png": {}, ".jpg": {}, ".jpeg": {},
		".gif": {}, ".svg": {}, ".webp": {}, ".ico": {}, ".ttf": {}, ".woff": {}, ".woff2": {},
	}

	return func(c *gin.Context) {
		p := c.Request.URL.Path

		if strings.HasPrefix(p, "/api/") {
			c.Header("Cache-Control", "no-store")
			return
		}

		// SPA entry points
		if p == "/" || p == "/index.html" {
			c.Header("Cache-Control", "no-cache")
			if f, err := http.Dir(spaDir).Open("index.html"); err == nil {
				defer f.Close()
				h := sha1.New()
				_, _ = io.CopyN(h, f, 32*1024)
				c.Header("ETag", `W/"`+hex.EncodeToString(h.Sum(nil))+`"`)
			}
			return
		}

		ext := filepath.Ext(p)
		if _, ok := staticExt[ext]; ok {
			// Fingerprinted build assets under /assets/
			if strings.HasPrefix(p, "/assets/") {
				if f := getOrLoadStatic(spaDir, p); f != nil {
					c.Header("Cache-Control", "public,max-age=31536000,immutable")
					c.Header("ETag", f.etag)
					c.Header("Last-Modified", f.lastModified)

					inm := c.GetHeader("If-None-Match")
					ims := c.GetHeader("If-Modified-Since")
					if (inm != "" && inm == f.etag) || (ims != "" && ims == f.lastModified) {
						c.Status(http.StatusNotModified)
						c.Abort()
						return
					}

					c.Header("Content-Type", f.contentType)
					br := &bytesReader{b: f.content}
					http.ServeContent(c.Writer, c.Request, p, f.modTime, br)
					c.Abort()
					return
				}
				c.Header("Cache-Control", "public,max-age=31536000,immutable")
				return
			}

			// Unversioned static files (e.g. /encryption.wasm, /wasm_exec.js):
			// only mark immutable when explicitly versioned via ?v=...
			if c.Query("v") != "" {
				c.Header("Cache-Control", "public,max-age=31536000,immutable")
			} else {
				c.Header("Cache-Control", "no-cache")
			}
			return
		}

		// SPA fallback routes (e.g. /{fileId}) serve index.html -> never cache stale HTML
		c.Header("Cache-Control", "no-cache")
	}
}

// bytesReader implements io.ReadSeeker for a byte slice without copying.
type bytesReader struct {
	b   []byte
	off int64
}

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
