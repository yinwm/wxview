package key

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreFindByDataDirRelPathAndSalt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	store := Store{Version: 1}
	entry := Entry{
		Account:   "wxid_a",
		DataDir:   filepath.Join(dir, "db_storage"),
		DBRelPath: "contact/contact.db",
		Key:       "001122",
		Salt:      "aabbcc",
	}
	store.Upsert(entry)
	if err := SaveStore(path, store); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("keys.json mode = %o, want 0600", info.Mode().Perm())
	}

	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Find(entry.DataDir, entry.DBRelPath, "AABBCC")
	if !ok {
		t.Fatal("expected key hit")
	}
	if got.Fingerprint == "" {
		t.Fatal("expected fingerprint")
	}
	if _, ok := loaded.Find(entry.DataDir, entry.DBRelPath, "different"); ok {
		t.Fatal("expected salt mismatch to miss")
	}
}
