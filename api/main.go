package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/db"
	"github.com/jonasbg/paste/m/v2/handlers"
	"github.com/jonasbg/paste/m/v2/middleware"
	"github.com/jonasbg/paste/m/v2/utils"
	"golang.org/x/time/rate"
)

const (
	requestsPerSecond = 10
	burstSize         = 20
)

func getUploadDir() string {
	if dir := os.Getenv("UPLOAD_DIR"); dir != "" {
		return dir
	}
	return "./uploads"
}

func getDatabaseDir() string {
	if dir := os.Getenv("DATABASE_DIR"); dir != "" {
		return filepath.Join(dir, "paste.db")
	}
	return filepath.Join("./uploads", "paste.db")
}

func startLogRotation(db *db.DB) {
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for {
			select {
			case <-ticker.C:
				// Keep logs for 180 days
				cutoff := time.Now().AddDate(0, 0, -180)
				if err := db.CleanOldLogs(cutoff); err != nil {
					log.Printf("Failed to clean old logs: %v", err)
				}
			}
		}
	}()
}

func main() {
	uploadDir := getUploadDir()
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	dbPath := getDatabaseDir()
	database, err := db.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	startLogRotation(database)

	limiter := middleware.NewIPRateLimiter(rate.Limit(requestsPerSecond), burstSize)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	api := r.Group("/api")
	api.Use(middleware.RateLimit(limiter), middleware.Logger(database))
	{
		api.POST("/upload", handlers.HandleUpload(uploadDir))
		api.GET("/metadata/:id", handlers.HandleMetadata(uploadDir))
		api.GET("/download/:id", handlers.HandleDownload(uploadDir))

		api.GET("/metrics/activity", handlers.HandleActivity(database))
		api.GET("/metrics/storage", handlers.HandleStorage(database))

		api.GET("/metrics/security", handlers.HandleSecurityMetrics(database))
	}

	spaDirectory := utils.GetEnv("WEB_DIR", "../web")
	spaDirectory = filepath.Clean(spaDirectory)

	if _, err := os.Stat(spaDirectory); os.IsNotExist(err) {
		log.Fatalf("Static files directory does not exist: %s", spaDirectory)
	}

	r.Use(middleware.Middleware("/", spaDirectory))

	r.MaxMultipartMemory = 8 << 20

	log.Printf("Starting server on :8080 with upload directory: %s", uploadDir)
	log.Fatal(r.Run(":8080"))
}
