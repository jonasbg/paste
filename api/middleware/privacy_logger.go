package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

// PrivacyLogger replaces gin.Logger to avoid printing raw client IPs to stdout.
func PrivacyLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {

		return fmt.Sprintf("[GIN] %s | %3d | %15s | %-7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.StatusCode,
			param.Latency.Truncate(time.Microsecond),
			param.Method,
			param.Path,
		)
	})
}
