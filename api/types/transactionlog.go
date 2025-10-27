package types

import "time"

type TransactionLog struct {
	ID         uint      `gorm:"primaryKey"`
	Timestamp  time.Time `gorm:"index"`
	Action     string    `gorm:"index"` // "upload", "download", "delete"
	FileID     string    `gorm:"index"`
	IP         string    `gorm:"type:varchar(128);index"`
	Size       int64
	Success    bool   // Whether the operation succeeded
	StatusCode int    // HTTP status code
	Error      string // Any error message if failed
	Duration   int64  // Request duration in milliseconds
	Method     string
}
