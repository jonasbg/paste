package middleware

import (
	"fmt"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// PrivacyLogger replaces gin.Logger to avoid printing raw client IPs, file IDs,
// or query strings to stdout. Only the matched route template is logged.
func PrivacyLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		fmt.Fprintf(os.Stdout, "[GIN] %s | %3d | %15s | %-7s %s\n",
			time.Now().Format("2006/01/02 - 15:04:05"),
			c.Writer.Status(),
			time.Since(start).Truncate(time.Microsecond),
			c.Request.Method,
			route,
		)
	}
}
