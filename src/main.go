package main

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const (
	uploadDir   = "./uploads"
	maxFileSize = 5 * 1024 * 1024 * 1024 // 5GB
)

func main() {
	// Create uploads directory if it doesn't exist
	os.MkdirAll(uploadDir, 0755)

	// Serve static files
	http.HandleFunc("/", serveFile)
	http.HandleFunc("/upload", handleUpload)
	http.HandleFunc("/download/", handleDownload)
	http.HandleFunc("/metadata/", handleMetadata)

	http.HandleFunc("/wasm_exec.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "wasm_exec.js")
	})

	http.HandleFunc("/encryption.wasm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		http.ServeFile(w, r, "encryption.wasm")
	})

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func generateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func serveFile(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		// Serve the main HTML file
		http.ServeFile(w, r, "index.html")
		return
	}

	// Check if path looks like a file ID
	if len(r.URL.Path) == 33 { // "/" + 32 hex chars
		http.ServeFile(w, r, "index.html")
		return
	}

	http.NotFound(w, r)
}

func handleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract file ID from path
	id := filepath.Base(r.URL.Path)
	if len(id) != 32 {
		http.Error(w, "Invalid file ID", http.StatusBadRequest)
		return
	}

	// Open file
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

	// Read first chunk of file (metadata should be at the start)
	buffer := make([]byte, 1024) // Usually metadata is small
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-cache")

	// Write the chunk containing metadata
	w.Write(buffer[:n])
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize)

	// Parse multipart form
	err := r.ParseMultipartForm(maxFileSize)
	if err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Generate unique ID for file
	id, err := generateID()
	if err != nil {
		http.Error(w, "Error generating ID", http.StatusInternalServerError)
		return
	}

	// Create file
	dst, err := os.Create(filepath.Join(uploadDir, id))
	if err != nil {
		http.Error(w, "Error creating file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Copy file contents
	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	// Return file ID
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"id":"` + id + `"}`))
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract file ID from path
	id := filepath.Base(r.URL.Path)
	if len(id) != 32 {
		http.Error(w, "Invalid file ID", http.StatusBadRequest)
		return
	}

	// Open file
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

	// Set headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache")

	// Stream file to response
	_, err = io.Copy(w, file)
	if err != nil {
		log.Printf("Error streaming file: %v", err)
	}
}
