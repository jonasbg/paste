package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxFileSize = 5 * 1024 * 1024 * 1024 // 5GB
)

// Logger colors
const (
	green   = "\033[32m"
	white   = "\033[37m"
	yellow  = "\033[33m"
	red     = "\033[31m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	reset   = "\033[0m"
)

// getUploadDir returns the upload directory from environment variable or default
func getUploadDir() string {
	if dir := os.Getenv("UPLOAD_DIR"); dir != "" {
		return dir
	}
	return "./uploads"
}

// getClientIP attempts to get the real client IP, falling back through various headers
func getClientIP(r *http.Request) string {
	// Try X-Real-IP first
	ip := r.Header.Get("X-Real-IP")
	if ip != "" && ip != "unknown" && !strings.HasPrefix(ip, "127.") && !strings.HasPrefix(ip, "::1") {
		return ip
	}

	// Try X-Forwarded-For
	ip = r.Header.Get("X-Forwarded-For")
	if ip != "" {
		// X-Forwarded-For can be a comma-separated list; take the first non-local address
		parts := strings.Split(ip, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" && !strings.HasPrefix(trimmed, "127.") && !strings.HasPrefix(trimmed, "::1") {
				return trimmed
			}
		}
	}

	// Fall back to RemoteAddr
	ip = r.RemoteAddr
	if ip != "" {
		// Remove port number if present
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
	}

	return ip
}

// statusCodeColor returns ANSI color code based on HTTP status code
func statusCodeColor(code int) string {
	switch {
	case code >= 200 && code < 300:
		return green
	case code >= 300 && code < 400:
		return white
	case code >= 400 && code < 500:
		return yellow
	default:
		return red
	}
}

// logRequest logs request information in a Gin-like format
func logRequest(handler string, r *http.Request, code int, latency time.Duration) {
	// Format latency
	var latencyColor string
	switch {
	case latency > 500*time.Millisecond:
		latencyColor = red
	case latency > 200*time.Millisecond:
		latencyColor = yellow
	default:
		latencyColor = green
	}

	latencyStr := fmt.Sprintf("%s%v%s", latencyColor, latency.Round(time.Millisecond), reset)
	statusColor := statusCodeColor(code)
	methodColor := cyan

	log.Printf("%s%s%s |%s %3d %s| %13v | %15s |%s %-7s %s %s\n",
		methodColor, r.Method, reset,
		statusColor, code, reset,
		latencyStr,
		getClientIP(r),
		blue, handler, r.URL.Path, reset)
}

// loggingMiddleware wraps an http.HandlerFunc and logs request details
func loggingMiddleware(handler string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response wrapper to capture the status code
		rw := &responseWriter{ResponseWriter: w}

		next(rw, r)

		latency := time.Since(start)
		logRequest(handler, r, rw.statusCode, latency)
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = 200
	}
	return rw.ResponseWriter.Write(b)
}

func main() {
	uploadDir := getUploadDir()

	// Create uploads directory if it doesn't exist
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	// Initialize router with new pattern matching
	mux := http.NewServeMux()

	// Static file routes
	mux.HandleFunc("GET /", loggingMiddleware("static", handleIndex))
	mux.HandleFunc("GET /{id}", loggingMiddleware("static", handleIndex))
	mux.HandleFunc("GET /wasm_exec.js", loggingMiddleware("static", handleWasmExec))
	mux.HandleFunc("GET /encryption.wasm", loggingMiddleware("static", handleWasmFile))

	// API routes
	mux.HandleFunc("POST /upload", loggingMiddleware("upload", handleUpload(uploadDir)))
	mux.HandleFunc("GET /download/{id}", loggingMiddleware("download", handleDownload(uploadDir)))
	mux.HandleFunc("GET /metadata/{id}", loggingMiddleware("metadata", handleMetadata(uploadDir)))

	log.Printf("%s[SERVER]%s Starting on :8080 with upload directory: %s", magenta, reset, uploadDir)
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func generateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		log.Printf("%s[ERROR]%s Failed to generate ID: %v", red, reset, err)
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func handleWasmExec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	http.ServeFile(w, r, "wasm_exec.js")
}

func handleWasmFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/wasm")
	http.ServeFile(w, r, "encryption.wasm")
}

func handleMetadata(uploadDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if len(id) != 32 {
			log.Printf("%s[ERROR]%s Invalid file ID: %s", red, reset, id)
			http.Error(w, "Invalid file ID", http.StatusBadRequest)
			return
		}

		file, err := os.Open(filepath.Join(uploadDir, id))
		if err != nil {
			if os.IsNotExist(err) {
				log.Printf("%s[ERROR]%s File not found: %s", red, reset, id)
				http.Error(w, "File not found", http.StatusNotFound)
			} else {
				log.Printf("%s[ERROR]%s Opening file %s: %v", red, reset, id, err)
				http.Error(w, "Error opening file", http.StatusInternalServerError)
			}
			return
		}
		defer file.Close()

		header := make([]byte, 16)
		if _, err = io.ReadFull(file, header); err != nil {
			log.Printf("%s[ERROR]%s Reading header for %s: %v", red, reset, id, err)
			http.Error(w, "Error reading file header", http.StatusInternalServerError)
			return
		}

		metadataLen := binary.LittleEndian.Uint32(header[12:16])
		if metadataLen > 1024*1024 {
			log.Printf("%s[ERROR]%s Invalid metadata length for %s: %d", red, reset, id, metadataLen)
			http.Error(w, "Invalid metadata length", http.StatusInternalServerError)
			return
		}

		fullMetadata := make([]byte, 16+int(metadataLen))
		copy(fullMetadata[:16], header)

		if _, err = io.ReadFull(file, fullMetadata[16:]); err != nil {
			log.Printf("%s[ERROR]%s Reading metadata for %s: %v", red, reset, id, err)
			http.Error(w, "Error reading metadata", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-cache")
		if _, err := w.Write(fullMetadata); err != nil {
			log.Printf("%s[ERROR]%s Writing metadata response for %s: %v", red, reset, id, err)
		}
	}
}

func handleUpload(uploadDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxFileSize)

		if err := r.ParseMultipartForm(maxFileSize); err != nil {
			log.Printf("%s[ERROR]%s Parsing multipart form: %v", red, reset, err)
			http.Error(w, "File too large", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			log.Printf("%s[ERROR]%s Retrieving file from form: %v", red, reset, err)
			http.Error(w, "Error retrieving file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		id, err := generateID()
		if err != nil {
			log.Printf("%s[ERROR]%s Generating file ID: %v", red, reset, err)
			http.Error(w, "Error generating ID", http.StatusInternalServerError)
			return
		}

		dst, err := os.Create(filepath.Join(uploadDir, id))
		if err != nil {
			log.Printf("%s[ERROR]%s Creating file %s: %v", red, reset, id, err)
			http.Error(w, "Error creating file", http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		_, err = io.Copy(dst, file)
		if err != nil {
			log.Printf("%s[ERROR]%s Saving file %s: %v", red, reset, id, err)
			http.Error(w, "Error saving file", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"id":"` + id + `"}`)); err != nil {
			log.Printf("%s[ERROR]%s Writing upload response for %s: %v", red, reset, id, err)
		}
	}
}

func handleDownload(uploadDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if len(id) != 32 {
			log.Printf("%s[ERROR]%s Invalid file ID for download: %s", red, reset, id)
			http.Error(w, "Invalid file ID", http.StatusBadRequest)
			return
		}

		file, err := os.Open(filepath.Join(uploadDir, id))
		if err != nil {
			if os.IsNotExist(err) {
				log.Printf("%s[ERROR]%s File not found for download: %s", red, reset, id)
				http.Error(w, "File not found", http.StatusNotFound)
			} else {
				log.Printf("%s[ERROR]%s Opening file for download %s: %v", red, reset, id, err)
				http.Error(w, "Error opening file", http.StatusInternalServerError)
			}
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-cache")

		_, err = io.Copy(w, file)
		if err != nil {
			log.Printf("%s[ERROR]%s Streaming file %s: %v", red, reset, id, err)
			return
		}
	}
}
