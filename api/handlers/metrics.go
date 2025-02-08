package handlers

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
	"github.com/jonasbg/paste/m/v2/types"
	"github.com/jonasbg/paste/m/v2/utils"
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

func HandleStorage(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		summary := types.StorageSummary{
			FileSizeDistribution: make(map[string]int),
		}

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
			summary.TotalFiles++
			summary.TotalSizeBytes += float64(info.Size())

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

		c.JSON(http.StatusOK, summary)
	}
}
