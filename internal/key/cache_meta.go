package key

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wxview/internal/app"
)

const cacheMetaRelPath = "mtime.json"

type sourceSignature struct {
	Size    int64
	MTimeNS int64
}

type cacheMetadata struct {
	Version int                           `json:"version"`
	Files   map[string]cacheMetadataEntry `json:"files"`
}

type cacheMetadataEntry struct {
	SourcePath    string    `json:"source_path"`
	SourceSize    int64     `json:"source_size"`
	SourceMTimeNS int64     `json:"source_mtime_ns"`
	SourceSalt    string    `json:"source_salt"`
	CachePath     string    `json:"cache_path"`
	RefreshedAt   time.Time `json:"refreshed_at"`
}

func CacheMetaPath(account string) (string, error) {
	return app.CacheDBPath(account, cacheMetaRelPath)
}

func isCacheFresh(target TargetDB, cachePath string, sourceSalt string) (bool, error) {
	if _, err := os.Stat(cachePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	current, err := statSourceSignature(target.DBPath)
	if err != nil {
		return false, err
	}
	metaPath, err := CacheMetaPath(target.Account)
	if err != nil {
		return false, err
	}
	meta, err := loadCacheMetadata(metaPath)
	if err != nil {
		return false, err
	}
	entry, ok := meta.Files[target.DBRelPath]
	if !ok {
		return false, nil
	}
	if !samePath(entry.SourcePath, target.DBPath) || !samePath(entry.CachePath, cachePath) {
		return false, nil
	}
	if !strings.EqualFold(entry.SourceSalt, sourceSalt) {
		return false, nil
	}
	return entry.SourceSize == current.Size && entry.SourceMTimeNS == current.MTimeNS, nil
}

func updateCacheMetadata(target TargetDB, cachePath string, sourceSalt string, sig sourceSignature) error {
	metaPath, err := CacheMetaPath(target.Account)
	if err != nil {
		return err
	}
	meta, err := loadCacheMetadata(metaPath)
	if err != nil {
		return err
	}
	if meta.Files == nil {
		meta.Files = map[string]cacheMetadataEntry{}
	}
	meta.Version = 1
	meta.Files[target.DBRelPath] = cacheMetadataEntry{
		SourcePath:    target.DBPath,
		SourceSize:    sig.Size,
		SourceMTimeNS: sig.MTimeNS,
		SourceSalt:    strings.ToLower(sourceSalt),
		CachePath:     cachePath,
		RefreshedAt:   time.Now().UTC(),
	}
	return saveCacheMetadata(metaPath, meta)
}

func statSourceSignature(path string) (sourceSignature, error) {
	info, err := os.Stat(path)
	if err != nil {
		return sourceSignature{}, err
	}
	return sourceSignature{
		Size:    info.Size(),
		MTimeNS: info.ModTime().UnixNano(),
	}, nil
}

func loadCacheMetadata(path string) (cacheMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cacheMetadata{Version: 1, Files: map[string]cacheMetadataEntry{}}, nil
		}
		return cacheMetadata{}, err
	}
	if len(data) == 0 {
		return cacheMetadata{Version: 1, Files: map[string]cacheMetadataEntry{}}, nil
	}
	var meta cacheMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return cacheMetadata{}, err
	}
	if meta.Version == 0 {
		meta.Version = 1
	}
	if meta.Files == nil {
		meta.Files = map[string]cacheMetadataEntry{}
	}
	return meta, nil
}

func saveCacheMetadata(path string, meta cacheMetadata) error {
	meta.Version = 1
	if meta.Files == nil {
		meta.Files = map[string]cacheMetadataEntry{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
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
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return app.ChownForSudo(path)
}
