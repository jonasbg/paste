package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
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

func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	info, exists := i.ips[ip]
	if !exists {
		info = &limiterInfo{
			limiter:  rate.NewLimiter(i.r, i.b),
			lastSeen: time.Now(),
		}
		i.ips[ip] = info
	} else {
		info.lastSeen = time.Now()
	}
	return info.limiter
}

// limiterIdleTTL is how long an idle IP keeps its bucket. A bucket refills
// completely within seconds, so a short TTL bounds memory without loosening
// the limit.
const limiterIdleTTL = 10 * time.Minute

func (i *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		i.mu.Lock()
		for ip, info := range i.ips {
			if time.Since(info.lastSeen) > limiterIdleTTL {
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
