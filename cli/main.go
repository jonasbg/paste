package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/jonasbg/paste/crypto"
)

const (
	DefaultURL  = "https://paste.torden.tech"
	ChunkSizeMB = 4
)

var Version = "dev"

var (
	pasteURL string
)

type Metadata struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type Config struct {
	MaxFileSizeBytes int64 `json:"max_file_size_bytes"`
	ChunkSize        int   `json:"chunk_size"`
	KeySize          int   `json:"key_size"`
}

func init() {
	// URL can be set via environment variable or build-time flag
	if envURL := os.Getenv("PASTE_URL"); envURL != "" {
		pasteURL = envURL
	} else {
		pasteURL = DefaultURL
	}
}

func main() {
	// Check if stdin is piped or redirected
	stat, _ := os.Stdin.Stat()
	stdinIsPiped := (stat.Mode() & os.ModeCharDevice) == 0

	uploadCmd := flag.NewFlagSet("upload", flag.ExitOnError)
	downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)

	// Upload flags
	uploadFile := uploadCmd.String("f", "", "File to upload (omit to read from stdin)")
	uploadName := uploadCmd.String("n", "", "Override filename (default: uses file name or 'stdin.txt')")
	uploadURL := uploadCmd.String("url", pasteURL, "Paste server URL")

	// Download flags
	downloadLink := downloadCmd.String("l", "", "Download link (format: https://paste.torden.tech/{id}#key={key})")
	downloadOutput := downloadCmd.String("o", "", "Output file (default: original filename or stdout)")
	downloadURL := downloadCmd.String("url", pasteURL, "Paste server URL")

	// If no args provided
	if len(os.Args) < 2 {
		if stdinIsPiped {
			// Default to upload from stdin
			if err := handleUpload("", "", pasteURL); err != nil {
				fmt.Fprintf(os.Stderr, "Upload failed: %v\n", err)
				os.Exit(1)
			}
			return
		}
		printUsage()
		os.Exit(1)
	}

	// If first arg is a flag and stdin is piped, treat as upload
	if strings.HasPrefix(os.Args[1], "-") && stdinIsPiped {
		uploadCmd.Parse(os.Args[1:])
		if err := handleUpload(*uploadFile, *uploadName, *uploadURL); err != nil {
			fmt.Fprintf(os.Stderr, "Upload failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch os.Args[1] {
	case "upload":
		uploadCmd.Parse(os.Args[2:])
		if err := handleUpload(*uploadFile, *uploadName, *uploadURL); err != nil {
			fmt.Fprintf(os.Stderr, "Upload failed: %v\n", err)
			os.Exit(1)
		}

	case "download":
		downloadCmd.Parse(os.Args[2:])
		if *downloadLink == "" {
			fmt.Fprintf(os.Stderr, "Error: download link is required\n")
			downloadCmd.PrintDefaults()
			os.Exit(1)
		}
		if err := handleDownload(*downloadLink, *downloadOutput, *downloadURL); err != nil {
			fmt.Fprintf(os.Stderr, "Download failed: %v\n", err)
			os.Exit(1)
		}

	case "version", "-v", "--version":
		fmt.Printf("paste v%s\n", Version)

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `paste v%s - Upload and download files to paste.torden.tech

Usage:
  paste [flags]                 Upload from stdin (when piped/redirected)
  paste upload [flags]          Upload a file or stdin
  paste download [flags]        Download a file
  paste version                 Show version
  paste help                    Show this help

Upload Examples:
  echo "Hello World" | paste
  cat file.txt | paste
  paste < myfile.txt
  echo "data" | paste -n "custom-name.txt"
  paste upload -f document.pdf
  cat image.png | paste upload -n "my-image.png"
  paste upload -f file.txt -url https://custom.paste.server

Download Examples:
  paste download -l "https://paste.torden.tech/abc123#key=xyz..."
  paste download -l "https://paste.torden.tech/abc123#key=xyz..." -o output.txt

Environment Variables:
  PASTE_URL    Default paste server URL (default: %s)

`, Version, DefaultURL)
}

func handleUpload(filePath, customName, serverURL string) error {
	var reader io.Reader
	var fileSize int64
	var filename string

	// Determine input source
	if filePath == "" {
		// Read from stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return errors.New("no input provided (use -f or pipe data to stdin)")
		}
		reader = os.Stdin
		filename = "stdin.txt"

		// For stdin, we need to buffer to determine size
		data, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		fileSize = int64(len(data))
		reader = strings.NewReader(string(data))
	} else {
		// Read from file
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat file: %w", err)
		}
		fileSize = stat.Size()
		filename = filepath.Base(filePath)
		reader = file
	}

	// Override filename if provided
	if customName != "" {
		filename = customName
	}

	// Get server config
	config, err := getServerConfig(serverURL)
	if err != nil {
		return fmt.Errorf("failed to get server config: %w", err)
	}

	if fileSize > config.MaxFileSizeBytes {
		return fmt.Errorf("file size (%d bytes) exceeds server limit (%d bytes)", fileSize, config.MaxFileSizeBytes)
	}

	// Generate encryption key
	key, err := crypto.GenerateKey(config.KeySize / 8)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}
	keyBase64 := base64.URLEncoding.EncodeToString(key)

	// Upload the file
	fileID, err := uploadFile(serverURL, reader, filename, fileSize, key, config.ChunkSize)
	if err != nil {
		return err
	}

	// Generate the shareable URL
	shareURL := fmt.Sprintf("%s/%s#key=%s", serverURL, fileID, keyBase64)
	fmt.Printf("\n%s\n", shareURL)
	fmt.Printf("\nDownload with: paste download -l \"%s\"\n", shareURL)

	return nil
}

