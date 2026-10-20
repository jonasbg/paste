package middleware

import (
	"path/filepath"
	"testing"
)

func TestResolveInDirStaysInRoot(t *testing.T) {
	root := filepath.FromSlash("/srv/web")
	cases := map[string]string{
		"/app.js":                  "app.js",
		"/../secret/creds.js":      "secret/creds.js",
		"../../etc/passwd.css":     "etc/passwd.css",
		"/assets/../../../x.woff2": "x.woff2",
		"":                         "",
	}
	for in, wantRel := range cases {
		rel, full := resolveInDir(root, in)
		if rel != wantRel {
			t.Errorf("resolveInDir(%q) rel = %q, want %q", in, rel, wantRel)
		}
		if want := filepath.Join(root, filepath.FromSlash(wantRel)); full != want {
			t.Errorf("resolveInDir(%q) full = %q, want %q", in, full, want)
		}
	}
}
