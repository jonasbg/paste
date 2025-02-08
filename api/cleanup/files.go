package cleanup

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func GetCleanupDays() int {
	if days := os.Getenv("FILES_RETENTION_DAYS"); days != "" {
		if val, err := strconv.Atoi(days); err == nil && val > 0 {
			return val
		}
		log.Printf("Invalid FILES_RETENTION_DAYS value, using default of 7 days")
	}
	return 7
}

func StartFileCleanup(uploadDir string) {
	cleanupDays := GetCleanupDays()
	log.Printf("File cleanup configured for %d days", cleanupDays)

	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for range ticker.C {
			if err := cleanOldFiles(uploadDir, cleanupDays); err != nil {
				log.Printf("Failed to clean old files: %v", err)
			}
		}
	}()
}

func cleanOldFiles(uploadDir string, days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)

	return filepath.Walk(uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the directory itself
		if path == uploadDir {
			return nil
		}

		// Check if file is older than cutoff
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				log.Printf("Failed to remove old file %s: %v", path, err)
				return err
			}
			log.Printf("Removed old file: %s (age: %v days)", path, time.Since(info.ModTime()).Hours()/24)
		}

		return nil
	})
}
