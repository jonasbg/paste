package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/cleanup"
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

	limiter := middleware.NewIPRateLimiter(rate.Limit(requestsPerSecond), burstSize)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(middleware.Logger(database))

	api := r.Group("/api")
	api.Use(middleware.RateLimit(limiter))
	{
		api.POST("/upload", handlers.HandleUpload(uploadDir))
		api.GET("/metadata/:id", handlers.HandleMetadata(uploadDir))
		api.GET("/download/:id", handlers.HandleDownload(uploadDir))

		api.GET("/ws/upload", handlers.HandleWSUpload(uploadDir, database))

		api.GET("/metrics/activity", handlers.HandleActivity(database))
		api.GET("/metrics/storage", handlers.HandleStorage(uploadDir))
		api.GET("/metrics/requests", handlers.HandleRequestMetrics(database))
		api.GET("/metrics/security", handlers.HandleSecurityMetrics(database))
	}

	spaDirectory := utils.GetEnv("WEB_DIR", "../web")
	spaDirectory = filepath.Clean(spaDirectory)

	if _, err := os.Stat(spaDirectory); os.IsNotExist(err) {
		log.Fatalf("Static files directory does not exist: %s", spaDirectory)
	}

	r.Use(middleware.Middleware("/", spaDirectory))

	cleanup.StartLogRotation(database)
	cleanup.StartFileCleanup(uploadDir)

	log.Printf("Starting server on :8080 with upload directory: %s", uploadDir)
	log.Fatal(r.Run(":8080"))
}
