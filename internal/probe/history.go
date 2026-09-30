package probe

import (
	"sort"
	"sync"
	"time"
)

type rttPoint struct {
	at time.Time
	ms float64
}

type RTTHistory struct {
	mu     sync.Mutex
	window time.Duration
	points map[string][]rttPoint
}

func NewRTTHistory(window time.Duration) *RTTHistory {
	return &RTTHistory{
		window: window,
		points: make(map[string][]rttPoint),
	}
}

func (h *RTTHistory) Median(target string, now time.Time) (float64, int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	pts := h.trim(target, now)
	if len(pts) == 0 {
		return 0, 0
	}

	vals := make([]float64, len(pts))
	for i, p := range pts {
		vals[i] = p.ms
	}

	return median(vals), len(vals)
}

func (h *RTTHistory) Add(target string, at time.Time, ms float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.points[target] = append(h.trim(target, at), rttPoint{at: at, ms: ms})
}

func (h *RTTHistory) trim(target string, now time.Time) []rttPoint {
	pts := h.points[target]
	keep := pts[:0]
	for _, p := range pts {
		if now.Sub(p.at) <= h.window {
			keep = append(keep, p)
		}
	}
	h.points[target] = keep

	return keep
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}

	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}

	return (sorted[n/2-1] + sorted[n/2]) / 2
}
