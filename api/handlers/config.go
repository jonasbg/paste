package handlers

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

var GlobalConfig Config

// Config represents the application configuration.
type Config struct {
	MaxFileSize      string `json:"max_file_size"`
	MaxFileSizeBytes int    `json:"max_file_size_bytes"`
	IDSize           int    `json:"id_size"`
	KeySize          int    `json:"key_size"`
	ChunkSize        int    `json:"chunk_size"`
}

func InitConfig() error {
	maxFileSize := getEnv("MAX_FILE_SIZE", "100MB")
	idSizeStr := getEnv("ID_SIZE", "128")
	keySizeStr := getEnv("KEY_SIZE", "256")
	chunkSizeStr := getEnv("CHUNK_SIZE", "4")

	// Validate maxFileSize
	if !isValidFileSize(maxFileSize) {
		return fmt.Errorf("invalid MAX_FILE_SIZE format. Must be a number followed by B, KB, MB, GB, or TB (case-insensitive)")
	}

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
	if err != nil {
		return fmt.Errorf("invalid CHUNK_SIZE. Must be an integer")
	}

	GlobalConfig = Config{
		MaxFileSize:      maxFileSize,
		MaxFileSizeBytes: int(maxFileSizeBytes),
		IDSize:           idSize,
		KeySize:          keySize,
		ChunkSize:        chunkSize,
	}

	return nil
}

func parseFileSize(size string) (int64, error) {
	// Regular expression to match number followed by unit
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([KMGT]?B)$`)
	matches := re.FindStringSubmatch(strings.ToUpper(size))

	if matches == nil {
		return 0, fmt.Errorf("invalid size format: %s", size)
	}

	num, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", matches[1])
	}

	multipliers := map[string]int64{
		"B":  1,
		"KB": 1 << 10, // 1024
		"MB": 1 << 20, // 1024 * 1024
		"GB": 1 << 30, // 1024 * 1024 * 1024
		"TB": 1 << 40, // 1024 * 1024 * 1024 * 1024
	}

	multiplier, ok := multipliers[matches[2]]
	if !ok {
		return 0, fmt.Errorf("invalid unit: %s", matches[2])
	}

	bytes := int64(num * float64(multiplier))
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

// getEnv retrieves an environment variable with a default value.
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// isValidFileSize checks if a given file size string is valid.
func isValidFileSize(s string) bool {
	s = strings.ToUpper(s)
	validSuffixes := []string{"KB", "MB", "GB", "TB"}
	for _, suffix := range validSuffixes {
		if strings.HasSuffix(s, suffix) {
			numPart := s[:len(s)-len(suffix)]
			_, err := strconv.ParseFloat(numPart, 64) //Accepts floats for flexibility
			return err == nil
		}
	}
	return false
}
