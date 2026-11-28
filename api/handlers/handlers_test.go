package handlers

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	if err := InitConfig(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestValidateID(t *testing.T) {
	for _, id := range []string{
		strings.Repeat("a", 16), strings.Repeat("F", 24), strings.Repeat("0", 32),
		strings.Repeat("b", 48), strings.Repeat("c", 64),
	} {
		if !validateID(id) {
			t.Errorf("validateID(%q) = false, want true", id)
		}
	}
	for _, id := range []string{
		"", strings.Repeat("a", 15), strings.Repeat("a", 33),
		"../../../../etc/", strings.Repeat("g", 16), strings.Repeat(".", 32),
	} {
		if validateID(id) {
			t.Errorf("validateID(%q) = true, want false", id)
		}
	}
}

func TestValidateToken(t *testing.T) {
	for _, tok := range []string{strings.Repeat("A", 22), strings.Repeat("a-_9", 10) + "abc"} {
		if !validateToken(tok) {
			t.Errorf("validateToken(%q) = false, want true", tok)
		}
	}
	for _, tok := range []string{"", strings.Repeat("A", 21), strings.Repeat("A", 44), strings.Repeat("A", 21) + "/", strings.Repeat("A", 21) + "."} {
		if validateToken(tok) {
			t.Errorf("validateToken(%q) = true, want false", tok)
		}
	}
}

func TestCheckOrigin(t *testing.T) {
	cases := []struct {
		origin, host string
		want         bool
	}{
		{"", "paste.example", true}, // non-browser client
		{"https://paste.example", "paste.example", true},
		{"https://evil.example", "paste.example", false},
		{"null", "paste.example", false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/api/ws/upload", nil)
		r.Host = tc.host
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if got := checkOrigin(r); got != tc.want {
			t.Errorf("checkOrigin(origin=%q, host=%q) = %v, want %v", tc.origin, tc.host, got, tc.want)
		}
	}
}

func TestUploadLimiterPerIP(t *testing.T) {
	l := NewUploadLimiter(t.TempDir())
	var held []*Upload
	for i := 0; i < GlobalConfig.UploadsPerIP; i++ {
		u, reason := l.Begin("1.2.3.4", 0)
		if u == nil {
			t.Fatalf("upload %d rejected: %s", i, reason)
		}
		held = append(held, u)
	}
	if u, _ := l.Begin("1.2.3.4", 0); u != nil {
		t.Fatal("expected per-IP limit to reject")
	}
	if u, _ := l.Begin("5.6.7.8", 0); u == nil {
		t.Fatal("other IP should be admitted")
	} else {
		u.Done()
	}
	held[0].Done()
	if u, _ := l.Begin("1.2.3.4", 0); u == nil {
		t.Fatal("slot should be free after Done")
	}
}

func TestUploadLimiterStorageCap(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "stored"), make([]byte, 600), 0600)
	os.WriteFile(filepath.Join(dir, "partial.tmp"), make([]byte, 5000), 0600) // counted via in-flight, not disk

	prev := GlobalConfig.MaxStorageBytes
	GlobalConfig.MaxStorageBytes = 1000
	defer func() { GlobalConfig.MaxStorageBytes = prev }()

	l := NewUploadLimiter(dir)
	if u, _ := l.Begin("ip", 500); u != nil {
		t.Fatal("declared size over the cap should be rejected")
	}
	a, _ := l.Begin("ip", 100)
	b, _ := l.Begin("ip", 100)
	if !a.Add(300) {
		t.Fatal("first 300 bytes fit")
	}
	if b.Add(200) {
		t.Fatal("parallel upload must not overshoot the cap")
	}
	a.Done()
	if !b.Add(200) {
		t.Fatal("space should be released after Done")
	}
	b.Done()
}

// newTestServer serves the WebSocket and HTTP handlers from a temp upload dir.
func newTestServer(t *testing.T) (*httptest.Server, string) {
	dir := t.TempDir()
	r := gin.New()
	limiter := NewUploadLimiter(dir)
	downloads := NewDownloadStore()
	r.GET("/api/ws/upload", HandleWSUpload(dir, nil, limiter, downloads))
	r.GET("/api/download/:id", HandleDownload(dir, downloads, limiter))
	r.GET("/api/metadata/:id", HandleMetadata(dir, downloads))
	r.DELETE("/api/delete/:id", HandleDelete(dir, downloads, limiter))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, dir
}

