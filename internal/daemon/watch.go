package daemon

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type fileSignature struct {
	size    int64
	modNano int64
}

func WatchFile(ctx context.Context, path string, interval time.Duration, debounce time.Duration, onChange func()) {
	if interval <= 0 {
		interval = time.Second
	}
	if debounce <= 0 {
		debounce = time.Second
	}
	debouncer := NewDebouncer(debounce, onChange)
	defer debouncer.Stop()

	last := statSignature(path)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := statSignature(path)
			if now != last {
				last = now
				debouncer.Trigger()
			}
		}
	}
}

func WatchFiles(ctx context.Context, paths func() []string, interval time.Duration, debounce time.Duration, onChange func()) {
	if interval <= 0 {
		interval = time.Second
	}
	if debounce <= 0 {
		debounce = time.Second
	}
	debouncer := NewDebouncer(debounce, onChange)
	defer debouncer.Stop()

	last := statFileSet(paths())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := statFileSet(paths())
			if now != last {
				last = now
				debouncer.Trigger()
			}
		}
	}
}

func statSignature(path string) fileSignature {
	info, err := os.Stat(path)
	if err != nil {
		return fileSignature{size: -1, modNano: -1}
	}
	return fileSignature{size: info.Size(), modNano: info.ModTime().UnixNano()}
}

func statFileSet(paths []string) string {
	paths = append([]string(nil), paths...)
	sort.Strings(paths)
	var b strings.Builder
	for _, path := range paths {
		sig := statSignature(path)
		b.WriteString(path)
		b.WriteByte('|')
		b.WriteString(strconv.FormatInt(sig.size, 10))
		b.WriteByte('|')
		b.WriteString(strconv.FormatInt(sig.modNano, 10))
		b.WriteByte('\n')
	}
	return b.String()
}
