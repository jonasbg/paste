package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
	"github.com/jonasbg/paste/m/v2/types"
)

func Logger(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Create transaction log for file operations
		var tx *types.TransactionLog
		if isFileOperation(path) {
			tx = &types.TransactionLog{
				Timestamp: start,
				Action:    getActionType(path, method),
				IP:        c.ClientIP(),
				UserAgent: c.Request.UserAgent(),
			}

			// Handle download/metadata requests - ID is in URL param
			if tx.Action == "download" || tx.Action == "metadata" {
				fileID := c.Param("id")
				if len(fileID) == 32 { // Validate ID length
					tx.FileID = fileID
				}
			}
		}

		// Process request
		c.Next()

		// Common metrics
		duration := time.Since(start)
		statusCode := c.Writer.Status()
		bodySize := c.Writer.Size()

		// Complete transaction log if it exists
		if tx != nil {
			tx.Duration = duration.Milliseconds()
			tx.StatusCode = statusCode
			tx.Size = int64(bodySize)
			tx.Success = statusCode >= 200 && statusCode < 300

			// Get error if any
			if len(c.Errors) > 0 {
				tx.Error = c.Errors.String()
			}

			// For uploads, get the file ID from the response
			if tx.Action == "upload" && statusCode == 200 {
				// Extract ID from response
				if response, exists := c.Get("responseData"); exists {
					if data, ok := response.(gin.H); ok {
						if id, exists := data["id"].(string); exists {
							tx.FileID = id
						}
					}
				}
			}

			// Only log if we have a valid file ID
			if tx.FileID != "" || statusCode != 200 {
				if err := database.LogTransaction(tx); err != nil {
					c.Error(err)
				}
			}
		}

		// Add common log info to context
		c.Set("requestDuration", duration)
		c.Set("clientIP", c.ClientIP())
		c.Set("requestBodySize", bodySize)
	}
}

func isFileOperation(path string) bool {
	return strings.HasPrefix(path, "/api/upload") ||
		strings.HasPrefix(path, "/api/download") ||
		strings.HasPrefix(path, "/api/metadata")
}

func getActionType(path string, method string) string {
	switch {
	case strings.HasPrefix(path, "/api/upload"):
		return "upload"
	case strings.HasPrefix(path, "/api/download"):
		return "download"
	case strings.HasPrefix(path, "/api/metadata"):
		return "metadata"
	default:
		return "unknown"
	}
}
