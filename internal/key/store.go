package key

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Account     string    `json:"account"`
	DataDir     string    `json:"data_dir"`
	DBRelPath   string    `json:"db_rel_path"`
	Key         string    `json:"key"`
	Salt        string    `json:"salt"`
	Fingerprint string    `json:"fingerprint"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Store struct {
	Version int     `json:"version"`
	Keys    []Entry `json:"keys"`
}

func LoadStore(path string) (Store, error) {
	var store Store
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Store{Version: 1}, nil
		}
		return store, err
	}
	if len(data) == 0 {
		return Store{Version: 1}, nil
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return store, err
	}
	if store.Version == 0 {
		store.Version = 1
	}
	return store, nil
}

func SaveStore(path string, store Store) error {
	if store.Version == 0 {
		store.Version = 1
	}
	sort.SliceStable(store.Keys, func(i, j int) bool {
		if store.Keys[i].Account != store.Keys[j].Account {
			return store.Keys[i].Account < store.Keys[j].Account
		}
		if store.Keys[i].DataDir != store.Keys[j].DataDir {
			return store.Keys[i].DataDir < store.Keys[j].DataDir
		}
		return store.Keys[i].DBRelPath < store.Keys[j].DBRelPath
	})
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write([]byte("\n")); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s Store) Find(dataDir string, dbRelPath string, salt string) (Entry, bool) {
	for _, entry := range s.Keys {
		if samePath(entry.DataDir, dataDir) && entry.DBRelPath == dbRelPath && strings.EqualFold(entry.Salt, salt) {
			return entry, true
		}
	}
	return Entry{}, false
}

func (s *Store) Upsert(entry Entry) {
	entry.Fingerprint = Fingerprint(entry.Key, entry.Salt, entry.DataDir, entry.DBRelPath)
	entry.UpdatedAt = time.Now().UTC()
	for i := range s.Keys {
		if samePath(s.Keys[i].DataDir, entry.DataDir) && s.Keys[i].DBRelPath == entry.DBRelPath && strings.EqualFold(s.Keys[i].Salt, entry.Salt) {
			s.Keys[i] = entry
			return
		}
	}
	s.Keys = append(s.Keys, entry)
}

func Fingerprint(keyHex string, saltHex string, dataDir string, relPath string) string {
	sum := sha256.Sum256([]byte(keyHex + "|" + saltHex + "|" + dataDir + "|" + relPath))
	return hex.EncodeToString(sum[:8])
}

func samePath(a string, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
