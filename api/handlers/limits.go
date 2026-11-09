package handlers

import (
	"log"
	"os"
	"strings"
	"sync"
)

// UploadLimiter bounds concurrent uploads per client IP and, when
// MAX_STORAGE_SIZE is set, the total bytes kept in the upload directory.
// Bytes of in-flight uploads are tracked in memory as they arrive, so parallel
// uploads cannot jointly overshoot the storage cap.
type UploadLimiter struct {
	uploadDir string

	mu       sync.Mutex
	active   map[string]int
	inFlight int64
}

func NewUploadLimiter(uploadDir string) *UploadLimiter {
	return &UploadLimiter{uploadDir: uploadDir, active: make(map[string]int)}
}

// Upload is one admitted upload. Call Done when it finishes, successfully or not.
type Upload struct {
	l       *UploadLimiter
	ip      string
	written int64
}

// Begin admits an upload of roughly size bytes from ip. It returns a
// client-facing error message when the upload must be rejected.
func (l *UploadLimiter) Begin(ip string, size int64) (*Upload, string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if limit := GlobalConfig.UploadsPerIP; limit > 0 && l.active[ip] >= limit {
		return nil, "Too many concurrent uploads"
	}
	if limit := GlobalConfig.MaxStorageBytes; limit > 0 {
		if l.storedBytes()+l.inFlight+size > limit {
			return nil, "Server storage is full, please try again later"
		}
	}

	l.active[ip]++
	return &Upload{l: l, ip: ip}, ""
}

// Add accounts n more received bytes and reports whether the storage cap
// still allows them.
func (u *Upload) Add(n int64) bool {
	l := u.l
	l.mu.Lock()
	defer l.mu.Unlock()

	if limit := GlobalConfig.MaxStorageBytes; limit > 0 && l.storedBytes()+l.inFlight+n > limit {
		return false
	}
	l.inFlight += n
	u.written += n
	return true
}

// Done releases the upload slot. Completed files are counted again by the
// directory scan, so the in-flight bytes are always released.
func (u *Upload) Done() {
	l := u.l
	l.mu.Lock()
	defer l.mu.Unlock()

	l.inFlight -= u.written
	if l.active[u.ip] <= 1 {
		delete(l.active, u.ip)
	} else {
		l.active[u.ip]--
	}
}

// storedBytes sums the completed uploads on disk. Partial *.tmp files are
// skipped because their bytes are already counted in inFlight. Only called
// when a storage cap is configured.
func (l *UploadLimiter) storedBytes() int64 {
	entries, err := os.ReadDir(l.uploadDir)
	if err != nil {
		log.Printf("Failed to read upload directory: %v", err)
		return 0
	}
	var total int64
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		if info, err := e.Info(); err == nil {
			total += info.Size()
		}
	}
	return total
}
