package qlcache

import (
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// breakerConfig holds thresholds. Defaults chosen so a brief Redis blip
// doesn't trip the breaker but a sustained outage does. Overridable via env.
type breakerConfig struct {
	failuresToOpen int
	failureWindow  time.Duration
	openDuration   time.Duration
}

var bkConfig = breakerConfig{
	failuresToOpen: envIntDefault("ERUQL_CACHE_BREAKER_FAILURES", 5),
	failureWindow:  time.Duration(envIntDefault("ERUQL_CACHE_BREAKER_WINDOW_SEC", 10)) * time.Second,
	openDuration:   time.Duration(envIntDefault("ERUQL_CACHE_BREAKER_OPEN_SEC", 30)) * time.Second,
}

func envIntDefault(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

type breakerState struct {
	mu          sync.Mutex
	failures    []time.Time
	openedUntil time.Time
	trips       atomic.Int64
}

var bk = &breakerState{}

// breakerAllow reports whether cache operations are currently allowed. When
// the breaker is open, callers should treat it as a cache bypass.
func breakerAllow() bool {
	bk.mu.Lock()
	defer bk.mu.Unlock()
	return time.Now().After(bk.openedUntil)
}

// breakerRecordFailure increments the failure counter within the rolling
// window. If the threshold is crossed, the breaker trips open for
// openDuration.
func breakerRecordFailure() {
	now := time.Now()
	bk.mu.Lock()
	defer bk.mu.Unlock()

	cutoff := now.Add(-bkConfig.failureWindow)
	kept := bk.failures[:0]
	for _, t := range bk.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	bk.failures = append(kept, now)

	if len(bk.failures) >= bkConfig.failuresToOpen && now.After(bk.openedUntil) {
		bk.openedUntil = now.Add(bkConfig.openDuration)
		bk.failures = bk.failures[:0]
		bk.trips.Add(1)
	}
}

// breakerRecordSuccess clears the failure history on a successful op.
func breakerRecordSuccess() {
	bk.mu.Lock()
	defer bk.mu.Unlock()
	if len(bk.failures) > 0 {
		bk.failures = bk.failures[:0]
	}
}

// BreakerOpen returns whether the breaker is currently blocking cache calls.
func BreakerOpen() bool { return !breakerAllow() }

// BreakerTrips returns how many times the breaker has opened since process
// start.
func BreakerTrips() int64 { return bk.trips.Load() }
