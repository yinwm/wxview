//go:build darwin

package key

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"weview/internal/app"
)

const contactRelPath = "contact/contact.db"
const messageRelDir = "message"

var messageAuxRelPaths = []string{
	"message/message_fts.db",
	"message/message_resource.db",
	"message/message_revoke.db",
}

type TargetDB struct {
	Account   string
	DataDir   string
	DBRelPath string
	DBPath    string
}

func DiscoverContactDB() (TargetDB, error) {
	home, err := app.HomeDir()
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
	return chooseContactCandidate(candidates, wechatOpenDataDirScores(root)), nil
}

func DiscoverMessageDBs() ([]TargetDB, error) {
	contactTarget, err := DiscoverContactDB()
	if err != nil {
		return nil, err
	}
	messageDir := filepath.Join(contactTarget.DataDir, messageRelDir)
	entries, err := os.ReadDir(messageDir)
	if err != nil {
		return nil, fmt.Errorf("detect WeChat message database directory %s: %w", messageDir, err)
	}

	var candidates []TargetDB
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		_, ok := messageShardIndex(entry.Name())
		if !ok {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join(messageRelDir, entry.Name()))
		dbPath := filepath.Join(contactTarget.DataDir, filepath.FromSlash(relPath))
		info, err := os.Stat(dbPath)
		if err != nil || info.IsDir() {
			continue
		}
		candidates = append(candidates, TargetDB{
			Account:   contactTarget.Account,
			DataDir:   contactTarget.DataDir,
			DBRelPath: relPath,
			DBPath:    dbPath,
		})
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no WeChat message shard database found under %s", messageDir)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, _ := messageShardIndex(filepath.Base(candidates[i].DBPath))
		right, _ := messageShardIndex(filepath.Base(candidates[j].DBPath))
		return left < right
	})
	return candidates, nil
}

func DiscoverMessageRelatedDBs() ([]TargetDB, error) {
	targets, err := DiscoverMessageDBs()
	if err != nil {
		return nil, err
	}
	auxTargets, err := DiscoverMessageAuxDBs()
	if err != nil {
		return nil, err
	}
	targets = append(targets, auxTargets...)
	return targets, nil
}

func DiscoverMessageAuxDBs() ([]TargetDB, error) {
	contactTarget, err := DiscoverContactDB()
	if err != nil {
		return nil, err
	}
	targets := make([]TargetDB, 0, len(messageAuxRelPaths))
	for _, relPath := range messageAuxRelPaths {
		dbPath := filepath.Join(contactTarget.DataDir, filepath.FromSlash(relPath))
		info, err := os.Stat(dbPath)
		if err != nil || info.IsDir() {
			continue
		}
		targets = append(targets, TargetDB{
			Account:   contactTarget.Account,
			DataDir:   contactTarget.DataDir,
			DBRelPath: relPath,
			DBPath:    dbPath,
		})
	}
	return targets, nil
}

func DiscoverRequiredDBs() ([]TargetDB, error) {
	contactTarget, err := DiscoverContactDB()
	if err != nil {
		return nil, err
	}
	targets := []TargetDB{contactTarget}
	messageTargets, err := DiscoverMessageDBs()
	if err != nil {
		return nil, err
	}
	targets = append(targets, messageTargets...)
	return targets, nil
}

func DiscoverSupportedDBs() ([]TargetDB, error) {
	targets, err := DiscoverRequiredDBs()
	if err != nil {
		return nil, err
	}
	auxTargets, err := DiscoverMessageAuxDBs()
	if err != nil {
		return nil, err
	}
	targets = append(targets, auxTargets...)
	return targets, nil
}

func messageShardIndex(name string) (int, bool) {
	if !strings.HasPrefix(name, "message_") || !strings.HasSuffix(name, ".db") {
		return 0, false
	}
	text := strings.TrimSuffix(strings.TrimPrefix(name, "message_"), ".db")
	index, err := strconv.Atoi(text)
	return index, err == nil
}

func dbModUnix(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

func chooseContactCandidate(candidates []TargetDB, openScores map[string]int) TargetDB {
	candidates = append([]TargetDB(nil), candidates...)
	sort.SliceStable(candidates, func(i, j int) bool {
		leftScore := openScores[filepath.Clean(candidates[i].DataDir)]
		rightScore := openScores[filepath.Clean(candidates[j].DataDir)]
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return dbModUnix(candidates[i].DBPath) > dbModUnix(candidates[j].DBPath)
	})
	return candidates[0]
}

func wechatOpenDataDirScores(root string) map[string]int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pids, err := wechatPIDs(ctx)
	if err != nil || len(pids) == 0 {
		return nil
	}
	scores := map[string]int{}
	for _, pid := range pids {
		cmd := exec.CommandContext(ctx, "lsof", "-Fn", "-p", strconv.Itoa(pid))
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.HasPrefix(line, "n") {
				continue
			}
			if dataDir, ok := accountDataDirFromOpenPath(root, line[1:]); ok {
				scores[filepath.Clean(dataDir)]++
			}
		}
	}
	return scores
}

func accountDataDirFromOpenPath(root string, path string) (string, bool) {
	path = strings.TrimSuffix(path, " (deleted)")
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 3 || parts[1] != "db_storage" {
		return "", false
	}
	return filepath.Join(root, parts[0], "db_storage"), true
}
