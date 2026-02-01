package ui

import (
	"fmt"
	"os"
	"time"
)

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ProgressBar represents a simple terminal progress bar
type ProgressBar struct {
	total       int64
	current     int64
	width       int
	description string
	startTime   time.Time
	lastUpdate  time.Time
	spinnerIdx  int
}

// NewProgressBar creates a new progress bar
func NewProgressBar(total int64, description string) *ProgressBar {
	return &ProgressBar{
		total:       total,
		current:     0,
		width:       40,
		description: description,
		startTime:   time.Now(),
		lastUpdate:  time.Now(),
		spinnerIdx:  0,
	}
}

// Update updates the progress bar
func (pb *ProgressBar) Update(current int64) {
	pb.current = current

	// Throttle updates to every 100ms
	now := time.Now()
	if now.Sub(pb.lastUpdate) < 100*time.Millisecond && current < pb.total {
		return
	}
	pb.lastUpdate = now
	pb.spinnerIdx = (pb.spinnerIdx + 1) % len(spinnerChars)

	pb.render()
}

// Finish completes the progress bar
func (pb *ProgressBar) Finish() {
	pb.current = pb.total
	pb.render()
	fmt.Fprint(os.Stderr, "\n")
}

func (pb *ProgressBar) render() {
	percentage := float64(pb.current) / float64(pb.total) * 100
	filled := int(float64(pb.width) * float64(pb.current) / float64(pb.total))

	// Build progress bar with fancy block characters
	bar := ""
	for i := 0; i < pb.width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	// Calculate speed
	elapsed := time.Since(pb.startTime).Seconds()
	var speed float64
	if elapsed > 0 {
		speed = float64(pb.current) / elapsed
	}

	// Calculate ETA
	var etaStr string
	if speed > 0 && pb.current < pb.total {
		remaining := pb.total - pb.current
		etaSeconds := float64(remaining) / speed
		if etaSeconds < 60 {
			etaStr = fmt.Sprintf("%.0fs", etaSeconds)
		} else {
			etaStr = fmt.Sprintf("%.0fm%.0fs", etaSeconds/60, float64(int(etaSeconds)%60))
		}
	}

	// Format sizes and speed based on file size
	if pb.total < 1024*1024 {
		// Use KB for files smaller than 1 MB
		currentKB := float64(pb.current) / 1024
		totalKB := float64(pb.total) / 1024
		speedKB := speed / 1024

		// If speed is >= 1 MB/s, display in MB/s for readability
		if speedKB >= 1024 {
			speedMB := speed / (1024 * 1024)
			if etaStr != "" {
				fmt.Fprintf(os.Stderr, "\r%.2f KB / %.2f KB %s %6.2f%% %.2f MB/s %s   ",
					currentKB, totalKB, bar, percentage, speedMB, etaStr)
			} else {
				fmt.Fprintf(os.Stderr, "\r%.2f KB / %.2f KB %s %6.2f%% %.2f MB/s   ",
					currentKB, totalKB, bar, percentage, speedMB)
			}
		} else {
			if etaStr != "" {
				fmt.Fprintf(os.Stderr, "\r%.2f KB / %.2f KB %s %6.2f%% %.2f KB/s %s   ",
					currentKB, totalKB, bar, percentage, speedKB, etaStr)
			} else {
				fmt.Fprintf(os.Stderr, "\r%.2f KB / %.2f KB %s %6.2f%% %.2f KB/s   ",
					currentKB, totalKB, bar, percentage, speedKB)
			}
		}
	} else {
		// Use MB for files 1 MB or larger
		currentMB := float64(pb.current) / (1024 * 1024)
		totalMB := float64(pb.total) / (1024 * 1024)
		speedMB := speed / (1024 * 1024)

		if etaStr != "" {
			fmt.Fprintf(os.Stderr, "\r%.2f MB / %.2f MB %s %6.2f%% %.2f MB/s %s   ",
				currentMB, totalMB, bar, percentage, speedMB, etaStr)
		} else {
			fmt.Fprintf(os.Stderr, "\r%.2f MB / %.2f MB %s %6.2f%% %.2f MB/s   ",
				currentMB, totalMB, bar, percentage, speedMB)
		}
	}
}
