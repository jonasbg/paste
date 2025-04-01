package db

import (
	"sort"
	"time"

	"github.com/jonasbg/paste/m/v2/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	db *gorm.DB
}

func NewDB(dbPath string) (*DB, error) {
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	db, err := gorm.Open(sqlite.Open(dbPath), config)
	if err != nil {
		return nil, err
	}

	db.Exec("PRAGMA journal_mode = WAL")
	db.Exec("PRAGMA synchronous = NORMAL")
	db.Exec("PRAGMA cache_size = 8000")
	db.Exec("PRAGMA temp_store = MEMORY")
	db.Exec("PRAGMA mmap_size = 30000000000")

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	err = db.AutoMigrate(&types.TransactionLog{}, &types.RequestLog{})
	if err != nil {
		return nil, err
	}

	db.Exec("CREATE INDEX IF NOT EXISTS idx_txlog_timestamp ON transaction_logs (timestamp);")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_txlog_action_success_ts ON transaction_logs (action, success, timestamp);")

	db.Exec("CREATE INDEX IF NOT EXISTS idx_reqlog_timestamp ON request_logs (timestamp);")


	return &DB{db: db}, nil
}

func (d *DB) GetSecurityMetrics(start, end time.Time) (types.SecurityMetrics, error) {
	var metrics types.SecurityMetrics
	metrics.StatusCodes = make(map[int]int64)

	// Get all transaction logs for the period with a single query
	var logs []types.TransactionLog
	if err := d.db.Where("timestamp BETWEEN ? AND ?", start, end).Find(&logs).Error; err != nil {
		return metrics, err
	}

	// Process everything in memory
	ipSet := make(map[string]struct{})
	ipCounts := make(map[string]struct {
		Requests int64
		Failures int64
	})

	var totalDuration float64

	for _, log := range logs {
		// Track status codes
		metrics.StatusCodes[log.StatusCode]++
		metrics.TotalRequests++

		if log.StatusCode >= 400 {
			metrics.FailedRequests++
		}

		// Track unique IPs
		ipSet[log.IP] = struct{}{}

		// Track IP stats
		stats := ipCounts[log.IP]
		stats.Requests++
		if log.StatusCode >= 400 {
			stats.Failures++
		}
		ipCounts[log.IP] = stats

		// Sum durations
		totalDuration += float64(log.Duration)
	}

	// Calculate metrics
	metrics.UniqueIPs = int64(len(ipSet))
	if metrics.TotalRequests > 0 {
		metrics.AverageLatency = totalDuration / float64(metrics.TotalRequests)
	}

	// Build top IPs list
	for ip, stats := range ipCounts {
		metrics.TopIPs = append(metrics.TopIPs, types.TopIPMetrics{
			IP:           ip,
			RequestCount: stats.Requests,
			ErrorCount:   stats.Failures,
		})
	}

	// Sort and limit top IPs
	sort.Slice(metrics.TopIPs, func(i, j int) bool {
		return metrics.TopIPs[i].RequestCount > metrics.TopIPs[j].RequestCount
	})
	if len(metrics.TopIPs) > 10 {
		metrics.TopIPs = metrics.TopIPs[:10]
	}

	return metrics, nil
}

func (d *DB) GetActivitySummary(start, end time.Time) ([]types.ActivitySummary, error) {
	if end.IsZero() {
		end = time.Now().UTC()
	}
	// Set end to the end of the day (23:59:59.999999999)
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 999999999, time.UTC)

	if start.IsZero() {
		start = end.AddDate(0, -1, 0)
	}
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	// Get all logs for the period in a single query
	var logs []types.TransactionLog
	if err := d.db.Where("timestamp BETWEEN ? AND ?", start, end).Find(&logs).Error; err != nil {
		return nil, err
	}

	// Process in memory
	dayMap := make(map[string]*types.ActivitySummary)
	intervals := int((end.Sub(start).Hours() / 24) + 1)

	// Initialize all days
	for i := 0; i < intervals; i++ {
		day := start.AddDate(0, 0, i)
		dayStr := day.Format("2006-01-02")
		dayMap[dayStr] = &types.ActivitySummary{
			Period: dayStr,
		}
	}

	// Process logs
	ipsByDay := make(map[string]map[string]struct{})
	for _, log := range logs {
		day := log.Timestamp.Format("2006-01-02")
		summary, exists := dayMap[day]
		if !exists {
			continue // Skip if outside our range
		}

		// Track unique IPs
		if ipsByDay[day] == nil {
			ipsByDay[day] = make(map[string]struct{})
		}
		ipsByDay[day][log.IP] = struct{}{}

		// Count uploads and downloads
		if log.Action == "upload" && log.Success {
			summary.Uploads++
		} else if log.Action == "download" && log.Success {
			summary.Downloads++
		}
	}

	// Count unique visitors
	for day, ips := range ipsByDay {
		if summary, exists := dayMap[day]; exists {
			summary.UniqueVisitors = int64(len(ips))
		}
	}

	// Convert map to slice in order
	result := make([]types.ActivitySummary, 0, intervals)
	for i := intervals - 1; i >= 0; i-- {
		day := start.AddDate(0, 0, i)
		dayStr := day.Format("2006-01-02")
		if summary, exists := dayMap[dayStr]; exists {
			result = append(result, *summary)
		}
	}

	return result, nil
}

