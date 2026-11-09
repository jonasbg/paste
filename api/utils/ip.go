package utils

import (
	"os"
	"strings"
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
