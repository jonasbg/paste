package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
)

func HandleRequestMetrics(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse time range from query parameters with defaults
		end := time.Now().UTC()
		start := end.AddDate(0, -1, 0) // Default to last month

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

		metrics, err := database.GetRequestMetrics(start, end)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, metrics)
	}
}
