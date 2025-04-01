package db

import (
	"database/sql"
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
	metrics.TopIPs = make([]types.IPStats, 0) // Initialize slice

	baseQuery := d.db.Model(&types.TransactionLog{}).Where("timestamp BETWEEN ? AND ?", start, end)

	// 1. Get combined simple aggregates (Total, Failed, Avg Latency)
	var simpleAggregates struct {
		TotalRequests  int64           // COUNT(*) returns 0 for no rows, so it's safe as int64
		FailedRequests int64           // Will receive 0 from COALESCE if SUM is NULL
		AvgLatency     sql.NullFloat64 // Keep using sql.NullFloat64 for AVG
	}

	err := baseQuery.Select(`
			COUNT(*) as total_requests,
			COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END), 0) as failed_requests,
			AVG(duration) as avg_latency
	`).Row().Scan(
		&simpleAggregates.TotalRequests,
		&simpleAggregates.FailedRequests, // Now scanning into int64 is safe
		&simpleAggregates.AvgLatency,
	)
	if err != nil {
		return metrics, err // Return other errors
	}

	metrics.TotalRequests = simpleAggregates.TotalRequests
	metrics.FailedRequests = simpleAggregates.FailedRequests // Direct assignment is now safe
	if simpleAggregates.AvgLatency.Valid {
		metrics.AverageLatency = simpleAggregates.AvgLatency.Float64
	} else {
		metrics.AverageLatency = 0 // Handle NULL average latency
	}

	var statusResults []struct {
		StatusCode int
		Count      int64
	}
	err = d.db.Model(&types.TransactionLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Group("status_code").
		Select("status_code, count(*) as count").
		Find(&statusResults).Error
	if err != nil {
		return metrics, err
	}
	for _, r := range statusResults {
		metrics.StatusCodes[r.StatusCode] = r.Count
	}

	var uniqueIPCount int64
	err = d.db.Model(&types.TransactionLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select("COUNT(DISTINCT ip)").
		Row().Scan(&uniqueIPCount)
	// Handle potential error during scan (though COUNT should return 0 if no rows)
	if err != nil && err != sql.ErrNoRows {
		return metrics, err
	}
	metrics.UniqueIPs = uniqueIPCount

	// 4. Get top 10 IPs
	err = d.db.Model(&types.TransactionLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Group("ip").
		// Add COALESCE here too for consistency, though count(*) should be okay
		Select("ip, count(*) as requests, COALESCE(sum(case when status_code >= 400 then 1 else 0 end), 0) as failures").
		Order("requests desc").
		Limit(10).
		Find(&metrics.TopIPs).Error // Ensure types.IPStats struct fields match 'ip', 'requests', 'failures'
	if err != nil {
		return metrics, err
	}

	return metrics, nil // return nil error on success
}

func (d *DB) GetActivitySummary(start, end time.Time) ([]types.ActivitySummary, error) {
	// Ensure start and end cover full days midnight to midnight UTC
	// (Your existing logic for this seems okay, but double-check application needs)
	if end.IsZero() {
		end = time.Now().UTC()
	}
	year, month, day := end.Date()
	end = time.Date(year, month, day, 23, 59, 59, 999999999, time.UTC) // End of the 'end' day

	if start.IsZero() {
		// Default to 30 days before end date? Adjust as needed.
		start = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -29)
	} else {
		year, month, day := start.Date()
		start = time.Date(year, month, day, 0, 0, 0, 0, time.UTC) // Start of the 'start' day
	}

	var dailyStats []struct {
		DateStr        string `gorm:"column:date_str"`
		Uploads        int64
		Downloads      int64
		UniqueVisitors int64
	}

	// Single query to get all stats grouped by day
	err := d.db.Model(&types.TransactionLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select(`
					STRFTIME('%Y-%m-%d', timestamp) as date_str,
					SUM(CASE WHEN action = 'upload' AND success = 1 THEN 1 ELSE 0 END) as uploads,
					SUM(CASE WHEN action = 'download' AND success = 1 THEN 1 ELSE 0 END) as downloads,
					COUNT(DISTINCT ip) as unique_visitors
			`).
		Group("date_str").     // Group by the formatted date string
		Order("date_str ASC"). // Order chronologically
		Find(&dailyStats).Error

	if err != nil {
		return nil, err
	}

	// Process results and fill in missing days with zeros
	summaryMap := make(map[string]types.ActivitySummary)
	for _, stat := range dailyStats {
		summaryMap[stat.DateStr] = types.ActivitySummary{
			Period:         stat.DateStr,
			Uploads:        stat.Uploads,
			Downloads:      stat.Downloads,
			UniqueVisitors: stat.UniqueVisitors,
		}
	}

	var finalSummary []types.ActivitySummary
	// Iterate through the date range day by day
	currentDate := start
	// Loop condition needs to include the end date
	for !currentDate.After(end) {
		dateStr := currentDate.Format("2006-01-02")
		summary, found := summaryMap[dateStr]
		if found {
			finalSummary = append(finalSummary, summary)
		} else {
			// Add a zero entry for days with no activity
			finalSummary = append(finalSummary, types.ActivitySummary{Period: dateStr})
		}
		currentDate = currentDate.AddDate(0, 0, 1) // Move to the next day
	}

	// Optional: Reverse the slice if you need newest-first presentation
	// for i, j := 0, len(finalSummary)-1; i < j; i, j = i+1, j-1 {
	//     finalSummary[i], finalSummary[j] = finalSummary[j], finalSummary[i]
	// }

	return finalSummary, nil
}

func (d *DB) GetUploadHistory(start, end time.Time) ([]types.UploadHistoryItem, error) {
	var results []types.UploadHistoryItem

	// SQL to aggregate uploads by day
	// Use different date formatting approach to ensure compatibility
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
	var result struct {
		TotalFiles     int64
		TotalSizeBytes sql.NullInt64 // Use sql.NullInt64 for SUM which can be NULL if no rows match
	}

	// Single query for both count and sum
	err := d.db.Model(&types.TransactionLog{}).
		Where("action = ? AND success = ?", "upload", true).
		Select("COUNT(*) as total_files, SUM(size) as total_size_bytes"). // COALESCE not strictly needed if handling NullInt64
		Row().
		Scan(&result.TotalFiles, &result.TotalSizeBytes)

	// Check for error, but allow sql.ErrNoRows for aggregates (will result in 0/NULL)
	if err != nil && err != sql.ErrNoRows {
		return summary, err
	}

	summary.TotalFiles = result.TotalFiles
	if result.TotalSizeBytes.Valid {
		summary.TotalSizeBytes = float64(result.TotalSizeBytes.Int64)
	} else {
		summary.TotalSizeBytes = 0 // Default to 0 if SUM was NULL (no matching rows)
	}

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
	metrics.TopIPs = make([]types.TopIPMetrics, 0)                   // Initialize slice
	metrics.TimeDistribution = make([]types.TimeDistributionData, 0) // Initialize slice

	baseQuery := d.db.Model(&types.RequestLog{}).Where("timestamp BETWEEN ? AND ?", start, end)

	// 1. Combined simple aggregates (Total, Avg Latency)
	var simpleAggregates struct {
		TotalRequests  int64
		AverageLatency float64 // GORM Scan handles COALESCE(..., 0) directly to float64
	}
	// Clone baseQuery to avoid modifying it if you reuse it later
	err := baseQuery.Session(&gorm.Session{}). // Create a new session based on baseQuery
							Select("COUNT(*) as total_requests, COALESCE(AVG(duration), 0) as average_latency").
							Row().Scan(&simpleAggregates.TotalRequests, &simpleAggregates.AverageLatency)
	if err != nil && err != sql.ErrNoRows {
		return metrics, err
	}
	metrics.TotalRequests = simpleAggregates.TotalRequests
	metrics.AverageLatency = simpleAggregates.AverageLatency

	// 2. Get unique IPs (Requires DISTINCT) - Keep separate
	// Clone or create new query
	err = d.db.Model(&types.RequestLog{}).Where("timestamp BETWEEN ? AND ?", start, end).
		Distinct("ip").Count(&metrics.UniqueIPs).Error
	if err != nil {
		return metrics, err
	}

	// 3. Get status code distribution (Requires GROUP BY status_code) - Keep separate
	var statusResults []struct {
		StatusCode int
		Count      int64
	}
	err = d.db.Model(&types.RequestLog{}).Where("timestamp BETWEEN ? AND ?", start, end).
		Select("status_code, COUNT(*) as count").Group("status_code").Scan(&statusResults).Error
	if err != nil {
		return metrics, err
	}
	for _, result := range statusResults {
		metrics.StatusDistribution[result.StatusCode] = result.Count
	}

	// 4. Get path distribution (Requires GROUP BY path) - Keep separate
	var pathResults []struct {
		Path  string
		Count int64
	}
	err = d.db.Model(&types.RequestLog{}).Where("timestamp BETWEEN ? AND ?", start, end).
		Select("path, COUNT(*) as count").Group("path").Order("count DESC").Limit(10).Scan(&pathResults).Error
	if err != nil {
		return metrics, err
	}
	for _, result := range pathResults {
		metrics.PathDistribution[result.Path] = result.Count
	}

	// 5. Get top 10 IPs (Requires GROUP BY ip) - Keep separate
	// Using SUM(CASE...) for clarity/portability
	err = d.db.Model(&types.RequestLog{}).Where("timestamp BETWEEN ? AND ?", start, end).
		Select(`ip, COUNT(*) as request_count, SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as error_count`).
		Group("ip").Order("request_count DESC").Limit(10).Find(&metrics.TopIPs).Error
	if err != nil {
		return metrics, err
	}

	// 6. Get time distribution (Requires GROUP BY date) - Keep separate
	// Use appropriate struct tags for gorm.Find if types.TimePoint fields don't match select aliases
	err = d.db.Model(&types.RequestLog{}).Where("timestamp BETWEEN ? AND ?", start, end).
		Select(`strftime('%Y-%m-%d', timestamp) as date, COUNT(*) as count`).
		Group("date").Order("date ASC").Find(&metrics.TimeDistribution).Error
	if err != nil {
		return metrics, err
	}

	return metrics, nil
}
