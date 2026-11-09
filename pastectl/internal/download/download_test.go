package download

import "testing"

func TestSafeFilename(t *testing.T) {
	ok := map[string]string{
		"report.pdf":                   "report.pdf",
		"../../.bashrc":                ".bashrc",
		"/etc/cron.d/evil":             "evil",
		`..\..\Windows\evil.bat`:       "evil.bat",
		"/home/u/.ssh/authorized_keys": "authorized_keys",
	}
	for in, want := range ok {
		got, err := safeFilename(in)
		if err != nil || got != want {
			t.Errorf("safeFilename(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", ".", "..", "dir/", "/", "a/..", "bad\x00name"} {
		if got, err := safeFilename(in); err == nil {
			t.Errorf("safeFilename(%q) = %q, want error", in, got)
		}
	}
}
