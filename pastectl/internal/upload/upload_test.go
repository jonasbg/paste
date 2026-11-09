package upload

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/jonasbg/paste/pastectl/internal/types"
)

// fakeUploadServer runs the server side of the upload protocol and lets the
// test choose the reply to each client step (init, token, header, chunk, end).
func fakeUploadServer(t *testing.T, replies map[string]any) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	reply := func(conn *websocket.Conn, step string, fallback any) bool {
		msg, ok := replies[step]
		if !ok {
			msg = fallback
		}
		if err := conn.WriteJSON(msg); err != nil {
			return false
		}
		// An error frame ends the session, as on the real server.
		m, _ := msg.(map[string]any)
		return m["type"] != "error"
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// init → id
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		if !reply(conn, "init", map[string]any{"type": "id", "id": "abcdefghijklmnop"}) {
			return
		}
		// token → token_accepted
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		if !reply(conn, "token", map[string]any{"type": "token_accepted"}) {
			return
		}
		// metadata header → ready
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		if !reply(conn, "header", map[string]any{"type": "ready"}) {
			return
		}
		// IV (no reply)
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		// chunks → ack, until the single 0x00 end marker → complete
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if len(data) == 1 && data[0] == 0 {
				reply(conn, "end", map[string]any{"type": "complete", "id": "abcdefghijklmnop"})
				return
			}
			if !reply(conn, "chunk", map[string]any{"type": "ack", "ack": len(data)}) {
				return
			}
		}
	}))
}

func runUpload(t *testing.T, srv *httptest.Server) (string, error) {
	t.Helper()
	h := NewHandler(srv.URL, &types.Config{ChunkSize: 1, KeySize: 256})
	data := []byte("hello paste")
	key := bytes.Repeat([]byte{7}, 32)
	return h.uploadFile(bytes.NewReader(data), "hello.txt", "text/plain", int64(len(data)), key)
}

func TestUploadSucceedsOnComplete(t *testing.T) {
	srv := fakeUploadServer(t, nil)
	defer srv.Close()

	id, err := runUpload(t, srv)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if id != "abcdefghijklmnop" {
		t.Fatalf("got id %q", id)
	}
}

// Regression: the server reports a failed finalize (e.g. os.Rename failing) as
// an error frame after the end marker. The CLI used to read it as the final
// response without checking its type and print a share link to a file that
// was never saved.
func TestUploadFailsWhenServerCannotSaveFile(t *testing.T) {
	srv := fakeUploadServer(t, map[string]any{
		"end": map[string]any{"type": "error", "error": "Failed to save file"},
	})
	defer srv.Close()

	_, err := runUpload(t, srv)
	if err == nil || !strings.Contains(err.Error(), "Failed to save file") {
		t.Fatalf("expected the server's save error, got %v", err)
	}
}

func TestUploadSurfacesServerErrorsAtEachStep(t *testing.T) {
	for _, step := range []string{"init", "token", "header", "chunk"} {
		t.Run(step, func(t *testing.T) {
			srv := fakeUploadServer(t, map[string]any{
				step: map[string]any{"type": "error", "error": "boom at " + step},
			})
			defer srv.Close()

			_, err := runUpload(t, srv)
			if err == nil || !strings.Contains(err.Error(), "boom at "+step) {
				t.Fatalf("expected server error for step %s, got %v", step, err)
			}
		})
	}
}

func TestUploadRejectsOutOfOrderReply(t *testing.T) {
	srv := fakeUploadServer(t, map[string]any{
		"token": map[string]any{"type": "ready"},
	})
	defer srv.Close()

	_, err := runUpload(t, srv)
	if err == nil || !strings.Contains(err.Error(), `unexpected server message "ready"`) {
		t.Fatalf("expected out-of-order error, got %v", err)
	}
}
