package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content)), Typeflag: tar.TypeReg})
	tw.Write(content)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(name)
	w.Write(content)
	zw.Close()
	return buf.Bytes()
}

// fakeRelease serves a GitHub-like releases API with the given archives.
// sums overrides the generated SHA256SUMS when non-nil.
type fakeRelease struct {
	tag      string
	archives map[string][]byte
	sums     []byte
}

func (f *fakeRelease) server(t *testing.T) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/"+repo+"/releases/latest" || r.URL.Path == "/repos/"+repo+"/releases/tags/"+f.tag:
			type asset struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			}
			var assets []asset
			for name := range f.archives {
				assets = append(assets, asset{name, srv.URL + "/dl/" + name})
			}
			assets = append(assets, asset{checksumsAsset, srv.URL + "/dl/" + checksumsAsset})
			json.NewEncoder(w).Encode(map[string]any{"tag_name": f.tag, "assets": assets})
		case r.URL.Path == "/dl/"+checksumsAsset:
			if f.sums != nil {
				w.Write(f.sums)
				return
			}
			for name, data := range f.archives {
				fmt.Fprintf(w, "%x  %s\n", sha256.Sum256(data), name)
			}
		case strings.HasPrefix(r.URL.Path, "/dl/"):
			data, ok := f.archives[strings.TrimPrefix(r.URL.Path, "/dl/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newTestUpdater targets a fake installed binary in a temp dir.
func newTestUpdater(t *testing.T, srv *httptest.Server, goos string) (*Updater, string, *bytes.Buffer) {
	exe := filepath.Join(t.TempDir(), "pastectl")
	if err := os.WriteFile(exe, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	u := New(&out)
	u.APIBase = srv.URL
	u.GOOS, u.GOARCH = goos, "amd64"
	u.Executable = func() (string, error) { return exe, nil }
	u.VerifyBinary = nil
	return u, exe, &out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUpdateInstallsVerifiedRelease(t *testing.T) {
	rel := &fakeRelease{tag: "v1.3.0", archives: map[string][]byte{
		"pastectl-linux-amd64.tar.gz": tarGz(t, "pastectl", []byte("new binary")),
	}}
	u, exe, out := newTestUpdater(t, rel.server(t), "linux")

	if err := u.Run(context.Background(), "1.2.1", Options{}); err != nil {
		t.Fatalf("update failed: %v\n%s", err, out)
	}
	if got := readFile(t, exe); got != "new binary" {
		t.Fatalf("binary = %q, want new binary", got)
	}
	if info, _ := os.Stat(exe); info.Mode().Perm()&0111 == 0 {
		t.Fatal("installed binary is not executable")
	}
	if !strings.Contains(out.String(), "Checksum verified") {
		t.Errorf("output missing checksum confirmation: %s", out)
	}
	if entries, _ := os.ReadDir(filepath.Dir(exe)); len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

func TestUpdateRejectsChecksumMismatch(t *testing.T) {
	archive := tarGz(t, "pastectl", []byte("tampered binary"))
	rel := &fakeRelease{
		tag:      "v1.3.0",
		archives: map[string][]byte{"pastectl-linux-amd64.tar.gz": archive},
		sums:     []byte(fmt.Sprintf("%x  pastectl-linux-amd64.tar.gz\n", sha256.Sum256([]byte("something else")))),
	}
	u, exe, _ := newTestUpdater(t, rel.server(t), "linux")

	err := u.Run(context.Background(), "1.2.1", Options{})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
	if got := readFile(t, exe); got != "old binary" {
		t.Fatalf("binary was replaced despite mismatch: %q", got)
	}
}

func TestUpdateRejectsMissingChecksum(t *testing.T) {
	rel := &fakeRelease{
		tag:      "v1.3.0",
		archives: map[string][]byte{"pastectl-linux-amd64.tar.gz": tarGz(t, "pastectl", []byte("new"))},
		sums:     []byte("0000  pastectl-darwin-arm64.tar.gz\n"),
	}
	u, exe, _ := newTestUpdater(t, rel.server(t), "linux")

	if err := u.Run(context.Background(), "1.2.1", Options{}); err == nil || !strings.Contains(err.Error(), "no checksum") {
		t.Fatalf("err = %v, want missing checksum error", err)
	}
	if readFile(t, exe) != "old binary" {
		t.Fatal("binary was replaced without a checksum")
	}
}

func TestUpdateUpToDateAndDevBuilds(t *testing.T) {
	rel := &fakeRelease{tag: "v1.2.1", archives: map[string][]byte{
		"pastectl-linux-amd64.tar.gz": tarGz(t, "pastectl", []byte("same")),
	}}
	srv := rel.server(t)

	u, exe, _ := newTestUpdater(t, srv, "linux")
	if err := u.Run(context.Background(), "1.2.1", Options{}); !errors.Is(err, ErrUpToDate) {
		t.Fatalf("same version: err = %v, want ErrUpToDate", err)
	}
	if err := u.Run(context.Background(), "1.3.0", Options{}); !errors.Is(err, ErrUpToDate) {
		t.Fatalf("newer local version: err = %v, want ErrUpToDate", err)
	}
	if err := u.Run(context.Background(), "dev", Options{}); err == nil || !strings.Contains(err.Error(), "development build") {
		t.Fatalf("dev build: err = %v, want refusal", err)
	}
	if readFile(t, exe) != "old binary" {
		t.Fatal("binary replaced without --force")
	}
	if err := u.Run(context.Background(), "1.2.1", Options{Force: true}); err != nil {
		t.Fatalf("--force: %v", err)
	}
	if readFile(t, exe) != "same" {
		t.Fatal("--force did not reinstall")
	}
}

func TestUpdateCheckOnlyDoesNotInstall(t *testing.T) {
	rel := &fakeRelease{tag: "v1.3.0", archives: map[string][]byte{
		"pastectl-linux-amd64.tar.gz": tarGz(t, "pastectl", []byte("new")),
	}}
	u, exe, out := newTestUpdater(t, rel.server(t), "linux")

	if err := u.Run(context.Background(), "1.2.1", Options{CheckOnly: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "v1.2.1 -> v1.3.0") {
		t.Errorf("output = %q", out)
	}
	if readFile(t, exe) != "old binary" {
		t.Fatal("--check installed the update")
	}
}

func TestUpdateWindowsZip(t *testing.T) {
	rel := &fakeRelease{tag: "v1.3.0", archives: map[string][]byte{
		"pastectl-windows-amd64.zip": zipArchive(t, "pastectl.exe", []byte("new exe")),
	}}
	u, exe, _ := newTestUpdater(t, rel.server(t), "windows")

	if err := u.Run(context.Background(), "1.2.1", Options{}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, exe) != "new exe" {
		t.Fatal("exe not replaced")
	}
	if readFile(t, exe+".old") != "old binary" {
		t.Fatal("previous exe not kept as .old")
	}
}

func TestUpdateVerificationFailureKeepsOldBinary(t *testing.T) {
	rel := &fakeRelease{tag: "v1.3.0", archives: map[string][]byte{
		"pastectl-linux-amd64.tar.gz": tarGz(t, "pastectl", []byte("broken")),
	}}
	u, exe, _ := newTestUpdater(t, rel.server(t), "linux")
	u.VerifyBinary = func(string) (string, error) { return "", errors.New("exec format error") }

	if err := u.Run(context.Background(), "1.2.1", Options{}); err == nil {
		t.Fatal("expected verification error")
	}
	if readFile(t, exe) != "old binary" {
		t.Fatal("binary replaced despite failed verification")
	}
	if entries, _ := os.ReadDir(filepath.Dir(exe)); len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

func TestGetRefusesPlainHTTP(t *testing.T) {
	u := New(io.Discard)
	if _, err := u.get(context.Background(), "http://example.com/x", 10, "*/*"); err == nil || !strings.Contains(err.Error(), "non-HTTPS") {
		t.Fatalf("err = %v, want non-HTTPS refusal", err)
	}
}

func TestExtractIgnoresOtherEntries(t *testing.T) {
	// A traversal-named entry must not be mistaken for the binary.
	archive := tarGz(t, "../pastectl", []byte("evil"))
	if _, err := extract(archive, "x.tar.gz", "pastectl"); err == nil {
		t.Fatal("expected binary not found")
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.1", "1.2.1", 0},
		{"1.10.0", "1.9.9", 1},
		{"v1.2.0", "1.2.1", -1},
		{"2.0", "1.99.99", 1},
		{"1.2.0-rc1", "1.2.0", 0},
		{"1.2", "1.2.0", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestPackageManager(t *testing.T) {
	if got := packageManager("/opt/homebrew/Cellar/pastectl/1.2.1/bin/pastectl"); got != "brew" {
		t.Errorf("Cellar path: %q", got)
	}
	if got := packageManager("/opt/homebrew/bin/pastectl"); got != "" {
		t.Errorf("manually copied binary should be updatable, got %q", got)
	}
	if got := packageManager("/usr/local/bin/pastectl"); got != "" {
		t.Errorf("plain install: %q", got)
	}
}

func TestUpdateWarnsOnVersionMismatch(t *testing.T) {
	rel := &fakeRelease{tag: "v1.2.0", archives: map[string][]byte{
		"pastectl-linux-amd64.tar.gz": tarGz(t, "pastectl", []byte("old release")),
	}}
	u, exe, out := newTestUpdater(t, rel.server(t), "linux")
	u.VerifyBinary = func(string) (string, error) { return "pastectl vdev", nil }

	if err := u.Run(context.Background(), "1.2.1", Options{Version: "v1.2.0", Force: true}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, exe) != "old release" {
		t.Fatal("pinned release not installed")
	}
	if !strings.Contains(out.String(), `reports "pastectl vdev"`) {
		t.Errorf("missing mismatch note: %s", out)
	}
}

func TestRunVersionRejectsNonPastectl(t *testing.T) {
	// "echo version" runs fine but is not pastectl.
	if _, err := runVersion("/bin/echo"); err == nil || !strings.Contains(err.Error(), "unexpected version output") {
		t.Fatalf("err = %v, want unexpected version output", err)
	}
}
