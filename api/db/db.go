package db

import (
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
	err = d.db.Model(&types.TransactionLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select("avg(duration) as avg_latency").
		Row().Scan(&metrics.AverageLatency)
	if err != nil {
		return metrics, err
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

func (d *DB) GetStorageSummary(start, end time.Time) (types.StorageSummary, error) {
	var summary types.StorageSummary
	now := time.Now().UTC()

	// Current files: Files uploaded minus files downloaded
	// We count files uploaded in the last 7 days that haven't been downloaded
	sevenDaysAgo := now.AddDate(0, 0, -7)

	uploadedFiles := d.db.Model(&types.TransactionLog{}).
		Where("action = ? AND success = ? AND timestamp > ?", "upload", true, sevenDaysAgo)

	// Get current files count
	var uploads []string
	uploadedFiles.Distinct("file_id").Pluck("file_id", &uploads)

	// Get downloaded files
	var downloads []string
	d.db.Model(&types.TransactionLog{}).
		Where("action = ? AND success = ? AND file_id IN ?", "download", true, uploads).
		Distinct("file_id").
		Pluck("file_id", &downloads)

	downloadMap := make(map[string]bool)
	for _, id := range downloads {
		downloadMap[id] = true
	}

	// Calculate current files and size
	summary.CurrentFiles = 0
	summary.CurrentSize = 0
	for _, id := range uploads {
		if !downloadMap[id] {
			summary.CurrentFiles++
			var size int64
			d.db.Model(&types.TransactionLog{}).
				Where("file_id = ? AND action = ?", id, "upload").
				Select("size").
				Row().
				Scan(&size)
			summary.CurrentSize += float64(size)
		}
	}

	// Get total unique visitors for the period
	d.db.Model(&types.TransactionLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Distinct("ip").
		Count(&summary.TotalUniqueVisitors)

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

func (d *DB) GetRequestMetrics(start, end time.Time) (map[string]interface{}, error) {
	var metrics = make(map[string]interface{})

	// Total requests
	var totalRequests int64
	if err := d.db.Model(&types.RequestLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Count(&totalRequests).Error; err != nil {
		return nil, err
	}
	metrics["total_requests"] = totalRequests

	// Average response time
	var avgDuration float64
	if err := d.db.Model(&types.RequestLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select("AVG(duration)").
		Row().Scan(&avgDuration); err != nil {
		return nil, err
	}
	metrics["avg_response_time_ms"] = avgDuration

	// Status code distribution
	var statusCodes []struct {
		StatusCode int
		Count      int64
	}
	if err := d.db.Model(&types.RequestLog{}).
		Where("timestamp BETWEEN ? AND ?", start, end).
		Select("status_code, COUNT(*) as count").
		Group("status_code").
		Find(&statusCodes).Error; err != nil {
		return nil, err
	}
	metrics["status_codes"] = statusCodes

	return metrics, nil
}
