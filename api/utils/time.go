package utils

import (
	"strconv"
	"strings"
	"time"
)

func ParseTimeRange(rangeStr string) (start time.Time, end time.Time) {
	end = time.Now().UTC()

	if rangeStr == "" {
		start = end.AddDate(0, 0, -7) // Default to 7 days
		return
	}

	// Parse duration format like "30d", "24h", "7d"
	num, err := strconv.Atoi(rangeStr[:len(rangeStr)-1])
	if err != nil {
		start = end.AddDate(0, 0, -7) // Default to 7 days on error
		return
	}

	unit := strings.ToLower(rangeStr[len(rangeStr)-1:])
	switch unit {
	case "h":
		start = end.Add(time.Duration(-num) * time.Hour)
	case "d":
		start = end.AddDate(0, 0, -num)
	case "w":
		start = end.AddDate(0, 0, -num*7)
	case "m":
		start = end.AddDate(0, -num, 0)
	case "y":
		start = end.AddDate(-num, 0, 0)
	default:
		start = end.AddDate(0, 0, -7)
	}

	return
}
