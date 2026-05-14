package sqlitecli

import (
	"strings"
	"testing"
)

func TestImmutableURI(t *testing.T) {
	got := ImmutableURI("/tmp/a b/contact.db")
	if !strings.HasPrefix(got, "file:///tmp/a%20b/contact.db?") {
		t.Fatalf("uri = %q", got)
	}
	if !strings.Contains(got, "mode=ro") || !strings.Contains(got, "immutable=1") {
		t.Fatalf("uri missing readonly options: %q", got)
	}
}
