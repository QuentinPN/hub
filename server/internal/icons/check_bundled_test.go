package icons

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

// Every bundled file passes the same checks as a downloaded one.
func TestBundledIconsPassTheChecks(t *testing.T) {
	entries, err := fs.ReadDir(bundledFS, "bundled")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 30 {
		t.Fatalf("only %d bundled icons", len(entries))
	}
	for _, e := range entries {
		body, err := fs.ReadFile(bundledFS, path.Join("bundled", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		ct := "image/webp"
		if strings.HasSuffix(e.Name(), ".svg") {
			ct = "image/svg+xml"
		}
		if err := Check(ct, body); err != nil {
			t.Errorf("%s: %v", e.Name(), err)
		}
	}
}
