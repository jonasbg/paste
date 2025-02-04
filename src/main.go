package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const (
	maxFileSize = 5 * 1024 * 1024 * 1024 // 5GB
)

// getUploadDir returns the upload directory from environment variable or default
func getUploadDir() string {
	if dir := os.Getenv("UPLOAD_DIR"); dir != "" {
		return dir
	}
	return "./uploads"
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
	mux.HandleFunc("GET /", handleIndex)
	mux.HandleFunc("GET /{id}", handleIndex)
	mux.HandleFunc("GET /wasm_exec.js", handleWasmExec)
	mux.HandleFunc("GET /encryption.wasm", handleWasmFile)

	// API routes
	mux.HandleFunc("POST /upload", handleUpload(uploadDir))
	mux.HandleFunc("GET /download/{id}", handleDownload(uploadDir))
	mux.HandleFunc("GET /metadata/{id}", handleMetadata(uploadDir))

	log.Printf("Server starting on :8080 with upload directory: %s", uploadDir)
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func generateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
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
			http.Error(w, "Invalid file ID", http.StatusBadRequest)
			return
		}

		file, err := os.Open(filepath.Join(uploadDir, id))
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "File not found", http.StatusNotFound)
			} else {
				http.Error(w, "Error opening file", http.StatusInternalServerError)
			}
			return
		}
		defer file.Close()

		// Read header to get metadata length (16 bytes: 12 for IV + 4 for length)
		header := make([]byte, 16)
		if _, err = io.ReadFull(file, header); err != nil {
			http.Error(w, "Error reading file header", http.StatusInternalServerError)
			return
		}

		// Extract metadata length from bytes 12-15
		metadataLen := binary.LittleEndian.Uint32(header[12:16])

		// Sanity check on metadata length
		if metadataLen > 1024*1024 { // 1MB max for metadata
			http.Error(w, "Invalid metadata length", http.StatusInternalServerError)
			return
		}

		// Allocate buffer for full metadata section
		fullMetadata := make([]byte, 16+int(metadataLen))
		copy(fullMetadata[:16], header)

		// Read the encrypted metadata portion
		if _, err = io.ReadFull(file, fullMetadata[16:]); err != nil {
			http.Error(w, "Error reading metadata", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(fullMetadata)
	}
}

func handleUpload(uploadDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Limit request body size
		r.Body = http.MaxBytesReader(w, r.Body, maxFileSize)

		if err := r.ParseMultipartForm(maxFileSize); err != nil {
			http.Error(w, "File too large", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Error retrieving file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		id, err := generateID()
		if err != nil {
			http.Error(w, "Error generating ID", http.StatusInternalServerError)
			return
		}

		dst, err := os.Create(filepath.Join(uploadDir, id))
		if err != nil {
			http.Error(w, "Error creating file", http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		if _, err = io.Copy(dst, file); err != nil {
			http.Error(w, "Error saving file", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"` + id + `"}`))
	}
}

func handleDownload(uploadDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if len(id) != 32 {
			http.Error(w, "Invalid file ID", http.StatusBadRequest)
			return
		}

		file, err := os.Open(filepath.Join(uploadDir, id))
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "File not found", http.StatusNotFound)
			} else {
				http.Error(w, "Error opening file", http.StatusInternalServerError)
			}
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-cache")

		if _, err = io.Copy(w, file); err != nil {
			log.Printf("Error streaming file: %v", err)
		}
	}
}
