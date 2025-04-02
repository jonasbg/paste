package middleware

import (
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
	"github.com/jonasbg/paste/m/v2/types"
	"github.com/jonasbg/paste/m/v2/utils"
)

// Global log manager instance
var (
	logManager *LogManager
	logOnce    sync.Once
)

// InitLogManager initializes the global log manager
func InitLogManager(database *db.DB, options ...func(*LogManager)) {
	logOnce.Do(func() {
		logManager = NewLogManager(database, options...)
	})
}

// CloseLogManager shuts down the log manager and flushes all logs
func CloseLogManager() {
	if logManager != nil {
		logManager.Close()
	}
}

// Logger returns a middleware function that logs requests
func Logger(database *db.DB) gin.HandlerFunc {
	// Initialize log manager if not already done
	InitLogManager(database)

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

		// Log the request using the log manager
		logManager.LogRequest(requestLog)

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
				logManager.LogTransaction(tx)
			}
		}

		// Add common log info to context
		c.Set("requestDuration", duration)
		c.Set("clientIP", requestLog.IP)
		c.Set("requestBodySize", bodySize)
		c.Set("method", requestMethod)
	}
}

// The helper functions remain unchanged
func isFileOperation(path string) bool {
	return strings.HasPrefix(path, "/api/upload") ||
		strings.HasPrefix(path, "/api/download") ||
		strings.HasPrefix(path, "/api/metadata") ||
		strings.HasPrefix(path, "/api/ws/upload")
}

func getMethodType(c *gin.Context) string {
	if c.IsWebsocket() {
		return "websocket"
	}
	return strings.ToLower(c.Request.Method)
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
