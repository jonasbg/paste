package spa

import (
	"mime"
	"net/http"
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

		// Serve the requested file if it exists
		if _, err := filepath.Rel(urlPrefix, path); err == nil {
			if _, err := http.Dir(spaDirectory).Open(strings.TrimPrefix(path, "/")); err == nil {
				// Set the correct Content-Type header based on file extension
				ext := filepath.Ext(path)
				if ext != "" {
					if mimeType := mime.TypeByExtension(ext); mimeType != "" {
						c.Header("Content-Type", mimeType)
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
