package contacts

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestListReturnsConfiguredContactFields(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is required for contact query tests")
	}
	db := filepath.Join(t.TempDir(), "contact.db")
	sql := `
CREATE TABLE contact (username TEXT, local_type INTEGER, alias TEXT, remark TEXT, nick_name TEXT, big_head_url TEXT);
INSERT INTO contact VALUES ('u1', 1, 'alias1', 'Remark 1', 'Nick 1', 'https://example.com/1');
INSERT INTO contact VALUES ('u2', 3, '', '', 'Nick 2', '');
INSERT INTO contact VALUES ('u3', 0, '', '', '', '');
`
	if out, err := exec.Command("sqlite3", db, sql).CombinedOutput(); err != nil {
		t.Fatalf("create db: %v: %s", err, out)
	}
	got, err := NewService(db).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d contacts, want 3", len(got))
	}

	byUser := map[string]Contact{}
	for _, c := range got {
		byUser[c.Username] = c
	}
	if byUser["u1"].Alias != "alias1" {
		t.Fatalf("u1 alias = %q", byUser["u1"].Alias)
	}
	if byUser["u1"].Remark != "Remark 1" {
		t.Fatalf("u1 remark = %q", byUser["u1"].Remark)
	}
	if byUser["u2"].NickName != "Nick 2" {
		t.Fatalf("u2 nick_name = %q", byUser["u2"].NickName)
	}
	if byUser["u1"].HeadURL != "https://example.com/1" {
		t.Fatalf("u1 head_url = %q", byUser["u1"].HeadURL)
	}
	if !byUser["u1"].IsFriend {
		t.Fatal("u1 should be a friend")
	}
	if byUser["u2"].IsFriend {
		t.Fatal("u2 local_type=3 should not be a friend")
	}
}
