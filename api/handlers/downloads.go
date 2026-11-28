package handlers

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// downloadMeta persists the allowed and completed download counts for a stored file.
// MaxDownloads == 0 means unlimited (until retention expiry).
type downloadMeta struct {
	MaxDownloads int `json:"max_downloads"`
	Downloads    int `json:"downloads"`
}

// DownloadStore manages per-file download limits using companion .meta files.
// All mutations are serialized through mu so concurrent downloads cannot race
// past the configured limit.
type DownloadStore struct {
	mu sync.Mutex
}

func NewDownloadStore() *DownloadStore {
	return &DownloadStore{}
}

func metaPath(filePath string) string {
	return filePath + ".meta"
}

// Init writes the initial download limit metadata for a newly uploaded file.
// maxDownloads == 0 represents unlimited downloads until retention expiry.
func (s *DownloadStore) Init(filePath string, maxDownloads int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return writeMetaAtomic(metaPath(filePath), downloadMeta{
		MaxDownloads: maxDownloads,
		Downloads:    0,
	})
}

// Peek returns the configured maxDownloads, remaining downloads (-1 for unlimited),
// and whether the file is still downloadable. Legacy files without a .meta file
// default to maxDownloads=1, remaining=1.
func (s *DownloadStore) Peek(filePath string) (maxDownloads int, remaining int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta := readMetaOrDefault(metaPath(filePath))
	if meta.MaxDownloads == 0 {
		return 0, -1, true
	}
	rem := meta.MaxDownloads - meta.Downloads
	if rem <= 0 {
		return meta.MaxDownloads, 0, false
	}
	return meta.MaxDownloads, rem, true
}

// Consume records one download attempt for filePath.
// Returns (remaining, maxDownloads, exhausted, allowed):
//   - allowed=false: the limit was already reached.
//   - exhausted=true: this download used the final allowed slot; caller should delete the file.
//   - remaining=-1 when maxDownloads == 0 (unlimited).
func (s *DownloadStore) Consume(filePath string) (remaining int, maxDownloads int, exhausted bool, allowed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mp := metaPath(filePath)
	meta := readMetaOrDefault(mp)

	if meta.MaxDownloads == 0 {
		meta.Downloads++
		_ = writeMetaAtomic(mp, meta)
		return -1, 0, false, true
	}

	if meta.Downloads >= meta.MaxDownloads {
		return 0, meta.MaxDownloads, true, false
	}

	meta.Downloads++
	rem := meta.MaxDownloads - meta.Downloads
	if rem <= 0 {
		return 0, meta.MaxDownloads, true, true
	}

	_ = writeMetaAtomic(mp, meta)
	return rem, meta.MaxDownloads, false, true
}

// RemoveMeta deletes the companion .meta file if present.
func (s *DownloadStore) RemoveMeta(filePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.Remove(metaPath(filePath))
}

func readMetaOrDefault(path string) downloadMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Legacy files without sidecar metadata default to single download.
			return downloadMeta{MaxDownloads: 1, Downloads: 0}
		}
		return downloadMeta{MaxDownloads: 1, Downloads: 0}
	}
	var m downloadMeta
	if err := json.Unmarshal(data, &m); err != nil || m.MaxDownloads < 0 || m.Downloads < 0 {
		return downloadMeta{MaxDownloads: 1, Downloads: 0}
	}
	return m
}

func writeMetaAtomic(path string, meta downloadMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".meta-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
