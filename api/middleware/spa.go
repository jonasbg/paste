package middleware

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func Middleware(urlPrefix, spaDirectory string) gin.HandlerFunc {
	fileserver := http.FileServer(http.Dir(spaDirectory))
	if urlPrefix != "" {
		fileserver = http.StripPrefix(urlPrefix, fileserver)
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Serve the requested file if it exists (Stat instead of Open)
		if _, err := filepath.Rel(urlPrefix, path); err == nil {
			rel := strings.TrimPrefix(path, "/")
			full := filepath.Join(spaDirectory, rel)
			if info, err := os.Stat(full); err == nil && !info.IsDir() {
				if ext := filepath.Ext(path); ext != "" {
					if mt := mime.TypeByExtension(ext); mt != "" {
						c.Header("Content-Type", mt)
					}
				}
				fileserver.ServeHTTP(c.Writer, c.Request)
				c.Abort()
				return
			}
		}

		// If the file doesn't exist, serve the index.html
		c.Request.URL.Path = "/"
		c.Header("Content-Type", "text/html")
		fileserver.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}