func (d *DB) GetUploadHistory(start, end time.Time) ([]types.UploadHistoryItem, error) {
	var results []types.UploadHistoryItem

	rows, err := d.db.Raw(`
		SELECT
			STRFTIME('%Y-%m-%d', timestamp) AS date_str,
			COUNT(*) AS file_count,
			COALESCE(SUM(size), 0) AS total_size
		FROM
			transaction_logs
		WHERE
			action = ?
			AND timestamp BETWEEN ? AND ?
			AND success = ?
		GROUP BY
			date_str
		ORDER BY
			date_str ASC
	`, "upload", start, end, true).Rows()

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item types.UploadHistoryItem
		var dateStr string
		if err := rows.Scan(&dateStr, &item.FileCount, &item.TotalSize); err != nil {
			return nil, err
		}
		item.Date = dateStr
		results = append(results, item)
	}

	return results, nil
}

func (d *DB) GetStorageSummary() (types.StorageSummary, error) {
	var summary types.StorageSummary

	// Get total files (successful uploads)
	d.db.Model(&types.TransactionLog{}).
		Where("action = ? AND success = ?", "upload", true).
		Count(&summary.TotalFiles)

	// Get total size of all files ever uploaded
	d.db.Model(&types.TransactionLog{}).
		Where("action = ? AND success = ?", "upload", true).
		Select("COALESCE(SUM(size), 0)").
		Row().
		Scan(&summary.TotalSizeBytes)

	return summary, nil
}

func (d *DB) LogTransaction(tx *types.TransactionLog) error {
	return d.db.Create(tx).Error
}

func (d *DB) CleanOldLogs(before time.Time) error {
	return d.db.Where("timestamp < ?", before).Delete(&types.TransactionLog{}).Error
}

func (d *DB) LogRequest(log *types.RequestLog) error {
	return d.db.Create(log).Error
}

func (d *DB) CleanOldRequestLogs(before time.Time) error {
	return d.db.Where("timestamp < ?", before).Delete(&types.RequestLog{}).Error
}

func (d *DB) GetRequestMetrics(start, end time.Time) (types.RequestMetrics, error) {
	var metrics types.RequestMetrics
	metrics.StatusDistribution = make(map[int]int64)
	metrics.PathDistribution = make(map[string]int64)

	// Get all request logs for the time period in a single query
	var logs []types.RequestLog
	if err := d.db.Where("timestamp BETWEEN ? AND ?", start, end).Find(&logs).Error; err != nil {
		return metrics, err
	}

	// Calculate metrics in memory
	ipSet := make(map[string]struct{})
	pathCounts := make(map[string]int64)
	statusCounts := make(map[int]int64)
	dailyRequests := make(map[string]int64)
	ipStats := make(map[string]struct {
		Requests int64
		Errors   int64
	})

	var totalDuration float64
	// Pre-estimate the size of various collections
	estimatedSize := len(logs)

	// Pre-allocate slices with estimated capacity
	metrics.TopIPs = make([]types.TopIPMetrics, 0, min(estimatedSize, 10))
	metrics.TimeDistribution = make([]types.TimeDistributionData, 0, min(31, estimatedSize)) // Assume max 31 days

	for _, log := range logs {
		metrics.TotalRequests++
		ipSet[log.IP] = struct{}{}
		totalDuration += float64(log.Duration)

		// Count status codes
		statusCounts[log.StatusCode]++

		// Count paths
		pathCounts[log.Path]++

		// Track IP stats
		stats := ipStats[log.IP]
		stats.Requests++
		if log.StatusCode >= 400 {
			stats.Errors++
		}
		ipStats[log.IP] = stats

		// Track requests by day
		day := log.Timestamp.Format("2006-01-02")
		dailyRequests[day]++
	}

	// Set calculated metrics
	metrics.UniqueIPs = int64(len(ipSet))
	if metrics.TotalRequests > 0 {
		metrics.AverageLatency = totalDuration / float64(metrics.TotalRequests)
	}

	// Set status distribution
	metrics.StatusDistribution = statusCounts

	// Better approach for path distribution: sort all paths by count
	type pathCount struct {
		Path  string
		Count int64
	}
	pathsList := make([]pathCount, 0, len(pathCounts))
	for path, count := range pathCounts {
		pathsList = append(pathsList, pathCount{Path: path, Count: count})
	}

	// Sort by count in descending order
	sort.Slice(pathsList, func(i, j int) bool {
		return pathsList[i].Count > pathsList[j].Count
	})

	// Take only top 10
	for i := 0; i < len(pathsList) && i < 10; i++ {
		metrics.PathDistribution[pathsList[i].Path] = pathsList[i].Count
	}

	// Set top IPs - pre-allocate a reasonable size for the slice
	for ip, stats := range ipStats {
		metrics.TopIPs = append(metrics.TopIPs, types.TopIPMetrics{
			IP:           ip,
			RequestCount: stats.Requests,
			ErrorCount:   stats.Errors,
		})
	}

	// Sort TopIPs by request count descending
	sort.Slice(metrics.TopIPs, func(i, j int) bool {
		return metrics.TopIPs[i].RequestCount > metrics.TopIPs[j].RequestCount
	})
	if len(metrics.TopIPs) > 10 {
		metrics.TopIPs = metrics.TopIPs[:10]
	}

	// Set time distribution
	metrics.TimeDistribution = make([]types.TimeDistributionData, 0, len(dailyRequests))
	for date, count := range dailyRequests {
		metrics.TimeDistribution = append(metrics.TimeDistribution, types.TimeDistributionData{
			Date:  date,
			Count: count,
		})
	}

	// Sort time distribution by date
	sort.Slice(metrics.TimeDistribution, func(i, j int) bool {
		return metrics.TimeDistribution[i].Date < metrics.TimeDistribution[j].Date
	})

	return metrics, nil
}

// Helper function for min of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
