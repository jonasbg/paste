package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	headerSize      = 16          // Size of metadata header
	maxMetadataSize = 1024 * 1024 // 1MB max metadata size

	// Tokens are base64url HMACs truncated to the key length, so a 128-bit key
	// gives 22 characters and a 256-bit key 43. Accepting the whole range keeps
	// existing files reachable when an operator changes KEY_SIZE.
	minTokenLength = 22
	maxTokenLength = 43
)

// tokenDigest hashes an HMAC access token to lowercase hex before using it in
// on-disk filenames. This prevents raw access tokens from appearing in
// directory listings, backups, or error logs, and avoids case-folding issues
// on case-insensitive filesystems.
func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// generateID generates a cryptographically secure random ID with a specified bit length.
// It supports bit lengths of 64, 128, 192, and 256. If no length is provided, it defaults to 128 bits.
func generateID(bitLength ...int) (string, error) {
	length := 128
	if len(bitLength) > 0 {
		length = bitLength[0]
	}

	switch length {
	case 64, 128, 192, 256:
		byteLength := length / 8
		bytes := make([]byte, byteLength)
		if _, err := rand.Read(bytes); err != nil {
			return "", fmt.Errorf("failed to generate random bytes: %w", err)
		}
		return hex.EncodeToString(bytes), nil
	default:
		return "", fmt.Errorf("invalid ID length: %d. Must be 64, 128, 192, or 256", length)
	}
}

// resolveFile validates the file ID and the X-HMAC-Token header and opens the
// matching upload. On failure it writes the error response and returns nil.
func resolveFile(c *gin.Context, uploadDir string) (*os.File, string) {
	id := c.Param("id")
	if !validateID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return nil, ""
	}

	token := c.GetHeader("X-HMAC-Token")
	if !validateToken(token) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid token"})
		return nil, ""
	}

	// Primary path uses SHA-256 hex digest of the token.
	hashedPath := filepath.Join(uploadDir, id+"."+tokenDigest(token))
	file, err := os.Open(hashedPath)
	if err == nil {
		return file, hashedPath
	}

	// Fallback for legacy files stored before token hashing was introduced.
	if errors.Is(err, fs.ErrNotExist) {
		legacyPath := filepath.Join(uploadDir, id+"."+token)
		if legacyFile, legacyErr := os.Open(legacyPath); legacyErr == nil {
			return legacyFile, legacyPath
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return nil, ""
	}

	log.Printf("Error: Failed to open file for id=%s", id)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
	return nil, ""
}

func setDownloadCountHeaders(c *gin.Context, maxDownloads, remaining int) {
	c.Header("X-Max-Downloads", strconv.Itoa(maxDownloads))
	if maxDownloads == 0 || remaining < 0 {
		c.Header("X-Downloads-Remaining", "unlimited")
	} else {
		c.Header("X-Downloads-Remaining", strconv.Itoa(remaining))
	}
}

