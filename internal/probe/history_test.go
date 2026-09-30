package probe

import (
	"testing"
	"time"
)

func TestRTTHistory(t *testing.T) {
	h := NewRTTHistory(time.Hour)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	h.Add("1.1.1.1", base, 10)
	h.Add("1.1.1.1", base.Add(time.Minute), 30)
	h.Add("1.1.1.1", base.Add(2*time.Minute), 20)

	med, n := h.Median("1.1.1.1", base.Add(3*time.Minute))
	if n != 3 || med != 20 {
		t.Fatalf("median = %v n = %d", med, n)
	}

	h.Add("8.8.8.8", base, 10)
	h.Add("8.8.8.8", base.Add(time.Minute), 30)
	med, n = h.Median("8.8.8.8", base.Add(2*time.Minute))
	if n != 2 || med != 20 {
		t.Fatalf("even median = %v n = %d", med, n)
	}

	h.Add("1.1.1.1", base.Add(40*time.Minute), 7)
	med, n = h.Median("1.1.1.1", base.Add(90*time.Minute))
	if n != 1 || med != 7 {
		t.Fatalf("expired median = %v n = %d", med, n)
	}
}
