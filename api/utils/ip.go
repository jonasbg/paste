package utils

import (
	"bytes"
	"net"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetTrustedProxies() []string {
	// Get proxies from environment variable, default to Kubernetes range if not set
	proxiesEnv := os.Getenv("TRUSTED_PROXIES")
	if proxiesEnv == "" {
		return []string{"10.0.0.0/8"}
	}

	// Split by comma and trim spaces
	proxies := strings.Split(proxiesEnv, ",")
	for i := range proxies {
		proxies[i] = strings.TrimSpace(proxies[i])
	}

	return proxies
}

func GetRealIP(c *gin.Context) string {
	// Get the immediate client IP
	remoteAddr := c.Request.RemoteAddr
	clientIP, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr // Fallback if we can't parse it
	}

	// Check if the client is a trusted proxy
	clientIPParsed := net.ParseIP(clientIP)
	if clientIPParsed == nil {
		return clientIP // Return immediate client IP if we can't parse it
	}

	// Only proceed with header checking if the immediate client is a trusted proxy
	trustedProxies := GetTrustedProxies()
	isTrusted := false

	for _, proxyRange := range trustedProxies {
		_, ipNet, err := net.ParseCIDR(proxyRange)
		if err == nil && ipNet.Contains(clientIPParsed) {
			isTrusted = true
			break
		} else if proxyRange == clientIP {
			isTrusted = true
			break
		}
	}

	if !isTrusted {
		return clientIP // If not from a trusted proxy, return the immediate client IP
	}

	// From trusted proxy, check headers in order of preference
	if cfIP := c.GetHeader("CF-Connecting-IP"); cfIP != "" {
		return cfIP // Cloudflare-specific header
	}

	if realIP := c.GetHeader("X-Real-IP"); realIP != "" {
		return realIP
	}

	// For X-Forwarded-For, use the leftmost value as it's the original client
	if forwardedFor := c.GetHeader("X-Forwarded-For"); forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(ips[0])
	}

	// Fallback to other headers
	if clientIP := c.GetHeader("X-Client-IP"); clientIP != "" {
		return clientIP
	}

	if trueClientIP := c.GetHeader("True-Client-IP"); trueClientIP != "" {
		return trueClientIP
	}

	// If we get here, just return the immediate client IP
	return clientIP
}

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
