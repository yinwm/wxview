//go:build darwin

package key

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const contactRelPath = "contact/contact.db"

type TargetDB struct {
	Account   string
	DataDir   string
	DBRelPath string
	DBPath    string
}

func DiscoverContactDB() (TargetDB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return TargetDB{}, err
	}
	root := filepath.Join(home, "Library", "Containers", "com.tencent.xinWeChat", "Data", "Documents", "xwechat_files")
	entries, err := os.ReadDir(root)
	if err != nil {
		return TargetDB{}, fmt.Errorf("detect WeChat data root %s: %w", root, err)
	}

	var candidates []TargetDB
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		account := entry.Name()
		dataDir := filepath.Join(root, account, "db_storage")
		dbPath := filepath.Join(dataDir, filepath.FromSlash(contactRelPath))
		info, err := os.Stat(dbPath)
		if err != nil || info.IsDir() {
			continue
		}
		candidates = append(candidates, TargetDB{
			Account:   account,
			DataDir:   dataDir,
			DBRelPath: contactRelPath,
			DBPath:    dbPath,
		})
	}
	if len(candidates) == 0 {
		return TargetDB{}, fmt.Errorf("no WeChat contact database found under %s", root)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return dbModUnix(candidates[i].DBPath) > dbModUnix(candidates[j].DBPath)
	})
	return candidates[0], nil
}

func dbModUnix(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}
