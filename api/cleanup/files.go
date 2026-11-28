package cleanup

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
const (
	cleanupInterval  = time.Hour
	staleTempMaxAge  = 2 * time.Hour
	orphanMetaMaxAge = time.Minute
)

// CleanStaleTempFiles removes orphaned .tmp and .reserve files older than maxAge.
// Pass maxAge == 0 at startup (when no uploads can be in flight) to purge all
// leftover temp/reserve files left behind by a previous crash.
func CleanStaleTempFiles(uploadDir string, maxAge time.Duration) error {
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".tmp") && !strings.HasSuffix(name, ".reserve") {
			continue
		}
		if maxAge > 0 {
			info, err := entry.Info()
			if err != nil || !info.ModTime().Before(cutoff) {
				continue
			}
		}
		if err := os.Remove(filepath.Join(uploadDir, name)); err == nil {
			removed++
		}
	}
	if removed > 0 {
		log.Printf("Removed %d stale temporary/reservation file(s)", removed)
	}
	return nil
}

func StartFileCleanup(uploadDir string, onSweep func()) {
	cleanupDays := GetCleanupDays()
	log.Printf("File cleanup configured for %d days", cleanupDays)

	// Immediately clear any orphaned .tmp/.reserve files left by a prior crash.
	if err := CleanStaleTempFiles(uploadDir, 0); err != nil {
		log.Printf("Failed to clean startup temp files: %v", err)
	}

	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			if err := CleanStaleTempFiles(uploadDir, staleTempMaxAge); err != nil {
				log.Printf("Failed to clean stale temp files: %v", err)
			}
			if err := cleanOldFiles(uploadDir, cleanupDays); err != nil {
				log.Printf("Failed to clean old files: %v", err)
			}
			if onSweep != nil {
				onSweep()
			}
			<-ticker.C
		}
	}()
}

// redactFileID returns only the file ID prefix (before the first dot) so
// tokens or token digests never appear in logs.
func redactFileID(name string) string {
	if idx := strings.IndexByte(name, '.'); idx > 0 {
		return name[:idx]
	}
	return "<unknown>"
}

// cleanOldFiles removes uploads (and companion .meta files) older than the
// retention period, plus orphaned .meta files whose backing blob is gone.
func cleanOldFiles(uploadDir string, days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	orphanCutoff := time.Now().Add(-orphanMetaMaxAge)

	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		info, err := entry.Info()
		if err != nil {
			continue // removed concurrently
		}

		// Clean up orphaned .meta files whose data file no longer exists.
		if strings.HasSuffix(name, ".meta") {
			dataPath := filepath.Join(uploadDir, strings.TrimSuffix(name, ".meta"))
			if _, statErr := os.Stat(dataPath); errors.Is(statErr, fs.ErrNotExist) && info.ModTime().Before(orphanCutoff) {
				_ = os.Remove(filepath.Join(uploadDir, name))
			}
			continue
		}

		if !info.ModTime().Before(cutoff) {
			continue
		}
		path := filepath.Join(uploadDir, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("Failed to remove expired file id=%s", redactFileID(name))
			continue
		}
		_ = os.Remove(path + ".meta")
		log.Printf("Removed expired file id=%s (age: %.1f days)", redactFileID(name), time.Since(info.ModTime()).Hours()/24)
	}
	return nil
}
