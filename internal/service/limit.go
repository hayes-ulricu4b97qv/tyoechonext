package service

import (
	"sync"
	"time"
)

// HitCounter counts recent events for one process. Each server keeps its own counter.
type HitCounter struct {
	mu     sync.Mutex
	events map[string][]time.Time
}

func NewHitCounter() *HitCounter {
	return &HitCounter{events: map[string][]time.Time{}}
}

func (h *HitCounter) Allow(key string, limit int, window time.Duration) bool {
	if h == nil || limit < 1 || key == "" {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	kept := pruneTimes(h.events[key], now, window)
	if len(kept) >= limit {
		h.events[key] = kept
		return false
	}
	h.events[key] = append(kept, now)
	return true
}

// RecordFailure stores one failure and reports whether the key is now over the limit.
func (h *HitCounter) RecordFailure(key string, limit int, window time.Duration) bool {
	if h == nil || limit < 1 || key == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	kept := append(pruneTimes(h.events[key], now, window), now)
	h.events[key] = kept
	return len(kept) >= limit
}

func (h *HitCounter) Reset(key string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.events, key)
}

func pruneTimes(times []time.Time, now time.Time, window time.Duration) []time.Time {
	cutoff := now.Add(-window)
	kept := make([]time.Time, 0, len(times))
	for _, stamp := range times {
		if stamp.After(cutoff) {
			kept = append(kept, stamp)
		}
	}
	return kept
}
