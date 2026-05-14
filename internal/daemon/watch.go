package daemon

import (
	"context"
	"os"
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

func statSignature(path string) fileSignature {
	info, err := os.Stat(path)
	if err != nil {
		return fileSignature{size: -1, modNano: -1}
	}
	return fileSignature{size: info.Size(), modNano: info.ModTime().UnixNano()}
}
