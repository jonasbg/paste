package middleware

import (
	"mime"
	"net/http"
	"os"
	pathpkg "path"
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
		_, full := resolveInDir(spaDirectory, strings.TrimPrefix(path, urlPrefix))

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

		// If the file doesn't exist, serve the index.html
		c.Request.URL.Path = "/"
		c.Header("Content-Type", "text/html")
		fileserver.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}

// resolveInDir maps a request path onto a file inside root. Cleaning the path
// as a rooted path drops every ".." segment, so the result can never escape
// root (the same approach http.Dir uses). Returns the root-relative slash path
// and the full filesystem path.
func resolveInDir(root, urlPath string) (rel, full string) {
	rel = strings.TrimPrefix(pathpkg.Clean("/"+urlPath), "/")
	return rel, filepath.Join(root, filepath.FromSlash(rel))
}
