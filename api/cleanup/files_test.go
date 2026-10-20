package cleanup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanOldFilesRemovesOnlyExpired(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.token")
	fresh := filepath.Join(dir, "fresh.token")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().AddDate(0, 0, -8)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	if err := cleanOldFiles(dir, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expired file still present")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh file removed: %v", err)
	}
}
