package download

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/jonasbg/paste/crypto"
	"github.com/jonasbg/paste/pastectl/internal/client"
	"github.com/jonasbg/paste/pastectl/internal/types"
	"github.com/jonasbg/paste/pastectl/internal/ui"
)

// Handler handles file downloads
type Handler struct {
	client *client.Client
	config *types.Config
}

// NewHandler creates a new download handler
func NewHandler(c *client.Client, config *types.Config) *Handler {
	return &Handler{
		client: c,
		config: config,
	}
}

// Download downloads and decrypts a file
func (h *Handler) Download(fileID string, key []byte, outputPath string) error {
	// Fetch metadata
	metadata,token, err := h.client.FetchMetadata(fileID, key)
	if err != nil {
		return fmt.Errorf("failed to fetch metadata: %w", err)
	}

	// Determine output
	var writer io.Writer
	if outputPath == "" {
		// Check if stdout is a terminal
		stat, _ := os.Stdout.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			// Terminal - use original filename
			outputPath = metadata.Filename
		}
	}

	if outputPath != "" {
		file, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()
		writer = file
		fmt.Fprintf(os.Stderr, "Downloading to: %s\n", outputPath)
	} else {
		writer = os.Stdout
	}

	// Download and decrypt with streaming
	if err := h.downloadAndDecryptStreaming(fileID, token, key, writer); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Download complete: %s\n", outputPath)
	}

	if err := h.client.DeleteFile(fileID, token); err != nil {
		return fmt.Errorf("failed to delete file after download: %w", err)
	}

	return nil
}

// DownloadWithPassphrase downloads a file using a passphrase
func (h *Handler) DownloadWithPassphrase(passphrase string, outputPath string) error {
	// Validate passphrase
	if err := crypto.ValidatePassphrase(passphrase); err != nil {
		return fmt.Errorf("invalid passphrase: %w", err)
	}

	// Derive fileID and key from passphrase
	fileID, key, err := crypto.DeriveFromPassphrase(passphrase, h.config.KeySize/8)
	if err != nil {
		return fmt.Errorf("failed to derive key from passphrase: %w", err)
	}

	// Download using derived credentials
	return h.Download(fileID, key, outputPath)
}

// IsPassphrase checks if the input looks like a passphrase (word-word-word-...-suffix)
// New format: 3-8 words followed by a 4-char alphanumeric suffix with at least one digit
func IsPassphrase(input string) bool {
	// URLs contain :// or start with http/https
	if strings.Contains(input, "://") || strings.HasPrefix(input, "http") {
		return false
	}

	// Check if it matches passphrase pattern
	parts := strings.Split(input, "-")
	if len(parts) < 5 || len(parts) > 9 { // 4-8 words + 1 suffix
		return false
	}

	// All parts should be lowercase alphanumeric
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, c := range part {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				return false
			}
		}
	}

	// Last part should be a valid suffix (4 chars with at least one digit)
	suffix := parts[len(parts)-1]
	if len(suffix) != 4 {
		return false
	}
	hasDigit := false
	for _, c := range suffix {
		if c >= '0' && c <= '9' {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return false
	}

	return true
}

func (h *Handler) downloadAndDecryptStreaming(fileID string, token string, key []byte, writer io.Writer) error {
	// Get base URL from client
	baseURL := h.client.BaseURL()
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/download/%s", baseURL, fileID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-HMAC-Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Read metadata header (16 bytes)
	metadataHeader := make([]byte, 16)
	if _, err := io.ReadFull(resp.Body, metadataHeader); err != nil {
		return fmt.Errorf("failed to read metadata header: %w", err)
	}

	// Parse metadata length
	metadataLen := binary.LittleEndian.Uint32(metadataHeader[12:16])

	// Skip encrypted metadata (we already fetched it separately)
	if _, err := io.CopyN(io.Discard, resp.Body, int64(metadataLen)); err != nil {
		return fmt.Errorf("failed to skip metadata: %w", err)
	}

	// Read IV
	iv := make([]byte, crypto.IVSize)
	if _, err := io.ReadFull(resp.Body, iv); err != nil {
		return fmt.Errorf("failed to read IV: %w", err)
	}

	// Create stream decryptor
	streamCipher, err := crypto.NewStreamDecryptor(key, iv)
	if err != nil {
		return err
	}
	defer streamCipher.Clear()

	// Get content length for progress bar
	contentLength := resp.ContentLength

	// Use the chunk size from server config (in MB)
	chunkSize := h.config.ChunkSize * 1024 * 1024
	buffer := make([]byte, chunkSize+crypto.GCMTagSize)

	// Create progress bar
	var bar *ui.ProgressBar
	if contentLength > 0 {
		bar = ui.NewProgressBar(contentLength, "Downloading")
	}

	var totalRead int64

	// Read and decrypt chunks in a streaming fashion
	for {
		n, err := io.ReadFull(resp.Body, buffer)

		if err == io.EOF {
			break
		}

		if err == io.ErrUnexpectedEOF {
			if n == 0 {
				break
			}

			totalRead += int64(n)
			if bar != nil {
				bar.Update(totalRead)
			}

			decrypted, decryptErr := streamCipher.DecryptChunk(buffer[:n])
			if decryptErr != nil {
				return fmt.Errorf("decryption failed on final chunk: %w", decryptErr)
			}

			if _, err := writer.Write(decrypted); err != nil {
				return err
			}
			break
		}

		if err != nil {
			return fmt.Errorf("failed to read chunk: %w", err)
		}

		totalRead += int64(n)
		if bar != nil {
			bar.Update(totalRead)
		}

		// Decrypt the chunk
		decrypted, decryptErr := streamCipher.DecryptChunk(buffer[:n])
		if decryptErr != nil {
			return fmt.Errorf("decryption failed: %w", decryptErr)
		}

		// Write decrypted data
		if _, err := writer.Write(decrypted); err != nil {
			return err
		}
	}

	if bar != nil {
		bar.Finish()
	}

	return nil
}

// ParseLink parses a download link and extracts the file ID and key
func ParseLink(link string) (fileID string, key []byte, serverURL string, error error) {
	parsedURL, err := url.Parse(link)
	if err != nil {
		return "", nil, "", fmt.Errorf("invalid URL: %w", err)
	}

	// Extract server URL
	serverURL = fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)

	// Extract file ID from path
	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) == 0 || pathParts[len(pathParts)-1] == "" {
		return "", nil, "", errors.New("invalid link: missing file ID")
	}
	fileID = pathParts[len(pathParts)-1]

	// Extract key from fragment
	fragment := parsedURL.Fragment
	if !strings.HasPrefix(fragment, "key=") {
		return "", nil, "", errors.New("invalid link: missing encryption key")
	}
	keyBase64 := strings.TrimPrefix(fragment, "key=")

	// Add padding if needed
	if len(keyBase64)%4 != 0 {
		keyBase64 += strings.Repeat("=", 4-len(keyBase64)%4)
	}

	key, err = base64.URLEncoding.DecodeString(keyBase64)
	if err != nil {
		return "", nil, "", fmt.Errorf("invalid key: %w", err)
	}

	return fileID, key, serverURL, nil
}
