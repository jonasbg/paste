package main

import (
	"context"
	"errors"
	"log"
	"net/http"
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

	// Tighter per-IP rate limit specifically for WebSocket upload initiation
	// to throttle passphrase-ID probing and reservation abuse.
	uploadInitInterval = 2 * time.Second
	uploadInitBurst    = 10
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

	if err := handlers.InitConfig(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	limiter := middleware.NewIPRateLimiter(rate.Limit(requestsPerSecond), burstSize)
	uploadInitLimiter := middleware.NewIPRateLimiter(rate.Every(uploadInitInterval), uploadInitBurst)
	uploadLimiter := handlers.NewUploadLimiter(uploadDir)
	downloadStore := handlers.NewDownloadStore()

	r := gin.New()
	// Forwarded headers are only honoured when the direct peer is a trusted
	// proxy. Do not set r.TrustedPlatform: Gin returns that header verbatim
	// from any client, which lets callers spoof their IP and bypass rate limits.
	if err := r.SetTrustedProxies(utils.GetTrustedProxies()); err != nil {
		log.Fatalf("Invalid TRUSTED_PROXIES: %v", err)
	}

	r.Use(middleware.PrivacyLogger(), gin.Recovery())
	r.Use(middleware.UntrustedProxyWarning(utils.GetTrustedProxies()))
	r.Use(middleware.SecurityHeaders())
	r.Use(telemetryProvider.Middleware())

	// Add compression middleware with custom options
	r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedExtensions([]string{".pdf", ".mp4", ".avi", ".mov"}),
		// Exclude websocket endpoints and raw download endpoint (already encrypted/compressed data)
		gzip.WithExcludedPaths([]string{"/api/ws", "/api/download"})))

	api := r.Group("/api")
	api.Use(middleware.APINoStore(), middleware.RateLimit(limiter))
	{
		api.GET("/config", handlers.GetConfig())
		api.GET("/metadata/:id", handlers.HandleMetadata(uploadDir, downloadStore))
		api.GET("/download/:id", handlers.HandleDownload(uploadDir, downloadStore, uploadLimiter))
		api.DELETE("/delete/:id", handlers.HandleDelete(uploadDir, downloadStore, uploadLimiter))

		api.GET("/ws/upload",
			middleware.RateLimit(uploadInitLimiter),
			handlers.HandleWSUpload(uploadDir, telemetryProvider, uploadLimiter, downloadStore),
		)
	}

	metricsSrv, err := telemetry.StartMetricsServer(telemetryProvider.PrometheusHandler())
	if err != nil {
		log.Fatalf("Failed to initialize metrics server: %v", err)
	}
	if metricsSrv != nil {
		go func() {
			log.Printf("Starting metrics server on %s", metricsSrv.Addr)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("Metrics server stopped: %v", err)
			}
		}()
	} else {
		if err := telemetry.MountPrometheusRoute(r, telemetryProvider.PrometheusHandler()); err != nil {
			log.Fatalf("Failed to mount telemetry endpoint: %v", err)
		}
	}

	spaDirectory := utils.GetEnv("WEB_DIR", "../web")
	spaDirectory = filepath.Clean(spaDirectory)

	if _, err := os.Stat(spaDirectory); os.IsNotExist(err) {
		log.Fatalf("Static files directory does not exist: %s", spaDirectory)
	}

	r.Use(middleware.CacheHeaders(spaDirectory))

	// Explicit WASM MIME type for /encryption.wasm
	r.GET("/encryption.wasm", func(c *gin.Context) {
		c.Header("Content-Type", "application/wasm")
		c.FileFromFS("encryption.wasm", gin.Dir(spaDirectory, false))
	})

	r.Use(middleware.Middleware("/", spaDirectory))

	cleanup.StartFileCleanup(uploadDir, uploadLimiter.SyncStoredBytes)

	addr := utils.GetEnv("LISTEN_ADDR", ":8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down server gracefully...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown failed: %v", err)
		}
		if metricsSrv != nil {
			if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
				log.Printf("Metrics server shutdown failed: %v", err)
			}
		}
		if err := telemetryProvider.Shutdown(shutdownCtx); err != nil {
			log.Printf("Telemetry shutdown failed: %v", err)
		}
		log.Println("Server shutdown complete")
	}()

	log.Printf("Starting server on %s with upload directory: %s (web: %s)", addr, uploadDir, spaDirectory)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Server failed: %v", err)
	}
}
