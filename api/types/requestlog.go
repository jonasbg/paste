package types

import "time"

type RequestLog struct {
	ID          uint      `gorm:"primarykey"`
	Timestamp   time.Time `gorm:"index;not null"`
	IP          string    `gorm:"type:varchar(45);index;not null"`
	Method      string    `gorm:"type:varchar(10);not null"`
	Path        string    `gorm:"type:text;index;not null"`
	StatusCode  int       `gorm:"index;not null"`
	Duration    int64     `gorm:"not null"`
	UserAgent   string    `gorm:"type:text"`
	BodySize    int64     `gorm:"not null"`
	Error       string    `gorm:"type:text"`
	QueryParams string    `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}