// uploadWithDownloads runs the client side of the upload protocol with an
// optional maxDownloads value (nil omits the field, using the server default of 1).
func uploadWithDownloads(t *testing.T, srv *httptest.Server, fileID, token string, maxDownloads *int, header []byte, chunks ...[]byte) map[string]any {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/api/ws/upload", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	var m map[string]any
	step := func(v any) bool {
		switch v := v.(type) {
		case []byte:
			ws.WriteMessage(websocket.BinaryMessage, v)
		default:
			ws.WriteJSON(v)
		}
		m = nil
		if err := ws.ReadJSON(&m); err != nil {
			m = map[string]any{"type": "closed"}
		}
		return m["type"] != "error" && m["type"] != "closed"
	}

	initPayload := map[string]any{"type": "init", "size": 100, "fileId": fileID}
	if maxDownloads != nil {
		initPayload["maxDownloads"] = *maxDownloads
	}
	if !step(initPayload) ||
		!step(map[string]any{"type": "token", "token": token}) ||
		!step(header) {
		return m
	}
	ws.WriteMessage(websocket.BinaryMessage, make([]byte, 12)) // IV
	for _, c := range chunks {
		if !step(c) {
			return m
		}
	}
	step([]byte{0})
	return m
}

func upload(t *testing.T, srv *httptest.Server, fileID, token string, header []byte, chunks ...[]byte) map[string]any {
	return uploadWithDownloads(t, srv, fileID, token, nil, header, chunks...)
}

func metadataHeader(ciphertextLen int) []byte {
	h := make([]byte, 16+ciphertextLen)
	binary.LittleEndian.PutUint32(h[12:16], uint32(ciphertextLen))
	return h
}

func TestWSUploadRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	token := strings.Repeat("T", 43)
	chunk := []byte(strings.Repeat("c", 64))

	m := upload(t, srv, "", token, metadataHeader(20), chunk, chunk)
	if m["type"] != "complete" {
		t.Fatalf("upload failed: %v", m)
	}
	id := m["id"].(string)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/download/"+id, nil)
	req.Header.Set("X-HMAC-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if want := int64(16 + 20 + 12 + 2*64); resp.StatusCode != 200 || resp.ContentLength != want {
		t.Fatalf("download: status %d, length %d, want 200/%d", resp.StatusCode, resp.ContentLength, want)
	}

	req.Header.Set("X-HMAC-Token", strings.Repeat("W", 43))
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong token: status %d, want 403", resp.StatusCode)
	}
}

func TestWSUploadRejectsMismatchedHeaderLength(t *testing.T) {
	srv, dir := newTestServer(t)
	h := metadataHeader(20)
	h = append(h, 1, 2, 3) // trailing bytes would corrupt the stored layout
	if m := upload(t, srv, "", strings.Repeat("T", 43), h); m["type"] != "error" {
		t.Fatalf("expected error, got %v", m)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("partial upload left files behind: %v", entries)
	}
}

func TestWSUploadPassphraseIDIsExclusive(t *testing.T) {
	srv, dir := newTestServer(t)
	id := strings.Repeat("ab", 16)

	var wg sync.WaitGroup
	results := make([]map[string]any, 2)
	for i, tok := range []string{strings.Repeat("B", 43), strings.Repeat("C", 43)} {
		wg.Add(1)
		go func(i int, tok string) {
			defer wg.Done()
			results[i] = upload(t, srv, id, tok, metadataHeader(20), []byte(strings.Repeat("c", 32)))
		}(i, tok)
	}
	wg.Wait()

	completed := 0
	for _, m := range results {
		if m["type"] == "complete" {
			completed++
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, id+".*"))
	dataFiles := 0
	for _, match := range matches {
		if !strings.HasSuffix(match, ".meta") {
			dataFiles++
		}
	}
	if completed != 1 || dataFiles != 1 {
		t.Fatalf("want exactly one upload for a shared ID, got %d completed, files %v", completed, matches)
	}
}

