package middleware

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUntrustedProxyWarning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	r := gin.New()
	r.Use(UntrustedProxyWarning([]string{"10.0.0.0/8", "192.168.1.10"}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	send := func(peer string, xff bool) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = peer + ":12345"
		if xff {
			req.Header.Set("X-Forwarded-For", "203.0.113.7")
		}
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	cases := []struct {
		peer string
		xff  bool
		warn bool
	}{
		{"172.18.0.5", true, true},    // private, untrusted proxy
		{"172.18.0.5", true, false},   // same peer reported once
		{"10.1.2.3", true, false},     // trusted CIDR
		{"192.168.1.10", true, false}, // trusted single IP
		{"100.64.3.4", true, true},    // CGNAT range counts as internal
		{"198.51.100.9", true, false}, // public client spoofing: ignored
		{"172.18.0.6", false, false},  // no forwarding header
	}
	for _, tc := range cases {
		buf.Reset()
		send(tc.peer, tc.xff)
		if got := strings.Contains(buf.String(), "not in TRUSTED_PROXIES"); got != tc.warn {
			t.Errorf("peer %s xff=%v: warned=%v, want %v (log: %q)", tc.peer, tc.xff, got, tc.warn, buf.String())
		}
	}

	// Distinct peers are capped.
	buf.Reset()
	for i := 0; i < 2*maxWarnedPeers; i++ {
		send(fmt.Sprintf("172.20.0.%d", i+1), true)
	}
	if n := strings.Count(buf.String(), "not in TRUSTED_PROXIES"); n != maxWarnedPeers-2 {
		t.Errorf("got %d warnings, want %d (cap minus the two earlier peers)", n, maxWarnedPeers-2)
	}
	if !strings.Contains(buf.String(), "further warnings suppressed") {
		t.Error("missing suppression notice")
	}
}