func HandleMetadata(uploadDir string, downloads *DownloadStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, filePath := resolveFile(c, uploadDir)
		if file == nil {
			return
		}
		defer file.Close()

		if downloads != nil {
			maxDownloads, remaining, ok := downloads.Peek(filePath)
			if !ok {
				c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
				return
			}
			setDownloadCountHeaders(c, maxDownloads, remaining)
		}

		// Get file info for size
		fileInfo, err := file.Stat()
		if err != nil {
			log.Printf("Error: Failed to get file info for id=%s", c.Param("id"))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		// Read header to get metadata length
		header := make([]byte, headerSize)
		if _, err = io.ReadFull(file, header); err != nil {
			log.Printf("Error: Failed to read header for id=%s", c.Param("id"))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		metadataLen := binary.LittleEndian.Uint32(header[12:16])
		if metadataLen > maxMetadataSize {
			log.Printf("Error: Metadata size exceeds limit for id=%s", c.Param("id"))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		// Read full metadata including header
		fullMetadata := make([]byte, headerSize+int(metadataLen))
		copy(fullMetadata, header)

		if _, err = io.ReadFull(file, fullMetadata[headerSize:]); err != nil {
			log.Printf("Error: Failed to read metadata for id=%s", c.Param("id"))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}

		c.Header("Content-Type", "application/octet-stream")
		c.Header("Cache-Control", "no-store")
		c.Header("X-File-Size", strconv.FormatInt(fileInfo.Size(), 10))

		_, _ = c.Writer.Write(fullMetadata)
	}
}

func HandleDelete(uploadDir string, downloads *DownloadStore, limiter *UploadLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, filePath := resolveFile(c, uploadDir)
		if file == nil {
			return
		}

		var fileSize int64
		if info, err := file.Stat(); err == nil {
			fileSize = info.Size()
		}
		file.Close()

		// Unless force-delete is explicitly requested, do not delete a file that
		// still has remaining download slots (keeps multi-download files intact
		// when older clients issue a post-download DELETE).
		if downloads != nil && !strings.EqualFold(c.GetHeader("X-Paste-Force-Delete"), "true") {
			maxDownloads, remaining, ok := downloads.Peek(filePath)
			if ok && (maxDownloads == 0 || remaining > 0) {
				c.JSON(http.StatusOK, gin.H{
					"deleted":       false,
					"max_downloads": maxDownloads,
					"remaining":     remaining,
					"message":       "File retained for remaining downloads",
				})
				return
			}
		}

		if err := os.Remove(filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("Error: Failed to delete file for id=%s", c.Param("id"))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error"})
			return
		}
		if downloads != nil {
			downloads.RemoveMeta(filePath)
		}
		limiter.RecordDeleted(fileSize)

		c.JSON(http.StatusOK, gin.H{
			"deleted": true,
			"message": "File deleted successfully",
		})
	}
}

// isNonZeroRange reports whether the request asks for a resumed byte range
// starting after byte 0 (so range continuations don't consume extra slots).
func isNonZeroRange(r *http.Request) bool {
	rng := strings.TrimSpace(r.Header.Get("Range"))
	if rng == "" {
		return false
	}
	return !strings.HasPrefix(rng, "bytes=0-")
}

// HandleDownload streams the stored ciphertext and enforces server-side download
// limits. Previews (X-Paste-Purpose: preview) and resumed Range requests do not
// consume a download slot. When the final allowed download begins, the file is
// immediately unlinked while the open file descriptor finishes streaming.
func HandleDownload(uploadDir string, downloads *DownloadStore, limiter *UploadLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, filePath := resolveFile(c, uploadDir)
		if file == nil {
			return
		}
		defer file.Close()

		var fileSize int64
		if info, err := file.Stat(); err == nil {
			fileSize = info.Size()
		}

		if downloads != nil {
			isPreview := strings.EqualFold(c.GetHeader("X-Paste-Purpose"), "preview")
			if isPreview || isNonZeroRange(c.Request) {
				maxDownloads, remaining, ok := downloads.Peek(filePath)
				if !ok {
					c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
					return
				}
				setDownloadCountHeaders(c, maxDownloads, remaining)
			} else {
				remaining, maxDownloads, exhausted, allowed := downloads.Consume(filePath)
				if !allowed {
					c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
					return
				}
				setDownloadCountHeaders(c, maxDownloads, remaining)
				if exhausted {
					// Unlink immediately so no subsequent download can start;
					// the open file descriptor remains readable until ServeContent completes.
					if err := os.Remove(filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
						log.Printf("Error: Failed to remove exhausted file for id=%s", c.Param("id"))
					}
					downloads.RemoveMeta(filePath)
					limiter.RecordDeleted(fileSize)
				}
			}
		}

		c.Header("Content-Type", "application/octet-stream")
		c.Header("Cache-Control", "no-store")

		// ServeContent sets Content-Length and supports Range requests.
		http.ServeContent(c.Writer, c.Request, "", time.Time{}, file)
	}
}

func validateToken(token string) bool {
	if len(token) < minTokenLength || len(token) > maxTokenLength {
		return false
	}

	// Ensure token only contains safe base64url characters
	for i := 0; i < len(token); i++ {
		ch := token[i]
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// validateID checks that id is a hex file ID of a supported length and is
// therefore safe to use in a file name. Generated IDs are ID_SIZE/4 characters
// (64-256 bit); passphrase IDs are 32. 24 is kept for older clients.
func validateID(id string) bool {
	switch len(id) {
	case 16, 24, 32, 48, 64:
	default:
		return false
	}
	for i := 0; i < len(id); i++ {
		ch := id[i]
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F') {
			return false
		}
	}
	return true
}

// validatePassphraseID strictly validates a client-supplied passphrase-derived
// file ID (always 32 lowercase hex characters = 128 bits from HKDF).
// Restricting custom IDs prevents probing random-ID uploads of other lengths.
func validatePassphraseID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for i := 0; i < len(id); i++ {
		ch := id[i]
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}
