package daemon

import (
	"sync"
	"time"
)

type Debouncer struct {
	delay time.Duration
	fire  func()

	mu    sync.Mutex
	timer *time.Timer
}

func NewDebouncer(delay time.Duration, fire func()) *Debouncer {
	return &Debouncer{delay: delay, fire: fire}
}

func (d *Debouncer) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.delay, d.fire)
}

func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}
