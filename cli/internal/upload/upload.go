package upload

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/jonasbg/paste/cli/internal/types"
	"github.com/jonasbg/paste/cli/internal/ui"
	"github.com/jonasbg/paste/crypto"
)

// Handler handles file uploads
type Handler struct {
	serverURL string
	config    *types.Config
}

// NewHandler creates a new upload handler
func NewHandler(serverURL string, config *types.Config) *Handler {
	return &Handler{
		serverURL: serverURL,
		config:    config,
	}
}

// Upload uploads a file or stdin data
func (h *Handler) Upload(reader io.Reader, filename string, contentType string, fileSize int64, key []byte) (string, error) {
	fileID, err := h.uploadFile(reader, filename, contentType, fileSize, key)
	if err != nil {
		return "", err
	}

	// Generate the shareable URL
	keyBase64 := base64.URLEncoding.EncodeToString(key)
	shareURL := fmt.Sprintf("%s/%s#key=%s", h.serverURL, fileID, keyBase64)

	return shareURL, nil
}

func (h *Handler) uploadFile(reader io.Reader, filename string, contentType string, fileSize int64, key []byte) (string, error) {
	// Convert HTTP URL to WebSocket URL
	wsURL := strings.Replace(h.serverURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/ws/upload"

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	// Step 1: Initialize upload
	initMsg := map[string]interface{}{
		"type": "init",
		"size": fileSize,
	}
	if err := conn.WriteJSON(initMsg); err != nil {
		return "", fmt.Errorf("failed to send init: %w", err)
	}

	var initResp map[string]interface{}
	if err := conn.ReadJSON(&initResp); err != nil {
		return "", fmt.Errorf("failed to read init response: %w", err)
	}

	fileID, ok := initResp["id"].(string)
	if !ok {
		return "", errors.New("invalid init response")
	}

	// Step 2: Generate and send HMAC token
	token, err := crypto.GenerateHMACToken(fileID, key)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	tokenMsg := map[string]interface{}{
		"type":  "token",
		"token": token,
	}
	if err := conn.WriteJSON(tokenMsg); err != nil {
		return "", fmt.Errorf("failed to send token: %w", err)
	}

	var tokenResp map[string]interface{}
	if err := conn.ReadJSON(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	// Step 3: Encrypt and send metadata
	metadata := types.Metadata{
		Filename:    filename,
		ContentType: contentType,
		Size:        fileSize,
	}
	metadataJSON, _ := json.Marshal(metadata)

	encryptedMetadataHeader, err := crypto.EncryptMetadata(key, metadataJSON)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt metadata: %w", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, encryptedMetadataHeader); err != nil {
		return "", fmt.Errorf("failed to send metadata: %w", err)
	}

	var metadataResp map[string]interface{}
	if err := conn.ReadJSON(&metadataResp); err != nil {
		return "", fmt.Errorf("failed to read metadata response: %w", err)
	}

	// Step 4: Create streaming cipher and send IV
	streamCipher, err := crypto.NewStreamCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	defer streamCipher.Clear()

	if err := conn.WriteMessage(websocket.BinaryMessage, streamCipher.IV()); err != nil {
		return "", fmt.Errorf("failed to send IV: %w", err)
	}

	// Step 5: Stream encrypted chunks
	chunkSize := h.config.ChunkSize * 1024 * 1024
	buffer := make([]byte, chunkSize)

	// Create progress bar
	bar := ui.NewProgressBar(fileSize, "Uploading")

	var totalRead int64
	for {
		n, err := reader.Read(buffer)
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("failed to read data: %w", err)
		}
		if n == 0 {
			break
		}

		totalRead += int64(n)

		// Encrypt chunk
		encryptedChunk, err := streamCipher.EncryptChunk(buffer[:n])
		if err != nil {
			return "", fmt.Errorf("failed to encrypt chunk: %w", err)
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, encryptedChunk); err != nil {
			return "", fmt.Errorf("failed to send chunk: %w", err)
		}

		var ackResp map[string]interface{}
		if err := conn.ReadJSON(&ackResp); err != nil {
			return "", fmt.Errorf("failed to read ack: %w", err)
		}

		// Update progress bar
		bar.Update(totalRead)

		if err == io.EOF {
			break
		}
	}
	bar.Finish()

	// Step 6: Send end-of-upload marker
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0x00}); err != nil {
		return "", fmt.Errorf("failed to send end marker: %w", err)
	}

	var finalResp map[string]interface{}
	if err := conn.ReadJSON(&finalResp); err != nil {
		return "", fmt.Errorf("failed to read final response: %w", err)
	}

	return fileID, nil
}

