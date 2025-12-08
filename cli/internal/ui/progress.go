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

	// Build progress bar
	bar := "["
	for i := 0; i < pb.width; i++ {
		if i < filled {
			bar += "="
		} else if i == filled {
			bar += ">"
		} else {
			bar += " "
		}
	}
	bar += "]"

	// Calculate speed
	elapsed := time.Since(pb.startTime).Seconds()
	var speed float64
	if elapsed > 0 {
		speed = float64(pb.current) / elapsed
	}

	// Format sizes
	currentMB := float64(pb.current) / (1024 * 1024)
	totalMB := float64(pb.total) / (1024 * 1024)
	speedMB := speed / (1024 * 1024)

	// Spinner
	spinner := spinnerChars[pb.spinnerIdx]

	// Print progress bar
	fmt.Fprintf(os.Stderr, "\r%s %s %s %.1f%% (%.1f/%.1f MB, %.1f MB/s)   ",
		spinner, pb.description, bar, percentage, currentMB, totalMB, speedMB)
}