func TestDownloadLimitsAndPreviews(t *testing.T) {
	srv, dir := newTestServer(t)
	token := strings.Repeat("T", 43)
	chunk := []byte(strings.Repeat("c", 64))
	maxDl := 2

	m := uploadWithDownloads(t, srv, "", token, &maxDl, metadataHeader(20), chunk)
	if m["type"] != "complete" {
		t.Fatalf("upload failed: %v", m)
	}
	id := m["id"].(string)

	// Verify raw token is never exposed in filenames on disk.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), token) {
			t.Fatalf("raw token leaked in filename: %s", e.Name())
		}
	}

	doReq := func(method, path, purpose string) *http.Response {
		req, _ := http.NewRequest(method, srv.URL+path, nil)
		req.Header.Set("X-HMAC-Token", token)
		if purpose != "" {
			req.Header.Set("X-Paste-Purpose", purpose)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// Metadata reports 2 remaining.
	resp := doReq(http.MethodGet, "/api/metadata/"+id, "")
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Downloads-Remaining") != "2" || resp.Header.Get("X-Max-Downloads") != "2" {
		t.Fatalf("metadata: status=%d rem=%q max=%q", resp.StatusCode, resp.Header.Get("X-Downloads-Remaining"), resp.Header.Get("X-Max-Downloads"))
	}

	// Preview does not consume a download.
	resp = doReq(http.MethodGet, "/api/download/"+id, "preview")
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Downloads-Remaining") != "2" {
		t.Fatalf("preview: status=%d rem=%q", resp.StatusCode, resp.Header.Get("X-Downloads-Remaining"))
	}

	// Download #1 leaves 1 remaining.
	resp = doReq(http.MethodGet, "/api/download/"+id, "")
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Downloads-Remaining") != "1" {
		t.Fatalf("download 1: status=%d rem=%q", resp.StatusCode, resp.Header.Get("X-Downloads-Remaining"))
	}

	// Non-force DELETE retains the file while 1 download remains.
	resp = doReq(http.MethodDelete, "/api/delete/"+id, "")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("non-force delete: status=%d", resp.StatusCode)
	}

	// Download #2 uses the last slot and deletes the file.
	resp = doReq(http.MethodGet, "/api/download/"+id, "")
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Downloads-Remaining") != "0" {
		t.Fatalf("download 2: status=%d rem=%q", resp.StatusCode, resp.Header.Get("X-Downloads-Remaining"))
	}

	// Download #3 is rejected.
	resp = doReq(http.MethodGet, "/api/download/"+id, "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("download 3: status=%d, want 403", resp.StatusCode)
	}
}

func TestCustomFileIDRestrictedToPassphraseLength(t *testing.T) {
	srv, _ := newTestServer(t)
	token := strings.Repeat("T", 43)
	// 16-char hex ID (64-bit random ID length) must be rejected as custom fileId.
	if m := upload(t, srv, strings.Repeat("a", 16), token, metadataHeader(20)); m["type"] != "error" {
		t.Fatalf("expected error for non-32-char custom fileId, got %v", m)
	}
	// Uppercase hex must also be rejected for passphrase IDs.
	if m := upload(t, srv, strings.Repeat("A", 32), token, metadataHeader(20)); m["type"] != "error" {
		t.Fatalf("expected error for uppercase custom fileId, got %v", m)
	}
}

func TestParseFileSize(t *testing.T) {
	ok := map[string]int64{
		"100MB": 100 << 20, "100mb": 100 << 20, "5GiB": 5 << 30, "500Mi": 500 << 20,
		"1073741824": 1 << 30, "2 GB": 2 << 30, "1.5KB": 1536, "10B": 10, "1T": 1 << 40,
	}
	for in, want := range ok {
		if got, err := parseFileSize(in); err != nil || got != want {
			t.Errorf("parseFileSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "MB", "-1MB", "0", "1PB", "1XB", "abc"} {
		if got, err := parseFileSize(in); err == nil {
			t.Errorf("parseFileSize(%q) = %d, want error", in, got)
		}
	}
}

func TestUploadLimiterZeroMeansUnlimited(t *testing.T) {
	prev := GlobalConfig.UploadsPerIP
	GlobalConfig.UploadsPerIP = 0
	defer func() { GlobalConfig.UploadsPerIP = prev }()

	l := NewUploadLimiter(t.TempDir())
	for i := 0; i < 50; i++ {
		if u, reason := l.Begin("1.2.3.4", 0); u == nil {
			t.Fatalf("upload %d rejected with limit disabled: %s", i, reason)
		}
	}
}
