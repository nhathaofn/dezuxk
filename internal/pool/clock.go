package pool

import "time"

// Timer abstracts time.Timer for testability and lifecycle control.
type Timer interface {
	Stop() bool
}

// Clock abstracts system time functions for deterministic and fast testing.
type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) Timer
}

type realTimer struct {
	t *time.Timer
}

func (rt *realTimer) Stop() bool {
	if rt.t == nil {
		return false
	}
	return rt.t.Stop()
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

func (realClock) AfterFunc(d time.Duration, f func()) Timer {
	return &realTimer{t: time.AfterFunc(d, f)}
}
