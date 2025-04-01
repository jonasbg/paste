package middleware

import (
	"fmt"
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// IPSourceRestriction creates middleware that only allows requests from specified IP ranges
func IPSourceRestriction(allowedCIDRs string) gin.HandlerFunc {
	// Parse the comma-separated list of CIDRs
	cidrs := parseCIDRs(allowedCIDRs)

	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		// If no restrictions are set, allow all requests
		if len(cidrs) == 0 {
			c.Next()
			return
		}

		// Check if the client IP is in the allowed ranges
		if !isIPAllowed(clientIP, cidrs) {
			fmt.Printf("Access denied for IP: %s\n", clientIP)
			c.AbortWithStatusJSON(403, gin.H{"error": "Access denied from this IP address"})
			return
		}

		c.Next()
	}
}

// parseCIDRs converts a comma-separated string of CIDRs to a slice of *net.IPNet
func parseCIDRs(cidrList string) []*net.IPNet {
	if cidrList == "" {
		return nil
	}

	var cidrs []*net.IPNet
	for _, cidr := range strings.Split(cidrList, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}

		// If the CIDR doesn't contain a slash, assume it's a single IP and add /32 (IPv4) or /128 (IPv6)
		if !strings.Contains(cidr, "/") {
			ip := net.ParseIP(cidr)
			if ip == nil {
				// Invalid IP, skip it
				continue
			}
			if ip.To4() != nil {
				cidr = cidr + "/32" // IPv4
			} else {
				cidr = cidr + "/128" // IPv6
			}
		}

		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Invalid CIDR, skip it
			continue
		}
		cidrs = append(cidrs, ipNet)
	}

	return cidrs
}

// isIPAllowed checks if the given IP is within any of the allowed CIDR ranges
func isIPAllowed(ipStr string, allowedCIDRs []*net.IPNet) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, cidr := range allowedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}