// PrepareInput prepares the input for upload (file or stdin)
func PrepareInput(filePath, customName string) (io.Reader, string, string, int64, error) {
	var reader io.Reader
	var fileSize int64
	var filename string
	var contentType string

	// Determine input source
	if filePath == "" {
		// Read from stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return nil, "", "", 0, errors.New("no input provided (use -f or pipe data to stdin)")
		}
		reader = os.Stdin
		filename = "stdin.txt"

		// For stdin, we need to buffer to determine size
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, "", "", 0, fmt.Errorf("failed to read stdin: %w", err)
		}
		fileSize = int64(len(data))
		reader = strings.NewReader(string(data))

		// Detect content type from data
		contentType = http.DetectContentType(data)

		// If we detected a content type and no custom name, update filename extension
		if contentType != "application/octet-stream" && customName == "" {
			if ext := getExtensionFromContentType(contentType); ext != "" {
				filename = "stdin" + ext
			}
		}
	} else {
		// Check if it's a directory
		stat, err := os.Stat(filePath)
		if err != nil {
			return nil, "", "", 0, fmt.Errorf("failed to stat file: %w", err)
		}

		if stat.IsDir() {
			// Directory - create tar.gz archive
			fmt.Fprintf(os.Stderr, "Compressing directory: %s\n", filePath)
			archiveData, err := createTarGz(filePath)
			if err != nil {
				return nil, "", "", 0, fmt.Errorf("failed to create archive: %w", err)
			}

			fileSize = int64(len(archiveData))
			filename = filepath.Base(filePath) + ".tar.gz"
			contentType = "application/gzip"
			reader = bytes.NewReader(archiveData)
		} else {
			// Regular file
			file, err := os.Open(filePath)
			if err != nil {
				return nil, "", "", 0, fmt.Errorf("failed to open file: %w", err)
			}

			fileSize = stat.Size()
			filename = filepath.Base(filePath)

			// Detect content type from file data
			buffer := make([]byte, 512)
			n, _ := file.Read(buffer)
			contentType = http.DetectContentType(buffer[:n])

			// Reset file pointer to beginning
			file.Seek(0, 0)
			reader = file
		}
	}

	// Override filename if provided
	if customName != "" {
		filename = customName
	}

	// Default content type if not detected
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return reader, filename, contentType, fileSize, nil
}

func getExtensionFromContentType(contentType string) string {
	contentTypeMap := map[string]string{
		"image/jpeg":           ".jpg",
		"image/jpg":            ".jpg",
		"image/png":            ".png",
		"image/gif":            ".gif",
		"image/webp":           ".webp",
		"image/svg+xml":        ".svg",
		"video/mp4":            ".mp4",
		"video/mpeg":           ".mpeg",
		"video/webm":           ".webm",
		"video/quicktime":      ".mov",
		"audio/mpeg":           ".mp3",
		"audio/wav":            ".wav",
		"audio/ogg":            ".ogg",
		"application/pdf":      ".pdf",
		"application/zip":      ".zip",
		"application/x-gzip":   ".gz",
		"application/x-tar":    ".tar",
		"text/plain":           ".txt",
		"text/html":            ".html",
		"text/css":             ".css",
		"text/javascript":      ".js",
		"application/json":     ".json",
		"application/xml":      ".xml",
	}

	if ext, ok := contentTypeMap[contentType]; ok {
		return ext
	}
	return ""
}

// createTarGz creates a tar.gz archive of a directory
func createTarGz(dirPath string) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Get the base directory name for the archive
	baseDir := filepath.Base(dirPath)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Update the name to be relative to the base directory
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		header.Name = filepath.Join(baseDir, relPath)

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// If it's a file, write its contents
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Close writers
	if err := tarWriter.Close(); err != nil {
		return nil, err
	}
	if err := gzWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
