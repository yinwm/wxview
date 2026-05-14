package daemon

import (
	"testing"
	"time"
)

func TestDebouncerCoalescesTriggers(t *testing.T) {
	ch := make(chan struct{}, 3)
	d := NewDebouncer(30*time.Millisecond, func() {
		ch <- struct{}{}
	})
	defer d.Stop()

	d.Trigger()
	d.Trigger()
	d.Trigger()

	select {
	case <-ch:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected debounced fire")
	}

	select {
	case <-ch:
		t.Fatal("expected one debounced fire")
	case <-time.After(80 * time.Millisecond):
	}
}
