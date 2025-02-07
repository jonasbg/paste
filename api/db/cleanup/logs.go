// m/v2/cleanup/logs.go
package cleanup

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jonasbg/paste/m/v2/db"
)

const (
	defaultLogsRetentionDays = 180
	envLogsRetentionDays     = "LOGS_RETENTION_DAYS"
)

func GetLogsRetentionDays() int {
	if days := os.Getenv(envLogsRetentionDays); days != "" {
		if val, err := strconv.Atoi(days); err == nil {
			if val < 0 {
				log.Printf("Logs retention set to infinite")
				return -1
			}
			if val > 0 {
				return val
			}
		}
		log.Printf("Invalid %s value, using default of %d days", envLogsRetentionDays, defaultLogsRetentionDays)
	}
	return defaultLogsRetentionDays
}

func StartLogRotation(db *db.DB) {
	retentionDays := GetLogsRetentionDays()
	if retentionDays < 0 {
		log.Printf("Log rotation disabled (infinite retention)")
		return
	}

	log.Printf("Log rotation configured for %d days", retentionDays)
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for range ticker.C {
			cutoff := time.Now().AddDate(0, 0, -retentionDays)
			if err := db.CleanOldLogs(cutoff); err != nil {
				log.Printf("Failed to clean old logs: %v", err)
			}
		}
	}()
}
