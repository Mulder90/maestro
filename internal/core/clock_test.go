package core

import (
	"testing"
	"time"
)

func TestRealClock_Now(t *testing.T) {
	clock := RealClock{}
	before := time.Now()
	now := clock.Now()
	after := time.Now()

	if now.Before(before) || now.After(after) {
		t.Errorf("RealClock.Now() returned %v, expected between %v and %v", now, before, after)
	}
}

func TestRealClock_Since(t *testing.T) {
	clock := RealClock{}
	start := time.Now()
	time.Sleep(10 * time.Millisecond)
	elapsed := clock.Since(start)

	if elapsed < 10*time.Millisecond {
		t.Errorf("RealClock.Since() returned %v, expected >= 10ms", elapsed)
	}
}

func TestFakeClock_Now(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)

	if !clock.Now().Equal(start) {
		t.Errorf("FakeClock.Now() returned %v, expected %v", clock.Now(), start)
	}
}

func TestFakeClock_Advance(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)

	clock.Advance(1 * time.Hour)
	expected := start.Add(1 * time.Hour)

	if !clock.Now().Equal(expected) {
		t.Errorf("after Advance(1h), Now() returned %v, expected %v", clock.Now(), expected)
	}
}

func TestFakeClock_Since(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)

	// Since should return 0 initially
	if clock.Since(start) != 0 {
		t.Errorf("FakeClock.Since(start) = %v, expected 0", clock.Since(start))
	}

	// Advance and check Since
	clock.Advance(5 * time.Minute)
	if clock.Since(start) != 5*time.Minute {
		t.Errorf("after Advance(5m), Since(start) = %v, expected 5m", clock.Since(start))
	}
}

func TestFakeClock_Set(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)

	newTime := time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC)
	clock.Set(newTime)

	if !clock.Now().Equal(newTime) {
		t.Errorf("after Set(), Now() returned %v, expected %v", clock.Now(), newTime)
	}
}

func TestFakeClock_MultipleAdvances(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)

	clock.Advance(10 * time.Second)
	clock.Advance(20 * time.Second)
	clock.Advance(30 * time.Second)

	expected := start.Add(60 * time.Second)
	if !clock.Now().Equal(expected) {
		t.Errorf("after multiple Advances, Now() = %v, expected %v", clock.Now(), expected)
	}
}
