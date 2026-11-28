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

func TestCleanStaleTempFiles(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, "abc.hash.tmp")
	res := filepath.Join(dir, "abc.reserve")
	keep := filepath.Join(dir, "abc.hash")
	for _, p := range []string{tmp, res, keep} {
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := CleanStaleTempFiles(dir, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("stale .tmp was not removed")
	}
	if _, err := os.Stat(res); !os.IsNotExist(err) {
		t.Errorf("stale .reserve was not removed")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("completed file was removed: %v", err)
	}
}
