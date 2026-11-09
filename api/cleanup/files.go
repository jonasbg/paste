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

// cleanupInterval is how often expired files are swept. It is short relative
// to the retention period so files do not outlive it by much, and the first
// sweep runs at startup so frequent restarts cannot postpone deletion forever.
const cleanupInterval = time.Hour

func StartFileCleanup(uploadDir string) {
	cleanupDays := GetCleanupDays()
	log.Printf("File cleanup configured for %d days", cleanupDays)

	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			if err := cleanOldFiles(uploadDir, cleanupDays); err != nil {
				log.Printf("Failed to clean old files: %v", err)
			}
			<-ticker.C
		}
	}()
}

// cleanOldFiles removes uploads (including stale .tmp and .reserve files)
// older than the retention period. A failure on one file is logged and does
// not stop the sweep.
func cleanOldFiles(uploadDir string, days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)

	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue // removed concurrently
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		path := filepath.Join(uploadDir, entry.Name())
		if err := os.Remove(path); err != nil {
			log.Printf("Failed to remove old file %s: %v", path, err)
			continue
		}
		log.Printf("Removed old file: %s (age: %.1f days)", path, time.Since(info.ModTime()).Hours()/24)
	}
	return nil
}
