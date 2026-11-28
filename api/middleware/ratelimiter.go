package middleware

import (
	"net"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

const (
	// limiterIdleTTL is how long an idle client keeps its bucket.
	limiterIdleTTL = 10 * time.Minute
	// maxTrackedIPs bounds memory against IP-cycling floods (e.g. IPv6 /64).
	maxTrackedIPs = 50000
)

type limiterInfo struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type IPRateLimiter struct {
	ips map[string]*limiterInfo
	mu  sync.Mutex
	r   rate.Limit
	b   int
}

func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		ips: make(map[string]*limiterInfo),
		r:   r,
		b:   b,
	}
	go i.cleanupLoop()
	return i
}

// normalizeClientKey collapses IPv6 addresses to their /64 network prefix so a
// single client cannot evade limits or exhaust memory by rotating host IDs.
func normalizeClientKey(rawIP string) string {
	ip := net.ParseIP(rawIP)
	if ip == nil {
		return rawIP
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	masked := ip.Mask(net.CIDRMask(64, 128))
	return masked.String() + "/64"
}

func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	key := normalizeClientKey(ip)

	i.mu.Lock()
	defer i.mu.Unlock()

	now := time.Now()
	if info, exists := i.ips[key]; exists {
		info.lastSeen = now
		return info.limiter
	}

	if len(i.ips) >= maxTrackedIPs {
		i.evictLocked(now)
	}

	// If still at capacity after eviction, return a strict fallback limiter
	// without growing the map further.
	if len(i.ips) >= maxTrackedIPs {
		return rate.NewLimiter(i.r, 1)
	}

	info := &limiterInfo{
		limiter:  rate.NewLimiter(i.r, i.b),
		lastSeen: now,
	}
	i.ips[key] = info
	return info.limiter
}

// evictLocked drops expired entries first; if the map is still full, it drops
// the oldest entry so map size stays bounded.
func (i *IPRateLimiter) evictLocked(now time.Time) {
	for k, info := range i.ips {
		if now.Sub(info.lastSeen) > limiterIdleTTL {
			delete(i.ips, k)
		}
	}
	if len(i.ips) < maxTrackedIPs {
		return
	}
	var oldestKey string
	var oldestTime time.Time
	for k, info := range i.ips {
		if oldestKey == "" || info.lastSeen.Before(oldestTime) {
			oldestKey = k
			oldestTime = info.lastSeen
		}
	}
	if oldestKey != "" {
		delete(i.ips, oldestKey)
	}
}

func (i *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		now := time.Now()
		i.mu.Lock()
		for ip, info := range i.ips {
			if now.Sub(info.lastSeen) > limiterIdleTTL {
				delete(i.ips, ip)
			}
		}
		i.mu.Unlock()
	}
}

func RateLimit(limiter *IPRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !limiter.GetLimiter(c.ClientIP()).Allow() {
			c.JSON(429, gin.H{
				"error":       "Too many requests",
				"retry_after": "1s",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
