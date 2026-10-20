// Package update replaces the running pastectl binary with a GitHub release.
//
// The archive for the current OS/architecture is downloaded over HTTPS and
// its SHA-256 is checked against the release's SHA256SUMS before anything is
// extracted. This catches corrupted, truncated or mismatched downloads; it
// does not protect against a compromised release, since the checksums are
// published alongside the archives.
package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIBase = "https://api.github.com"
	repo           = "jonasbg/paste"
	checksumsAsset = "SHA256SUMS"

	maxMetadataSize = 1 << 20   // release JSON and SHA256SUMS
	maxArchiveSize  = 100 << 20 // release archives are ~7 MB
	maxBinarySize   = 100 << 20

	verifyTimeout = time.Minute
)

// ErrUpToDate is returned when the installed version is already current.
var ErrUpToDate = errors.New("already up to date")

// Options control a single update run.
type Options struct {
	// CheckOnly reports the available version without installing it.
	CheckOnly bool
	// Version installs a specific release tag (e.g. "v1.2.1") instead of the latest.
	Version string
	// Force reinstalls even when the target is not newer, and allows
	// replacing development builds.
	Force bool
}

// Updater downloads and installs releases. The zero value is not usable;
// call New.
type Updater struct {
	APIBase string
	Client  *http.Client
	GOOS    string
	GOARCH  string
	Out     io.Writer

	// Executable returns the path of the binary to replace.
	Executable func() (string, error)
	// VerifyBinary checks that the extracted binary runs on this machine
	// before it is installed. It returns the version the binary reports.
	VerifyBinary func(path string) (string, error)
}

// New returns an Updater for the running binary.
func New(out io.Writer) *Updater {
	return &Updater{
		APIBase:      defaultAPIBase,
		Client:       &http.Client{Timeout: 5 * time.Minute},
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		Out:          out,
		Executable:   currentExecutable,
		VerifyBinary: runVersion,
	}
}

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (r *release) assetURL(name string) (string, error) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL, nil
		}
	}
	return "", fmt.Errorf("release %s has no asset %s", r.TagName, name)
}

// AssetName is the release archive for an OS/architecture pair.
func AssetName(goos, goarch string) string {
	if goos == "windows" {
		return fmt.Sprintf("pastectl-%s-%s.zip", goos, goarch)
	}
	return fmt.Sprintf("pastectl-%s-%s.tar.gz", goos, goarch)
}

// Run updates the binary from currentVersion according to opts.
func (u *Updater) Run(ctx context.Context, currentVersion string, opts Options) error {
	rel, err := u.fetchRelease(ctx, opts.Version)
	if err != nil {
		return err
	}
	target := strings.TrimPrefix(rel.TagName, "v")
	current := strings.TrimPrefix(currentVersion, "v")

	newer := current == "dev" || compareVersions(target, current) > 0
	if opts.CheckOnly {
		if newer && current != "dev" {
			fmt.Fprintf(u.Out, "Update available: v%s -> v%s (run 'pastectl update')\n", current, target)
		} else {
			fmt.Fprintf(u.Out, "Current: %s, latest release: v%s\n", displayVersion(current), target)
		}
		return nil
	}
	if !opts.Force {
		if current == "dev" {
			return fmt.Errorf("this is a development build; use --force to replace it with v%s", target)
		}
		if !newer {
			return ErrUpToDate
		}
	}

	exe, err := u.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate the running binary: %w", err)
	}
	if managedBy := packageManager(exe); managedBy != "" {
		return fmt.Errorf("%s is managed by %s; update it with %s instead", exe, managedBy, managedBy)
	}

	asset := AssetName(u.GOOS, u.GOARCH)
	fmt.Fprintf(u.Out, "Downloading %s (v%s)...\n", asset, target)

	sums, err := u.download(ctx, rel, checksumsAsset, maxMetadataSize)
	if err != nil {
		return err
	}
	want, err := checksumFor(sums, asset)
	if err != nil {
		return err
	}

	archive, err := u.download(ctx, rel, asset, maxArchiveSize)
	if err != nil {
		return err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %x; not installing", asset, want, got)
	}
	fmt.Fprintf(u.Out, "Checksum verified (sha256 %s)\n", want)

	binName := "pastectl"
	if u.GOOS == "windows" {
		binName = "pastectl.exe"
	}
	binary, err := extract(archive, asset, binName)
	if err != nil {
		return err
	}

	if err := u.install(exe, binary, target); err != nil {
		return err
	}
	fmt.Fprintf(u.Out, "Updated %s: %s -> v%s\n", exe, displayVersion(current), target)
	return nil
}

func displayVersion(v string) string {
	if v == "dev" {
		return "dev"
	}
	return "v" + v
}

func (u *Updater) fetchRelease(ctx context.Context, version string) (*release, error) {
	endpoint := u.APIBase + "/repos/" + repo + "/releases/latest"
	if version != "" {
		if !strings.HasPrefix(version, "v") {
			version = "v" + version
		}
		endpoint = u.APIBase + "/repos/" + repo + "/releases/tags/" + url.PathEscape(version)
	}

	body, err := u.get(ctx, endpoint, maxMetadataSize, "application/vnd.github+json")
	if err != nil {
		return nil, fmt.Errorf("failed to look up release: %w", err)
	}
	var rel release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("invalid release metadata: %w", err)
	}
	if rel.TagName == "" {
		return nil, errors.New("invalid release metadata: missing tag")
	}
	return &rel, nil
}

