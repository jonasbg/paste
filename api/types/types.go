package types

type ActivitySummary struct {
	Period         string `json:"period"`
	Uploads        int64  `json:"uploads"`
	Downloads      int64  `json:"downloads"`
	UniqueVisitors int64  `json:"unique_visitors"`
}

type StorageSummary struct {
	TotalFiles           int64          `json:"total_files"`
	TotalSizeBytes       float64        `json:"total_size_bytes"`
	FileSizeDistribution map[string]int `json:"file_size_distribution"` // buckets like "0-1MB", "1-10MB", etc.
}

type SecurityMetrics struct {
	Period         string        `json:"period"`
	StatusCodes    map[int]int64 `json:"status_codes"`
	TotalRequests  int64         `json:"total_requests"`
	FailedRequests int64         `json:"failed_requests"`
	UniqueIPs      int64         `json:"unique_ips"`
	TopIPs         []IPStats     `json:"top_ips"`
	AverageLatency float64       `json:"average_latency"`
}

type IPStats struct {
	IP       string `json:"ip"`
	Requests int64  `json:"requests"`
	Failures int64  `json:"failures"`
}
