package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/cleanup"
	"github.com/jonasbg/paste/m/v2/handlers"
	"github.com/jonasbg/paste/m/v2/middleware"
	"github.com/jonasbg/paste/m/v2/telemetry"
	"github.com/jonasbg/paste/m/v2/utils"
	"golang.org/x/time/rate"
)

const (
	requestsPerSecond = 60
	burstSize         = 120
)

func getUploadDir() string {
	if dir := os.Getenv("UPLOAD_DIR"); dir != "" {
		return dir
	}
	return "./uploads"
}

func main() {
	uploadDir := getUploadDir()
	if err := os.MkdirAll(uploadDir, 0750); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	telemetryProvider, err := telemetry.Init(context.Background())
	if err != nil {
		log.Fatalf("Failed to initialize telemetry: %v", err)
	}

	handlers.InitConfig()

	limiter := middleware.NewIPRateLimiter(rate.Limit(requestsPerSecond), burstSize)

	r := gin.New()
	r.SetTrustedProxies(utils.GetTrustedProxies())
	r.TrustedPlatform = "X-Forwarded-For"

	r.Use(middleware.PrivacyLogger(), gin.Recovery())
	r.Use(telemetryProvider.Middleware())

	// Add compression middleware with custom options
	r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedExtensions([]string{".pdf", ".mp4", ".avi", ".mov"}),
		// Exclude websocket endpoints and raw download endpoint (already encrypted/compressed data)
		gzip.WithExcludedPaths([]string{"/api/ws", "/api/download"})))

	api := r.Group("/api")
	api.Use(middleware.RateLimit(limiter))
	{
		api.GET("/config", handlers.GetConfig())
		api.GET("/metadata/:id", handlers.HandleMetadata(uploadDir))
		api.GET("/download/:id", handlers.HandleDownload(uploadDir))
		api.DELETE("/delete/:id", handlers.HandleDelete(uploadDir))

		api.GET("/ws/upload", handlers.HandleWSUpload(uploadDir, telemetryProvider))
		api.GET("/ws/download", handlers.HandleWSDownload(uploadDir, telemetryProvider))
	}

	if err := telemetry.MountPrometheusRoute(r, telemetryProvider.PrometheusHandler()); err != nil {
		log.Fatalf("Failed to mount telemetry endpoint: %v", err)
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

	cleanup.StartFileCleanup(uploadDir)

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		log.Println("Shutting down server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetryProvider.Shutdown(shutdownCtx); err != nil {
			log.Printf("Telemetry shutdown failed: %v", err)
		}
		log.Println("Server shutdown complete")
		os.Exit(0)
	}()

	log.Printf("Starting server on :8080 with upload directory: %s", uploadDir)
	log.Fatal(r.Run(":8080"))
}
