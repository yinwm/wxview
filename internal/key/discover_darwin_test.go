//go:build darwin

package key

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChooseContactCandidatePrefersOpenWeChatDataDir(t *testing.T) {
	dir := t.TempDir()
	oldDB := filepath.Join(dir, "old", "db_storage", "contact", "contact.db")
	currentDB := filepath.Join(dir, "current", "db_storage", "contact", "contact.db")
	writeTestDBFile(t, oldDB, time.Unix(200, 0))
	writeTestDBFile(t, currentDB, time.Unix(100, 0))

	got := chooseContactCandidate([]TargetDB{
		{Account: "old", DataDir: filepath.Join(dir, "old", "db_storage"), DBPath: oldDB},
		{Account: "current", DataDir: filepath.Join(dir, "current", "db_storage"), DBPath: currentDB},
	}, map[string]int{
		filepath.Join(dir, "current", "db_storage"): 3,
	})

	if got.Account != "current" {
		t.Fatalf("chosen account = %q, want current", got.Account)
	}
}

func TestChooseContactCandidateFallsBackToNewestDB(t *testing.T) {
	dir := t.TempDir()
	oldDB := filepath.Join(dir, "old", "db_storage", "contact", "contact.db")
	newDB := filepath.Join(dir, "new", "db_storage", "contact", "contact.db")
	writeTestDBFile(t, oldDB, time.Unix(100, 0))
	writeTestDBFile(t, newDB, time.Unix(200, 0))

	got := chooseContactCandidate([]TargetDB{
		{Account: "old", DataDir: filepath.Join(dir, "old", "db_storage"), DBPath: oldDB},
		{Account: "new", DataDir: filepath.Join(dir, "new", "db_storage"), DBPath: newDB},
	}, nil)

	if got.Account != "new" {
		t.Fatalf("chosen account = %q, want new", got.Account)
	}
}

func TestAccountDataDirFromOpenPath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "Users", "me", "xwechat_files")
	path := filepath.Join(root, "wxid_a", "db_storage", "message", "message_0.db")

	got, ok := accountDataDirFromOpenPath(root, path)
	if !ok {
		t.Fatal("expected open path to match account data dir")
	}
	want := filepath.Join(root, "wxid_a", "db_storage")
	if got != want {
		t.Fatalf("data dir = %q, want %q", got, want)
	}
}

func writeTestDBFile(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}
