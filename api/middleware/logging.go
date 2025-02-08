package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
	"github.com/jonasbg/paste/m/v2/types"
	"github.com/jonasbg/paste/m/v2/utils"
)

func Logger(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		requestMethod := getMethodType(c)

		// Initialize request log
		requestLog := &types.RequestLog{
			Timestamp:   start,
			IP:          utils.GetRealIP(c),
			Method:      requestMethod,
			Path:        path,
			UserAgent:   c.Request.UserAgent(),
			QueryParams: c.Request.URL.RawQuery,
		}

		// Create transaction log for file operations
		var tx *types.TransactionLog
		if isFileOperation(path) {
			tx = &types.TransactionLog{
				Timestamp: start,
				Action:    getActionType(path, requestMethod),
				IP:        requestLog.IP,
				UserAgent: requestLog.UserAgent,
				Method:    requestMethod,
			}

			// Handle download/metadata requests - ID is in URL param
			if tx.Action == "download" || tx.Action == "metadata" {
				fileID := c.Param("id")
				if len(fileID) == 32 {
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

		// Update request log
		requestLog.Duration = duration.Milliseconds()
		requestLog.StatusCode = statusCode
		requestLog.BodySize = int64(bodySize)
		if len(c.Errors) > 0 {
			requestLog.Error = c.Errors.String()
		}

		// Log the request
		if err := database.LogRequest(requestLog); err != nil {
			c.Error(err)
		}

		// Complete transaction log if it exists
		if tx != nil {
			tx.Duration = duration.Milliseconds()
			tx.StatusCode = statusCode
			tx.Size = int64(bodySize)
			tx.Success = (statusCode >= 200 && statusCode < 300) || requestMethod == "websocket"

			if len(c.Errors) > 0 {
				tx.Error = c.Errors.String()
			}

			// For uploads, get the file ID from the response
			if tx.Action == "upload" {
				if response, exists := c.Get("responseData"); exists {
					if data, ok := response.(gin.H); ok {
						if id, exists := data["id"].(string); exists {
							tx.FileID = id
						}
					}
				}
			}

			// Only log if we have a valid file ID or there was an error
			if tx.FileID != "" || !tx.Success {
				if err := database.LogTransaction(tx); err != nil {
					c.Error(err)
				}
			}
		}

		// Add common log info to context
		c.Set("requestDuration", duration)
		c.Set("clientIP", requestLog.IP)
		c.Set("requestBodySize", bodySize)
		c.Set("method", requestMethod)
	}
}

func isFileOperation(path string) bool {
	return strings.HasPrefix(path, "/api/upload") ||
		strings.HasPrefix(path, "/api/download") ||
		strings.HasPrefix(path, "/api/metadata") ||
		strings.HasPrefix(path, "/api/ws/upload") // Add WebSocket path
}

func getMethodType(c *gin.Context) string {
	if c.IsWebsocket() {
		return "websocket"
	}
	return strings.ToLower(c.Request.Method) // "get", "post", etc.
}

func getActionType(path string, method string) string {
	switch {
	case strings.HasPrefix(path, "/api/upload") || strings.HasPrefix(path, "/api/ws/upload"):
		return "upload"
	case strings.HasPrefix(path, "/api/download"):
		return "download"
	case strings.HasPrefix(path, "/api/metadata"):
		return "metadata"
	default:
		return "unknown"
	}
}
