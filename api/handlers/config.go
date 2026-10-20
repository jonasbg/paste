package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Passphrase-mode entropy floor. Passphrase-derived shares turn the passphrase
// into the *entire* secret (fileID, key, and token are all derivable from it),
// so the generated passphrase must clear a minimum entropy bar. The word count
// is configurable via PASSPHRASE_WORDS but clamped to [min,max]: operators can
// raise it for more entropy but cannot weaken it below the safe baseline.
const (
	defaultPassphraseWords = 4
	minPassphraseWords     = 4
	maxPassphraseWords     = 8

	minChunkSizeMB = 1
	maxChunkSizeMB = 64

	defaultUploadsPerIP = 4
)

var GlobalConfig Config

// Config represents the application configuration.
type Config struct {
	MaxFileSize      string `json:"max_file_size"`
	MaxFileSizeBytes int    `json:"max_file_size_bytes"`
	IDSize           int    `json:"id_size"`
	KeySize          int    `json:"key_size"`
	ChunkSize        int    `json:"chunk_size"`
	TokenMinLength   int    `json:"token_min_length"`
	PassphraseWords  int    `json:"passphrase_words"`

	// Server-side limits, not exposed to clients.
	MaxStorageBytes int64 `json:"-"` // 0 = unlimited
	UploadsPerIP    int   `json:"-"`
	// Extra browser origins allowed to open WebSockets, besides the request host.
	AllowedOrigins []string `json:"-"`
}

func InitConfig() error {
	maxFileSize := getEnv("MAX_FILE_SIZE", "100MB")
	idSizeStr := getEnv("ID_SIZE", "128")
	keySizeStr := getEnv("KEY_SIZE", "256")
	chunkSizeStr := getEnv("CHUNK_SIZE", "4")

	maxFileSizeBytes, err := parseFileSize(maxFileSize)
	if err != nil {
		return fmt.Errorf("failed to parse MAX_FILE_SIZE: %v", err)
	}

	// Validate and convert ID Size
	idSize, err := parseBitSize(idSizeStr, []int{64, 128, 192, 256})
	if err != nil {
		return fmt.Errorf("invalid ID_SIZE. Must be one of: 64, 128, 192, 256 (optionally followed by 'bit')")
	}

	// Validate and convert Key Size
	keySize, err := parseBitSize(keySizeStr, []int{128, 192, 256})
	if err != nil {
		return fmt.Errorf("invalid KEY_SIZE. Must be one of: 128, 192, 256 (optionally followed by 'bit')")
	}

	// Convert Chunk Size
	chunkSize, err := strconv.Atoi(chunkSizeStr)
	if err != nil || chunkSize < minChunkSizeMB || chunkSize > maxChunkSizeMB {
		return fmt.Errorf("invalid CHUNK_SIZE. Must be an integer between %d and %d (MB)", minChunkSizeMB, maxChunkSizeMB)
	}

	var maxStorageBytes int64
	if v := os.Getenv("MAX_STORAGE_SIZE"); v != "" {
		if maxStorageBytes, err = parseFileSize(v); err != nil {
			return fmt.Errorf("failed to parse MAX_STORAGE_SIZE: %v", err)
		}
	}

	uploadsPerIP, err := strconv.Atoi(getEnv("MAX_UPLOADS_PER_IP", strconv.Itoa(defaultUploadsPerIP)))
	if err != nil || uploadsPerIP < 1 {
		return fmt.Errorf("invalid MAX_UPLOADS_PER_IP. Must be a positive integer")
	}

	passphraseWords := parsePassphraseWords(getEnv("PASSPHRASE_WORDS", strconv.Itoa(defaultPassphraseWords)))

	GlobalConfig = Config{
		MaxFileSize:      maxFileSize,
		MaxFileSizeBytes: int(maxFileSizeBytes),
		IDSize:           idSize,
		KeySize:          keySize,
		ChunkSize:        chunkSize,
		TokenMinLength:   calculateTokenMinLength(keySize),
		PassphraseWords:  passphraseWords,
		MaxStorageBytes:  maxStorageBytes,
		UploadsPerIP:     uploadsPerIP,
		AllowedOrigins:   parseList(os.Getenv("ALLOWED_ORIGINS")),
	}

	return nil
}

// parsePassphraseWords reads the configured passphrase word count and enforces
// the entropy floor by clamping to [minPassphraseWords, maxPassphraseWords].
// Invalid or out-of-range values are corrected with a warning rather than
// failing startup, so a misconfiguration can never silently weaken security.
func parsePassphraseWords(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		log.Printf("Invalid PASSPHRASE_WORDS %q, using default of %d", s, defaultPassphraseWords)
		return defaultPassphraseWords
	}
	if n < minPassphraseWords {
		log.Printf("PASSPHRASE_WORDS=%d is below the minimum of %d; clamping to %d", n, minPassphraseWords, minPassphraseWords)
		return minPassphraseWords
	}
	if n > maxPassphraseWords {
		log.Printf("PASSPHRASE_WORDS=%d exceeds the maximum of %d; clamping to %d", n, maxPassphraseWords, maxPassphraseWords)
		return maxPassphraseWords
	}
	return n
}

func calculateTokenMinLength(keyBits int) int {
	keyBytes := keyBits / 8
	if keyBytes == 0 {
		return 0
	}

	fullGroups := keyBytes / 3
	remainder := keyBytes % 3
	length := fullGroups * 4

	if remainder > 0 {
		length += remainder + 1
	}

	return length
}

var fileSizeRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(?:([KMGT])(?:I?B|I)?|B)?$`)

// parseFileSize parses sizes such as "100MB", "5GiB", "500Mi" or raw bytes
// ("1073741824"). Units are binary: KB, KiB and Ki all mean 1024 bytes.
func parseFileSize(size string) (int64, error) {
	matches := fileSizeRe.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(size)))
	if matches == nil {
		return 0, fmt.Errorf("invalid size %q: use raw bytes or a number followed by KB, MB, GB, TB (or KiB/Ki, ...)", size)
	}

	num, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", matches[1])
	}

	shift := map[string]uint{"": 0, "K": 10, "M": 20, "G": 30, "T": 40}[matches[2]]
	bytes := int64(num * float64(int64(1)<<shift))
	if bytes <= 0 {
		return 0, fmt.Errorf("size must be positive: %q", size)
	}
	return bytes, nil
}

// GetConfig returns a handler function that returns the current configuration
func GetConfig() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, GlobalConfig)
	}
}

func parseBitSize(size string, allowedSizes []int) (int, error) {
	// Remove "bit" suffix if present
	size = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(size)), "bit")
	size = strings.TrimSpace(size)

	// Convert to integer
	value, err := strconv.Atoi(size)
	if err != nil {
		return 0, err
	}

	// Validate against allowed sizes
	for _, allowed := range allowedSizes {
		if value == allowed {
			return value, nil
		}
	}
	return 0, fmt.Errorf("invalid bit size")
}

// parseList splits a comma-separated value, dropping empty entries.
func parseList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// getEnv retrieves an environment variable with a default value.
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
