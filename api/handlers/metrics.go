package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
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

func HandleStorage(db *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var start, end time.Time

		if rangeStr := c.Query("range"); rangeStr != "" {
			start, end = utils.ParseTimeRange(rangeStr)
		} else {
			start, _ = time.Parse(time.RFC3339, c.Query("start"))
			end, _ = time.Parse(time.RFC3339, c.Query("end"))
		}

		summary, err := db.GetStorageSummary(start, end)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, summary)
	}
}
