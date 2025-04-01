package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
	"github.com/jonasbg/paste/m/v2/utils"
)

func HandleRequestMetrics(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Set default end time to today at 23:59:59
		now := time.Now().UTC()
		end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)
		start := end.AddDate(0, -1, 0) // Default to last month

		// Try to use range parameter first
		if rangeStr := c.Query("range"); rangeStr != "" {
			start, end = utils.ParseTimeRange(rangeStr)
		} else {
			// Fall back to explicit start/end parameters
			if startStr := c.Query("start"); startStr != "" {
				if parsedStart, err := time.Parse("2006-01-02", startStr); err == nil {
					start = parsedStart
				}
			}
			if endStr := c.Query("end"); endStr != "" {
				if parsedEnd, err := time.Parse("2006-01-02", endStr); err == nil {
					end = parsedEnd
				}
			}
		}

		metrics, err := database.GetRequestMetrics(start, end)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, metrics)
	}
}
