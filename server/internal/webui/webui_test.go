package webui

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestHashInlineScripts(t *testing.T) {
	body := "\n\t\t\tstart();\n\t\t"
	html := []byte(`<head><script type="module" src="/_app/x.js"></script></head><body><script>` + body + `</script></body>`)

	got := hashInlineScripts(html)

	sum := sha256.Sum256([]byte(body))
	want := "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
	if len(got) != 1 || got[0] != want {
		t.Fatalf("hashInlineScripts = %v, want [%s]", got, want)
	}
}
