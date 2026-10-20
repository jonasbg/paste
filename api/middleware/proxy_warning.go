package middleware

import (
	"log"
	"net"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// maxWarnedPeers caps how many distinct peers are reported, so the log
// cannot be flooded.
const maxWarnedPeers = 10

// cgnatRange (100.64.0.0/10) is used for pod and node networks by some
// clusters but is not covered by net.IP.IsPrivate.
var cgnatRange = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// UntrustedProxyWarning logs when a peer on a private network sends
// forwarding headers but is not in TRUSTED_PROXIES. That usually means the
// reverse proxy's range is missing, so every user appears as the proxy's IP
// and shares one rate limit and upload limit. Public peers are ignored:
// internet clients may send these headers themselves. Each peer is reported
// once, up to maxWarnedPeers.
func UntrustedProxyWarning(trustedProxies []string) gin.HandlerFunc {
	trusted := parseIPNets(trustedProxies)

	var (
		mu     sync.Mutex
		warned = make(map[string]struct{})
	)

	return func(c *gin.Context) {
		header := ""
		for _, h := range []string{"X-Forwarded-For", "X-Real-IP"} {
			if c.GetHeader(h) != "" {
				header = h
				break
			}
		}
		if header == "" {
			c.Next()
			return
		}

		peer := net.ParseIP(c.RemoteIP())
		if peer == nil || !isInternal(peer) || containsIP(trusted, peer) {
			c.Next()
			return
		}

		mu.Lock()
		_, seen := warned[peer.String()]
		full := len(warned) >= maxWarnedPeers
		if !seen && !full {
			warned[peer.String()] = struct{}{}
			if len(warned) == maxWarnedPeers {
				defer log.Printf("WARNING: untrusted proxy warnings reached %d peers; further warnings suppressed", maxWarnedPeers)
			}
		}
		mu.Unlock()

		if !seen && !full {
			log.Printf("WARNING: request from %s carries %s but %s is not in TRUSTED_PROXIES; "+
				"if it is your reverse proxy, add it, otherwise all users share its IP for rate and upload limits",
				peer, header, peer)
		}
		c.Next()
	}
}

func isInternal(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || cgnatRange.Contains(ip)
}

// parseIPNets accepts the same forms as gin's SetTrustedProxies: CIDRs or
// single IPs.
func parseIPNets(entries []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if _, n, err := net.ParseCIDR(e); err == nil {
			nets = append(nets, n)
			continue
		}
		if ip := net.ParseIP(e); ip != nil {
			bits := 128
			if ip.To4() != nil {
				ip, bits = ip.To4(), 32
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		}
	}
	return nets
}

func containsIP(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
