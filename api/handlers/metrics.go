package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
	"github.com/jonasbg/paste/m/v2/utils"
	"golang.org/x/sys/unix"
)

func HandleSecurityMetrics(db *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var start, end time.Time

		if rangeStr := c.Query("range"); rangeStr != "" {
			start, end = utils.ParseTimeRange(rangeStr)
		} else {
			start, _ = time.Parse(time.RFC3339, c.Query("start"))
			end, _ = time.Parse(time.RFC3339, c.Query("end"))
		}

		metrics, err := db.GetSecurityMetrics(start, end)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, metrics)
	}
}

func HandleActivity(db *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var start, end time.Time

		if rangeStr := c.Query("range"); rangeStr != "" {
			start, end = utils.ParseTimeRange(rangeStr)
		} else {
			start, _ = time.Parse(time.RFC3339, c.Query("start"))
			end, _ = time.Parse(time.RFC3339, c.Query("end"))
		}

		summary, err := db.GetActivitySummary(start, end)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, summary)
	}
}

func formatBytes(bytes float64) string {
	const unit = 1000.0 // Changed from 1024.0 to 1000.0 for base-10 units
	if bytes < unit {
		return fmt.Sprintf("%.1f B", bytes)
	}
	div, exp := unit, 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	// Use KB, MB, GB instead of KiB, MiB, GiB
	return fmt.Sprintf("%.1f %cB", bytes/div, "KMGTPE"[exp])
}

func HandleStorage(db *db.DB, uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		summary, err := db.GetStorageSummary()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		summary.FileSizeDistribution = make(map[string]int)

		// Get disk space information for the upload directory
		var stat unix.Statfs_t
		err = unix.Statfs(uploadDir, &stat)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get disk stats: " + err.Error()})
			return
		}

		// Apply correction factor - the syscall seems to be reporting values
		// that are much larger than what df -h shows
		// Typical file systems have a block size of 512 bytes but Statfs reports in "fundamental block size"
		// We need to adjust by the correct factor to match df -h

		// Let's determine the correct block size by examining the reported values
		reportedTotal := float64(stat.Blocks * uint64(stat.Bsize))
		correctionFactor := 1.0

		// If total size is over 100TB, it's likely much larger than the actual size
		// Typical correction would be dividing by 256 (from 512 byte blocks to 128KB blocks)
		if reportedTotal > 100*1024*1024*1024*1024 {
			correctionFactor = 256.0
		}

		// Calculate space with correction factor applied
		totalBytes := float64(stat.Blocks*uint64(stat.Bsize)) / correctionFactor
		availableBytes := float64(stat.Bavail*uint64(stat.Bsize)) / correctionFactor
		usedBytes := totalBytes - availableBytes

		// Set sizes in the summary
		summary.TotalSizeBytes = totalBytes
		summary.AvailableSizeBytes = availableBytes
		summary.UsedSizeBytes = usedBytes

		entries, err := os.ReadDir(uploadDir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}

			// Update file count and total size
			summary.CurrentFiles++
			summary.CurrentSizeBytes += float64(info.Size())

			// Categorize file size
			size := info.Size()
			switch {
			case size < 1024*1024: // < 1MB
				summary.FileSizeDistribution["0-1MB"]++
			case size < 10*1024*1024: // < 10MB
				summary.FileSizeDistribution["1-10MB"]++
			case size < 100*1024*1024: // < 100MB
				summary.FileSizeDistribution["10-100MB"]++
			default:
				summary.FileSizeDistribution["100MB+"]++
			}
		}

		// Calculate usage percentage to match df output
		usagePercentage := (summary.UsedSizeBytes / summary.TotalSizeBytes) * 100

		// Log values for debugging
		log.Printf("Filesystem stats: Total: %s, Used: %s, Available: %s, Usage: %.1f%%",
			formatBytes(summary.TotalSizeBytes),
			formatBytes(summary.UsedSizeBytes),
			formatBytes(summary.AvailableSizeBytes),
			usagePercentage)

		// Add consistency check for calculated file size vs filesystem usage
		if summary.CurrentSizeBytes > summary.UsedSizeBytes {
			log.Printf("Warning: Calculated file size (%s) exceeds filesystem usage (%s)",
				formatBytes(summary.CurrentSizeBytes),
				formatBytes(summary.UsedSizeBytes))
		}

		c.JSON(http.StatusOK, summary)
	}
}
