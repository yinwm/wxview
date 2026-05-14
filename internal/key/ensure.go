package key

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"weview/internal/app"
	"weview/internal/decrypt"
)

type EnsureResult struct {
	Entry  Entry
	Target TargetDB
	Reused bool
}

func EnsureContactKey(ctx context.Context) (EnsureResult, error) {
	target, err := DiscoverContactDB()
	if err != nil {
		return EnsureResult{}, err
	}

	page1, saltBytes, err := decrypt.ReadPage1(target.DBPath)
	if err != nil {
		return EnsureResult{}, fmt.Errorf("read %s: %w", target.DBPath, err)
	}
	salt := hex.EncodeToString(saltBytes)

	storePath, err := app.KeyStorePath()
	if err != nil {
		return EnsureResult{}, err
	}
	store, err := LoadStore(storePath)
	if err != nil {
		return EnsureResult{}, fmt.Errorf("load key store: %w", err)
	}

	if existing, ok := store.Find(target.DataDir, target.DBRelPath, salt); ok {
		if decrypt.ValidateRawHexKey(page1, existing.Key) {
			if err := app.ChownForSudo(storePath); err != nil {
				return EnsureResult{}, fmt.Errorf("fix key store ownership: %w", err)
			}
			return EnsureResult{Entry: existing, Target: target, Reused: true}, nil
		}
	}

	keyHex, err := ScanContactKey(ctx, salt, page1)
	if err != nil {
		return EnsureResult{}, err
	}
	if !decrypt.ValidateRawHexKey(page1, keyHex) {
		return EnsureResult{}, fmt.Errorf("scanned key failed page 1 hmac verification")
	}
	entry := Entry{
		Account:   target.Account,
		DataDir:   target.DataDir,
		DBRelPath: target.DBRelPath,
		Key:       strings.ToLower(keyHex),
		Salt:      salt,
	}
	store.Upsert(entry)
	if err := SaveStore(storePath, store); err != nil {
		return EnsureResult{}, fmt.Errorf("save key store: %w", err)
	}
	if err := app.ChownForSudo(storePath); err != nil {
		return EnsureResult{}, fmt.Errorf("fix key store ownership: %w", err)
	}
	saved, _ := store.Find(target.DataDir, target.DBRelPath, salt)
	if saved.Fingerprint == "" {
		saved = entry
		saved.Fingerprint = Fingerprint(entry.Key, entry.Salt, entry.DataDir, entry.DBRelPath)
	}
	return EnsureResult{Entry: saved, Target: target, Reused: false}, nil
}

func ContactCachePath(account string) (string, error) {
	return app.CacheDBPath(account, contactRelPath)
}

func EnsureContactCache(ctx context.Context) (EnsureResult, string, error) {
	res, err := EnsureContactKey(ctx)
	if err != nil {
		return EnsureResult{}, "", err
	}
	cachePath, err := ContactCachePath(res.Target.Account)
	if err != nil {
		return EnsureResult{}, "", err
	}
	if err := decrypt.DecryptToCache(ctx, res.Target.DBPath, cachePath, res.Entry.Key); err != nil {
		return EnsureResult{}, "", fmt.Errorf("decrypt contact cache: %w", err)
	}
	if err := app.ChownTreeForSudo(appCacheRoot(cachePath)); err != nil {
		return EnsureResult{}, "", fmt.Errorf("fix cache ownership: %w", err)
	}
	return res, cachePath, nil
}

func appCacheRoot(cachePath string) string {
	cacheRoot, err := app.BaseDir()
	if err != nil {
		return cachePath
	}
	return cacheRoot
}

func HasContactCache() (TargetDB, string, bool) {
	target, err := DiscoverContactDB()
	if err != nil {
		return TargetDB{}, "", false
	}
	cachePath, err := ContactCachePath(target.Account)
	if err != nil {
		return TargetDB{}, "", false
	}
	if _, err := os.Stat(cachePath); err == nil {
		return target, cachePath, true
	}
	return target, cachePath, false
}
