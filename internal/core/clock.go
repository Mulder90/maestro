package core

import "time"

// Clock provides time operations that can be mocked for testing.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

// RealClock uses the standard time package.
type RealClock struct{}

func (RealClock) Now() time.Time                     { return time.Now() }
func (RealClock) Since(t time.Time) time.Duration   { return time.Since(t) }

// FakeClock is a test clock that can be manually advanced.
type FakeClock struct {
	current time.Time
}

func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{current: start}
}

func (f *FakeClock) Now() time.Time                   { return f.current }
func (f *FakeClock) Since(t time.Time) time.Duration { return f.current.Sub(t) }
func (f *FakeClock) Advance(d time.Duration)         { f.current = f.current.Add(d) }
func (f *FakeClock) Set(t time.Time)                 { f.current = t }
