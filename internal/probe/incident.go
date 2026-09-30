package probe

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
)

const (
	KindGateway  = "gateway"
	KindExternal = "external"
)

type Threshold struct {
	LossPct    float64
	RTTFactor  float64
	RTTFloorMs float64
	MinSamples int
}

func (t Threshold) Tripped(lossPct, rttAvg float64, hasRTT bool, medianMs float64, samples int) bool {
	if lossPct >= t.LossPct {
		return true
	}
	if !hasRTT || samples < t.MinSamples {
		return false
	}

	limit := t.RTTFactor * medianMs
	if t.RTTFloorMs > limit {
		limit = t.RTTFloorMs
	}

	return rttAvg > limit
}

type Sample struct {
	Target   string
	Kind     string
	Loss     float64
	RTT      float64
	HasRTT   bool
	Incident bool
}

func MTRTarget(samples []Sample, fallbackExternal string) (string, bool) {
	best := -1
	for i := range samples {
		s := &samples[i]
		if !s.Incident || s.Kind != KindExternal {
			continue
		}
		if best < 0 || worseSample(s, &samples[best]) {
			best = i
		}
	}
	if best >= 0 {
		return samples[best].Target, true
	}

	for _, s := range samples {
		if s.Incident && fallbackExternal != "" {
			return fallbackExternal, true
		}
	}

	return "", false
}

func worseSample(a, b *Sample) bool {
	if a.Loss != b.Loss {
		return a.Loss > b.Loss
	}

	return a.RTT > b.RTT
}

func MTR(ctx context.Context, target string) ([]byte, error) {
	if net.ParseIP(target) == nil {
		return nil, fmt.Errorf("invalid mtr target %q", target)
	}

	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "mtr", "-n", "--report", "-c", "15", "-w", target)
	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("mtr %s: %w", target, err)
	}

	return out, nil
}
