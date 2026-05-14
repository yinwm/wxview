package key

import (
	"context"
	"encoding/hex"
	"errors"
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

type EnsureKeysResult struct {
	Keys     []EnsureResult
	Warnings []EnsureWarning
}

type EnsureWarning struct {
	DBRelPath string
	Message   string
}

type MissingKeyError struct {
	DBRelPath string
}

func (e MissingKeyError) Error() string {
	return fmt.Sprintf("missing key for %s; run `sudo weview init`", e.DBRelPath)
}

func IsMissingKeyError(err error) bool {
	var missing MissingKeyError
	return errors.As(err, &missing)
}

func EnsureContactKey(ctx context.Context) (EnsureResult, error) {
	target, err := DiscoverContactDB()
	if err != nil {
		return EnsureResult{}, err
	}
	return EnsureDBKey(ctx, target)
}

func EnsureSupportedKeys(ctx context.Context) (EnsureKeysResult, error) {
	requiredTargets, err := DiscoverRequiredDBs()
	if err != nil {
		return EnsureKeysResult{}, err
	}
	results := make([]EnsureResult, 0, len(requiredTargets))
	for _, target := range requiredTargets {
		res, err := EnsureDBKey(ctx, target)
		if err != nil {
			return EnsureKeysResult{Keys: results}, err
		}
		results = append(results, res)
	}

	auxTargets, err := DiscoverMessageAuxDBs()
	if err != nil {
		return EnsureKeysResult{Keys: results}, err
	}
	warnings := make([]EnsureWarning, 0)
	for _, target := range auxTargets {
		res, err := EnsureDBKey(ctx, target)
		if err != nil {
			warnings = append(warnings, EnsureWarning{
				DBRelPath: target.DBRelPath,
				Message:   "auxiliary key missing; skipped. Retry later with `sudo weview init`.",
			})
			continue
		}
		results = append(results, res)
	}
	if len(requiredTargets) > 0 {
		if err := CompactStoreFile(requiredTargets[0].Account); err != nil {
			return EnsureKeysResult{Keys: results, Warnings: warnings}, err
		}
	}
	return EnsureKeysResult{Keys: results, Warnings: warnings}, nil
}

func EnsureDBKey(ctx context.Context, target TargetDB) (EnsureResult, error) {
	page1, saltBytes, err := decrypt.ReadPage1(target.DBPath)
	if err != nil {
		return EnsureResult{}, fmt.Errorf("read %s: %w", target.DBPath, err)
	}
	salt := hex.EncodeToString(saltBytes)

	storePath, err := KeyStorePath(target.Account)
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

	keyHex, err := ScanDBKey(ctx, salt, page1, target.DBRelPath)
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

func LoadDBKey(ctx context.Context, target TargetDB) (EnsureResult, error) {
	page1, saltBytes, err := decrypt.ReadPage1(target.DBPath)
	if err != nil {
		return EnsureResult{}, fmt.Errorf("read %s: %w", target.DBPath, err)
	}
	salt := hex.EncodeToString(saltBytes)

	storePath, err := KeyStorePath(target.Account)
	if err != nil {
		return EnsureResult{}, err
	}
	store, err := LoadStore(storePath)
	if err != nil {
		return EnsureResult{}, fmt.Errorf("load key store: %w", err)
	}
	existing, ok := store.Find(target.DataDir, target.DBRelPath, salt)
	if !ok || !decrypt.ValidateRawHexKey(page1, existing.Key) {
		return EnsureResult{}, MissingKeyError{DBRelPath: target.DBRelPath}
	}
	if err := app.ChownForSudo(storePath); err != nil {
		return EnsureResult{}, fmt.Errorf("fix key store ownership: %w", err)
	}
	return EnsureResult{Entry: existing, Target: target, Reused: true}, nil
}

func ContactCachePath(account string) (string, error) {
	return app.CacheDBPath(account, contactRelPath)
}

func CachePath(account string, relPath string) (string, error) {
	return app.CacheDBPath(account, relPath)
}

func EnsureContactCache(ctx context.Context) (EnsureResult, string, error) {
	target, err := DiscoverContactDB()
	if err != nil {
		return EnsureResult{}, "", err
	}
	res, err := LoadDBKey(ctx, target)
	if err != nil {
		return EnsureResult{}, "", err
	}
	cachePath, err := ContactCachePath(res.Target.Account)
	if err != nil {
		return EnsureResult{}, "", err
	}
	if err := ensureDecryptedCache(ctx, res, cachePath); err != nil {
		return EnsureResult{}, "", fmt.Errorf("decrypt contact cache: %w", err)
	}
	if err := app.ChownTreeForSudo(appCacheRoot(cachePath)); err != nil {
		return EnsureResult{}, "", fmt.Errorf("fix cache ownership: %w", err)
	}
	return res, cachePath, nil
}

func EnsureDBCache(ctx context.Context, target TargetDB) (EnsureResult, string, error) {
	res, err := LoadDBKey(ctx, target)
	if err != nil {
		return EnsureResult{}, "", err
	}
	cachePath, err := CachePath(res.Target.Account, res.Target.DBRelPath)
	if err != nil {
		return EnsureResult{}, "", err
	}
	if err := ensureDecryptedCache(ctx, res, cachePath); err != nil {
		return EnsureResult{}, "", fmt.Errorf("decrypt %s cache: %w", res.Target.DBRelPath, err)
	}
	if err := app.ChownTreeForSudo(appCacheRoot(cachePath)); err != nil {
		return EnsureResult{}, "", fmt.Errorf("fix cache ownership: %w", err)
	}
	return res, cachePath, nil
}

func ensureDecryptedCache(ctx context.Context, res EnsureResult, cachePath string) error {
	fresh, err := isCacheFresh(res.Target, cachePath, res.Entry.Salt)
	if err != nil {
		return err
	}
	if fresh {
		return nil
	}
	before, err := statSourceSignature(res.Target.DBPath)
	if err != nil {
		return err
	}
	if err := decrypt.DecryptToCache(ctx, res.Target.DBPath, cachePath, res.Entry.Key); err != nil {
		return err
	}
	after, err := statSourceSignature(res.Target.DBPath)
	if err != nil {
		return err
	}
	if before != after {
		return nil
	}
	return updateCacheMetadata(res.Target, cachePath, res.Entry.Salt, after)
}

func EnsureMessageCaches(ctx context.Context) ([]string, error) {
	targets, err := DiscoverMessageDBs()
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		if _, _, err := EnsureDBCache(ctx, target); err != nil {
			return nil, err
		}
	}
	paths, allExist, err := MessageCachePaths()
	if err != nil {
		return nil, err
	}
	if !allExist {
		return nil, fmt.Errorf("message cache was not found after refresh")
	}
	_ = ensureMessageAuxCachesBestEffort(ctx)
	return paths, nil
}

func EnsureMessageRelatedCaches(ctx context.Context) ([]string, error) {
	paths, err := EnsureMessageCaches(ctx)
	if err != nil {
		return nil, err
	}
	return paths, nil
}

func ensureMessageAuxCachesBestEffort(ctx context.Context) []EnsureWarning {
	targets, err := DiscoverMessageAuxDBs()
	if err != nil {
		return []EnsureWarning{{Message: err.Error()}}
	}
	warnings := make([]EnsureWarning, 0)
	for _, target := range targets {
		if _, _, err := EnsureDBCache(ctx, target); err != nil {
			warnings = append(warnings, EnsureWarning{
				DBRelPath: target.DBRelPath,
				Message:   "auxiliary DB cache skipped; run `sudo weview init` again if this DB becomes needed",
			})
		}
	}
	return warnings
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

func MessageCachePaths() ([]string, bool, error) {
	targets, err := DiscoverMessageDBs()
	if err != nil {
		return nil, false, err
	}
	paths := make([]string, 0, len(targets))
	allExist := true
	for _, target := range targets {
		cachePath, err := CachePath(target.Account, target.DBRelPath)
		if err != nil {
			return nil, false, err
		}
		if _, err := os.Stat(cachePath); err != nil {
			if os.IsNotExist(err) {
				allExist = false
			} else {
				return nil, false, err
			}
		}
		paths = append(paths, cachePath)
	}
	return paths, allExist, nil
}
