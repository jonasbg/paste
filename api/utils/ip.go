package utils

import (
	"bytes"
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetRealIP(c *gin.Context) string {
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
