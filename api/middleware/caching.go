package middleware

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

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
			c.Header("Cache-Control", "public,max-age=31536000,immutable")
			// Optional: strong ETag if desired (skip for performance)
			return
		}

		// Fallback: short cache
		c.Header("Cache-Control", "public,max-age=300")
	}
}
