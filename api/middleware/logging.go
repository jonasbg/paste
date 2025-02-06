package middleware

import (
	"bytes"
	"net"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
	"github.com/jonasbg/paste/m/v2/types"
)

// isPrivateIP checks if an IP address is private
func isPrivateIP(ip net.IP) bool {
	// Check against private IP ranges
	privateRanges := []struct {
		start net.IP
		end   net.IP
	}{
		{net.ParseIP("10.0.0.0"), net.ParseIP("10.255.255.255")},
		{net.ParseIP("172.16.0.0"), net.ParseIP("172.31.255.255")},
		{net.ParseIP("192.168.0.0"), net.ParseIP("192.168.255.255")},
		{net.ParseIP("127.0.0.0"), net.ParseIP("127.255.255.255")},
	}

	for _, r := range privateRanges {
		if bytes.Compare(ip, r.start) >= 0 && bytes.Compare(ip, r.end) <= 0 {
			return true
		}
	}
	return false
}

// getRealIP attempts to get the real client IP address
func getRealIP(c *gin.Context) string {
	// List of headers to check for IP addresses
	headers := []string{
		"X-Real-IP",
		"X-Forwarded-For",
		"X-Client-IP",
		"CF-Connecting-IP", // Cloudflare
		"True-Client-IP",
	}

	var candidateIPs []string

	// Check header-provided IPs
	for _, header := range headers {
		if ip := c.GetHeader(header); ip != "" {
			// X-Forwarded-For may contain multiple IPs
			if header == "X-Forwarded-For" {
				ips := strings.Split(ip, ",")
				for _, ip := range ips {
					candidateIPs = append(candidateIPs, strings.TrimSpace(ip))
				}
			} else {
				candidateIPs = append(candidateIPs, strings.TrimSpace(ip))
			}
		}
	}

	// Add the direct remote address
	if remoteAddr := c.Request.RemoteAddr; remoteAddr != "" {
		ip, _, err := net.SplitHostPort(remoteAddr)
		if err == nil {
			candidateIPs = append(candidateIPs, ip)
		}
	}

	// First pass: look for valid public IPs
	for _, ipStr := range candidateIPs {
		ip := net.ParseIP(ipStr)
		if ip != nil && !isPrivateIP(ip) && !ip.IsLoopback() && !ip.IsUnspecified() {
			return ipStr
		}
	}

	// Second pass: accept private IPs if no public IP was found
	for _, ipStr := range candidateIPs {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			return ipStr
		}
	}

	// Fallback to RemoteAddr if everything else fails
	if remoteAddr := c.Request.RemoteAddr; remoteAddr != "" {
		ip, _, err := net.SplitHostPort(remoteAddr)
		if err == nil {
			return ip
		}
	}

	return ""
}

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
				IP:        getRealIP(c), // Use our enhanced IP detection
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
		c.Set("clientIP", getRealIP(c)) // Use enhanced IP detection here too
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