func (u *Updater) download(ctx context.Context, rel *release, name string, limit int64) ([]byte, error) {
	assetURL, err := rel.assetURL(name)
	if err != nil {
		return nil, err
	}
	body, err := u.get(ctx, assetURL, limit, "application/octet-stream")
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", name, err)
	}
	return body, nil
}

// get fetches rawURL over HTTPS (plain HTTP only for loopback test servers)
// and fails if the body exceeds limit.
func (u *Updater) get(ctx context.Context, rawURL string, limit int64, accept string) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "https" && !isLoopback(parsed.Hostname()) {
		return nil, fmt.Errorf("refusing non-HTTPS URL %s", rawURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "pastectl-updater")

	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", rawURL, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", rawURL, limit)
	}
	return body, nil
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// checksumFor finds asset in a sha256sum-style file ("<hex>  <name>").
func checksumFor(sums []byte, asset string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*") // binary-mode marker
		if name != asset {
			continue
		}
		sum := strings.ToLower(fields[0])
		if len(sum) != sha256.Size*2 {
			return "", fmt.Errorf("malformed checksum for %s", asset)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return "", fmt.Errorf("malformed checksum for %s", asset)
		}
		return sum, nil
	}
	return "", fmt.Errorf("%s has no checksum for %s; not installing", checksumsAsset, asset)
}

// extract returns the named binary from a .tar.gz or .zip archive. Only a
// top-level regular file with exactly that name is accepted.
func extract(archive []byte, archiveName, binName string) ([]byte, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, fmt.Errorf("invalid zip archive: %w", err)
		}
		for _, f := range zr.File {
			if f.Name != binName || !f.Mode().IsRegular() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return readLimited(rc, binName)
		}
		return nil, fmt.Errorf("%s not found in %s", binName, archiveName)
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("invalid gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s not found in %s", binName, archiveName)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid tar archive: %w", err)
		}
		if hdr.Name == binName && hdr.Typeflag == tar.TypeReg {
			return readLimited(tr, binName)
		}
	}
}

func readLimited(r io.Reader, name string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBinarySize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBinarySize {
		return nil, fmt.Errorf("%s exceeds %d bytes", name, maxBinarySize)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	return data, nil
}

// install writes binary next to exe, verifies it, and swaps it into place.
func (u *Updater) install(exe string, binary []byte, version string) error {
	mode := os.FileMode(0755)
	if info, err := os.Stat(exe); err == nil {
		mode = info.Mode().Perm() | 0111
	}

	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".pastectl-update-*")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("no permission to write to %s; re-run with sudo or reinstall manually", dir)
		}
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once renamed into place

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}

	if u.VerifyBinary != nil {
		reported, err := u.VerifyBinary(tmpPath)
		if err != nil {
			return fmt.Errorf("downloaded binary failed verification, not installing: %w", err)
		}
		// Releases built before version injection was fixed report "vdev";
		// the checksum already proves this is the published asset.
		if !strings.Contains(reported, version) {
			fmt.Fprintf(u.Out, "Note: v%s binary reports %q\n", version, reported)
		}
	}

	if u.GOOS == "windows" {
		// A running .exe cannot be overwritten, but it can be renamed.
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return fmt.Errorf("failed to move current binary aside: %w", err)
		}
		if err := os.Rename(tmpPath, exe); err != nil {
			os.Rename(old, exe)
			return fmt.Errorf("failed to install new binary: %w", err)
		}
		return nil
	}

	if err := os.Rename(tmpPath, exe); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("no permission to replace %s; re-run with sudo or reinstall manually", exe)
		}
		return fmt.Errorf("failed to install new binary: %w", err)
	}
	return nil
}

func currentExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// packageManager reports a package manager that owns exe, if any, so the
// updater does not fight it.
func packageManager(exe string) string {
	p := filepath.ToSlash(exe)
	switch {
	case strings.Contains(p, "/Cellar/"):
		return "brew"
	case strings.Contains(p, "/nix/store/"):
		return "nix"
	case strings.Contains(p, "/scoop/apps/"):
		return "scoop"
	}
	return ""
}

// runVersion executes the new binary's version command, which proves it
// runs on this OS/architecture and is pastectl.
func runVersion(path string) (string, error) {
	// Generous timeout: macOS scans a new binary on its first run, which
	// can take well over 10 seconds.
	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "version").CombinedOutput()
	reported := strings.TrimSpace(string(out))
	if ctx.Err() != nil {
		return "", fmt.Errorf("running %s version timed out after %s", filepath.Base(path), verifyTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("running %s version: %w", filepath.Base(path), err)
	}
	if !strings.HasPrefix(reported, "pastectl v") {
		return "", fmt.Errorf("unexpected version output %q", reported)
	}
	return reported, nil
}

// compareVersions compares dotted numeric versions ("1.10.0" > "1.9.3").
// Pre-release suffixes ("1.2.0-rc1") are ignored, and non-numeric parts
// compare as 0.
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var parts []int
	for _, s := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(s)
		parts = append(parts, n)
	}
	return parts
}
