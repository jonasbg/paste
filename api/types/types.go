package types

type ActivitySummary struct {
	Period         string `json:"period"`
	Uploads        int64  `json:"uploads"`
	Downloads      int64  `json:"downloads"`
	UniqueVisitors int64  `json:"unique_visitors"`
}

type UploadHistoryItem struct {
	Date      string  `json:"date"`
	FileCount int     `json:"file_count"`
	TotalSize float64 `json:"total_size"` // in bytes
}

type StorageSummary struct {
	TotalSizeBytes       float64        `json:"total_size_bytes"`
	SystemTotalSizeBytes float64        `json:"system_total_size_bytes"`
	AvailableSizeBytes   float64        `json:"available_size_bytes"`
	CurrentSizeBytes     float64        `json:"current_size_bytes"`
	CurrentFiles         int            `json:"current_files"`
	FileSizeDistribution map[string]int `json:"file_size_distribution"`
	TotalFiles           int64          `json:"total_files"`
	UsedSizeBytes        float64        `json:"used_size_bytes"`
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
