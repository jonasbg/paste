package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

// HashIP returns a deterministic hash of the provided IP address.
// An optional LOG_HASH_SALT environment variable can be supplied to
// introduce instance-specific entropy while keeping the hash stable.
func HashIP(ip string) string {
	if ip == "" {
		return ""
	}

	hasher := sha256.New()
	if salt := os.Getenv("LOG_HASH_SALT"); salt != "" {
		hasher.Write([]byte(salt))
		hasher.Write([]byte("|"))
	}
	hasher.Write([]byte(ip))
	return hex.EncodeToString(hasher.Sum(nil))
}
