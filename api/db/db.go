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

	return &DB{db: db}, nil
}

func (d *DB) GetSecurityMetrics(start, end time.Time) (types.SecurityMetrics, error) {
	var metrics types.SecurityMetrics
	metrics.StatusCodes = make(map[int]int64)

	// Get status code distribution
	var statusResults []struct {
		StatusCode int
		Count      int64
	}
	err := d.db.Model(&types.TransactionLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Group("status_code").
		Select("status_code, count(*) as count").
		Find(&statusResults).Error
	if err != nil {
		return metrics, err
	}

	for _, r := range statusResults {
		metrics.StatusCodes[r.StatusCode] = r.Count
		metrics.TotalRequests += r.Count
		if r.StatusCode >= 400 {
			metrics.FailedRequests += r.Count
		}
	}

	// Get unique IPs
	err = d.db.Model(&types.TransactionLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Distinct("ip").
		Count(&metrics.UniqueIPs).Error
	if err != nil {
		return metrics, err
	}

	// Get average latency
	var avgLatency sql.NullFloat64
	err = d.db.Model(&types.TransactionLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select("avg(duration) as avg_latency").
		Row().Scan(&avgLatency)
	if err != nil {
		return metrics, err
	}

	// Handle NULL case
	if avgLatency.Valid {
		metrics.AverageLatency = avgLatency.Float64
	} else {
		metrics.AverageLatency = 0 // or another appropriate default value
	}

	// Get top 10 IPs by request count
	err = d.db.Model(&types.TransactionLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Group("ip").
		Select("ip, count(*) as requests, sum(case when status_code >= 400 then 1 else 0 end) as failures").
		Order("requests desc").
		Limit(10).
		Find(&metrics.TopIPs).Error

	return metrics, err
}

func (d *DB) GetActivitySummary(start, end time.Time) ([]types.ActivitySummary, error) {
	if end.IsZero() {
		end = time.Now().UTC()
	}
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)

	if start.IsZero() {
		start = end.AddDate(0, -1, 0)
	}
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	var summary []types.ActivitySummary
	intervals := int((end.Sub(start).Hours() / 24) + 1)

	for i := intervals - 1; i >= 0; i-- {
		periodStart := start.AddDate(0, 0, i)
		periodEnd := periodStart.Add(24 * time.Hour).Add(-time.Nanosecond)

		var periodSummary types.ActivitySummary
		periodSummary.Period = periodStart.Format("2006-01-02")

		// Count successful uploads
		d.db.Model(&types.TransactionLog{}).
			Where("timestamp BETWEEN ? AND ? AND action = ? AND success = ?",
				periodStart, periodEnd, "upload", true).
			Count(&periodSummary.Uploads)

		// Count successful downloads
		d.db.Model(&types.TransactionLog{}).
			Where("timestamp BETWEEN ? AND ? AND action = ? AND success = ?",
				periodStart, periodEnd, "download", true).
			Count(&periodSummary.Downloads)

		// Count unique visitors (IPs) for the period
		d.db.Model(&types.TransactionLog{}).
			Where("timestamp BETWEEN ? AND ?", periodStart, periodEnd).
			Distinct("ip").
			Count(&periodSummary.UniqueVisitors)

		summary = append(summary, periodSummary)
	}

	return summary, nil
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

	// Get total requests
	if err := d.db.Model(&types.RequestLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Count(&metrics.TotalRequests).Error; err != nil {
		return metrics, err
	}

	// Get unique IPs
	if err := d.db.Model(&types.RequestLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Distinct("ip").
		Count(&metrics.UniqueIPs).Error; err != nil {
		return metrics, err
	}

	// Get average latency
	if err := d.db.Model(&types.RequestLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select("COALESCE(AVG(duration), 0)").
		Scan(&metrics.AverageLatency).Error; err != nil {
		return metrics, err
	}

	// Get status code distribution
	var statusResults []struct {
		StatusCode int
		Count      int64
	}
	if err := d.db.Model(&types.RequestLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select("status_code, COUNT(*) as count").
		Group("status_code").
		Scan(&statusResults).Error; err != nil {
		return metrics, err
	}
	for _, result := range statusResults {
		metrics.StatusDistribution[result.StatusCode] = result.Count
	}

	// Get path distribution (top 10 paths)
	var pathResults []struct {
		Path  string
		Count int64
	}
	if err := d.db.Model(&types.RequestLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select("path, COUNT(*) as count").
		Group("path").
		Order("count DESC").
		Limit(10).
		Scan(&pathResults).Error; err != nil {
		return metrics, err
	}
	for _, result := range pathResults {
		metrics.PathDistribution[result.Path] = result.Count
	}

	// Get top 10 IPs with their error counts
	if err := d.db.Model(&types.RequestLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select(`ip,
							COUNT(*) as request_count,
							COUNT(CASE WHEN status_code >= 400 THEN 1 END) as error_count`).
		Group("ip").
		Order("request_count DESC").
		Limit(10).
		Find(&metrics.TopIPs).Error; err != nil {
		return metrics, err
	}

	// Get time distribution (requests per day)
	// Modified to use strftime for SQLite date formatting
	if err := d.db.Model(&types.RequestLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select(`strftime('%Y-%m-%d', timestamp) as date, COUNT(*) as count`).
		Group("date").
		Order("date ASC").
		Find(&metrics.TimeDistribution).Error; err != nil {
		return metrics, err
	}

	return metrics, nil
}
