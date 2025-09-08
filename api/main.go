package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gin-contrib/gzip"
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
	if err := os.MkdirAll(uploadDir, 0750); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	dbPath := getDatabaseDir()
	database, err := db.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	handlers.InitConfig()

	limiter := middleware.NewIPRateLimiter(rate.Limit(requestsPerSecond), burstSize)

	r := gin.New()
	r.SetTrustedProxies(utils.GetTrustedProxies())
	r.TrustedPlatform = "X-Forwarded-For"

	r.Use(gin.Logger(), gin.Recovery())

	// Add compression middleware with custom options
	r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedExtensions([]string{".pdf", ".mp4", ".avi", ".mov"}),
		// Exclude websocket endpoints and raw download endpoint (already encrypted/compressed data)
		gzip.WithExcludedPaths([]string{"/api/ws", "/api/download"})))

	r.Use(middleware.Logger(database))

	api := r.Group("/api")
	api.Use(middleware.RateLimit(limiter))
	{
		api.GET("/config", handlers.GetConfig())
		api.GET("/metadata/:id", handlers.HandleMetadata(uploadDir))
		api.GET("/download/:id", handlers.HandleDownload(uploadDir))
		api.DELETE("/delete/:id", handlers.HandleDelete(uploadDir))

		api.GET("/ws/upload", handlers.HandleWSUpload(uploadDir, database))
		api.GET("/ws/download", handlers.HandleWSDownload(uploadDir, database))
	}

	allowedMetricsIPs := utils.GetEnv("METRICS_ALLOWED_IPS", "127.0.0.1/8,::1/128")

	// Replace the metrics API group with this:
	metricsAPI := api.Group("")
	metricsAPI.Use(middleware.IPSourceRestriction(allowedMetricsIPs))
	{
		metricsAPI.GET("/metrics/activity", handlers.HandleActivity(database))
		metricsAPI.GET("/metrics/storage", handlers.HandleStorage(database, uploadDir))
		metricsAPI.GET("/metrics/requests", handlers.HandleRequestMetrics(database))
		metricsAPI.GET("/metrics/security", handlers.HandleSecurityMetrics(database))
		metricsAPI.GET("/metrics/upload-history", handlers.HandleUploadHistory(database))
	}

	spaDirectory := utils.GetEnv("WEB_DIR", "../web")
	spaDirectory = filepath.Clean(spaDirectory)

	if _, err := os.Stat(spaDirectory); os.IsNotExist(err) {
		log.Fatalf("Static files directory does not exist: %s", spaDirectory)
	}

	r.Use(middleware.CacheHeaders(spaDirectory))

	// Add custom WASM MIME type configuration
	r.GET("/encryption.wasm", func(c *gin.Context) {
		c.Header("Content-Type", "application/wasm")
		c.FileFromFS("encryption.wasm", gin.Dir(spaDirectory, false))
	})

	r.Use(middleware.Middleware("/", spaDirectory))

	cleanup.StartLogRotation(database)
	cleanup.StartFileCleanup(uploadDir)

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		log.Println("Shutting down server...")

		// Flush logs before exit
		middleware.CloseLogManager()
		log.Println("Logs flushed, shutting down")
		os.Exit(0)
	}()

	log.Printf("Starting server on :8080 with upload directory: %s", uploadDir)
	log.Fatal(r.Run(":8080"))
}
