package types

import "time"

type RequestLog struct {
	ID          uint      `gorm:"primarykey"`
	Timestamp   time.Time `gorm:"index;not null"`
	IP          string    `gorm:"type:varchar(128);index;not null"`
	Method      string    `gorm:"type:varchar(10);not null"`
	StatusCode  int       `gorm:"index;not null"`
	Duration    int64     `gorm:"not null"`
	BodySize    int64     `gorm:"not null"`
	Error       string    `gorm:"type:text"`
	QueryParams string    `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

type TimeDistributionData struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type RequestMetrics struct {
	TotalRequests      int64                  `json:"total_requests"`
	UniqueIPs          int64                  `json:"unique_ips"`
	AverageLatency     float64                `json:"average_latency_ms"`
	StatusDistribution map[int]int64          `json:"status_distribution"`
	TopIPs             []TopIPMetrics         `json:"top_ips"`
	TimeDistribution   []TimeDistributionData `json:"time_distribution"`
}

type TopIPMetrics struct {
	IP           string `json:"ip"`
	RequestCount int64  `json:"request_count"`
	ErrorCount   int64  `json:"error_count"`
}