func handleDownload(link, outputPath, serverURL string) error {
	// Parse the download link
	fileID, key, err := parseDownloadLink(link)
	if err != nil {
		return err
	}

	// Extract server URL from link if present
	if parsedURL, err := url.Parse(link); err == nil && parsedURL.Host != "" {
		serverURL = fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	}

	// Get server config for chunk size
	config, err := getServerConfig(serverURL)
	if err != nil {
		return fmt.Errorf("failed to get server config: %w", err)
	}

	// Fetch metadata
	metadata, err := fetchMetadata(serverURL, fileID, key)
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

	// Download and decrypt with streaming (memory-efficient)
	if err := downloadAndDecryptStreaming(serverURL, fileID, key, config.ChunkSize, writer); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Download complete: %s\n", outputPath)
	}

	return nil
}

func getServerConfig(serverURL string) (*Config, error) {
	resp, err := http.Get(serverURL + "/api/config")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var config Config
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func uploadFile(serverURL string, reader io.Reader, filename string, fileSize int64, key []byte, chunkSizeMB int) (string, error) {
	// Convert HTTP URL to WebSocket URL
	wsURL := strings.Replace(serverURL, "https://", "wss://", 1)
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
	metadata := Metadata{
		Filename:    filename,
		ContentType: "application/octet-stream",
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
	chunkSize := chunkSizeMB * 1024 * 1024
	buffer := make([]byte, chunkSize)

	for {
		n, err := reader.Read(buffer)
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("failed to read data: %w", err)
		}
		if n == 0 {
			break
		}

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

		if err == io.EOF {
			break
		}
	}

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

func parseDownloadLink(link string) (fileID string, key []byte, error error) {
	parsedURL, err := url.Parse(link)
	if err != nil {
		return "", nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Extract file ID from path
	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) == 0 || pathParts[len(pathParts)-1] == "" {
		return "", nil, errors.New("invalid link: missing file ID")
	}
	fileID = pathParts[len(pathParts)-1]

	// Extract key from fragment
	fragment := parsedURL.Fragment
	if !strings.HasPrefix(fragment, "key=") {
		return "", nil, errors.New("invalid link: missing encryption key")
	}
	keyBase64 := strings.TrimPrefix(fragment, "key=")

	// Add padding if needed
	if len(keyBase64)%4 != 0 {
		keyBase64 += strings.Repeat("=", 4-len(keyBase64)%4)
	}

	key, err = base64.URLEncoding.DecodeString(keyBase64)
	if err != nil {
		return "", nil, fmt.Errorf("invalid key: %w", err)
	}

	return fileID, key, nil
}

func fetchMetadata(serverURL, fileID string, key []byte) (*Metadata, error) {
	token, err := crypto.GenerateHMACToken(fileID, key)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", serverURL+"/api/metadata/"+fileID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-HMAC-Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	decrypted, err := crypto.DecryptMetadata(key, data)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	var metadata Metadata
	if err := json.Unmarshal(decrypted, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

func downloadAndDecryptStreaming(serverURL, fileID string, key []byte, chunkSizeMB int, writer io.Writer) error {
	token, err := crypto.GenerateHMACToken(fileID, key)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("GET", serverURL+"/api/download/"+fileID, nil)
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

	// Use the chunk size from server config (in MB)
	chunkSize := chunkSizeMB * 1024 * 1024
	buffer := make([]byte, chunkSize+crypto.GCMTagSize)

	// Read and decrypt chunks in a streaming fashion
	// Memory usage: Only one chunk in memory at a time (~4MB by default)
	for {
		n, err := io.ReadFull(resp.Body, buffer)

		if err == io.EOF {
			// No more data
			break
		}

		if err == io.ErrUnexpectedEOF {
			// Partial read - this is the last chunk (smaller than buffer)
			if n == 0 {
				break
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

	return nil
}
