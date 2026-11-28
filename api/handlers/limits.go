package handlers

import (
	"log"
	"os"
	"strings"
	"sync"
)

// UploadLimiter bounds concurrent uploads per client IP and, when
// MAX_STORAGE_SIZE is set, the total bytes kept in the upload directory.
// Completed and in-flight byte totals are maintained in memory in O(1) so
// parallel uploads never stall on per-chunk directory scans.
type UploadLimiter struct {
	uploadDir string

	mu       sync.Mutex
	active   map[string]int
	stored   int64
	inFlight int64
}

func NewUploadLimiter(uploadDir string) *UploadLimiter {
	l := &UploadLimiter{
		uploadDir: uploadDir,
		active:    make(map[string]int),
	}
	l.stored = l.scanStoredBytes()
	return l
}

// Upload is one admitted upload. Call Done when it finishes, successfully or not.
type Upload struct {
	l       *UploadLimiter
	ip      string
	written int64
	done    bool
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
		if l.stored+l.inFlight+size > limit {
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

	if limit := GlobalConfig.MaxStorageBytes; limit > 0 && l.stored+l.inFlight+n > limit {
		return false
	}
	l.inFlight += n
	u.written += n
	return true
}

// Commit moves this upload's bytes from inFlight to stored after the file has
// been atomically renamed into its final location.
func (u *Upload) Commit(finalSize int64) {
	l := u.l
	l.mu.Lock()
	defer l.mu.Unlock()

	l.inFlight -= u.written
	if l.inFlight < 0 {
		l.inFlight = 0
	}
	u.written = 0
	if finalSize > 0 {
		l.stored += finalSize
	}
}

// Done releases the upload slot and any uncommitted in-flight bytes.
func (u *Upload) Done() {
	l := u.l
	l.mu.Lock()
	defer l.mu.Unlock()

	if u.done {
		return
	}
	u.done = true

	l.inFlight -= u.written
	if l.inFlight < 0 {
		l.inFlight = 0
	}
	u.written = 0

	if l.active[u.ip] <= 1 {
		delete(l.active, u.ip)
	} else {
		l.active[u.ip]--
	}
}

// RecordDeleted subtracts bytes removed from disk (e.g. after download exhaustion or DELETE).
func (l *UploadLimiter) RecordDeleted(size int64) {
	if l == nil || size <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.stored -= size
	if l.stored < 0 {
		l.stored = 0
	}
}

// SyncStoredBytes recalculates the stored byte total from disk without holding
// the lock during directory traversal. Intended to run after periodic cleanup sweeps.
func (l *UploadLimiter) SyncStoredBytes() {
	if l == nil {
		return
	}
	total := l.scanStoredBytes()
	l.mu.Lock()
	l.stored = total
	l.mu.Unlock()
}

// scanStoredBytes sums the completed uploads on disk. Partial *.tmp, *.reserve,
// and *.meta sidecar files are skipped.
func (l *UploadLimiter) scanStoredBytes() int64 {
	entries, err := os.ReadDir(l.uploadDir)
	if err != nil {
		log.Printf("Failed to read upload directory: %v", err)
		return 0
	}
	var total int64
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".reserve") || strings.HasSuffix(name, ".meta") {
			continue
		}
		if info, err := e.Info(); err == nil {
			total += info.Size()
		}
	}
	return total
}
