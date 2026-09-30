package probe

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

var (
	pingLossRe = regexp.MustCompile(`(\d+) packets transmitted, (\d+) received,.*?([\d.]+)% packet loss`)
	pingRTTRe  = regexp.MustCompile(`rtt min/avg/max/mdev = ([\d.]+)/([\d.]+)/([\d.]+)/([\d.]+) ms`)
)

type PingStats struct {
	Sent    int
	Recv    int
	LossPct float64
	Min     float64
	Avg     float64
	Max     float64
	Mdev    float64
	HasRTT  bool
}

func Ping(ctx context.Context, target string, count int, interval time.Duration) (PingStats, error) {
	if net.ParseIP(target) == nil {
		return PingStats{}, fmt.Errorf("invalid ping target %q", target)
	}
	if count < 1 {
		return PingStats{}, fmt.Errorf("ping count must be positive")
	}
	if interval <= 0 {
		return PingStats{}, fmt.Errorf("ping interval must be positive")
	}

	deadline := time.Duration(count)*interval + 2*time.Second
	if deadline < 5*time.Second {
		deadline = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, deadline+3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ping",
		"-n", "-q",
		"-c", strconv.Itoa(count),
		"-i", strconv.FormatFloat(interval.Seconds(), 'f', -1, 64),
		"-W", "1",
		"-w", strconv.Itoa(int(deadline.Seconds()+0.999)),
		target,
	)
	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")

	out, err := cmd.CombinedOutput()
	stats, parseErr := ParsePingOutput(string(out))
	if parseErr == nil && stats.Sent > 0 {
		return stats, nil
	}
	if err != nil {
		return PingStats{}, fmt.Errorf("ping %s: %w", target, err)
	}

	return PingStats{}, fmt.Errorf("ping %s: %w", target, parseErr)
}

func ParsePingOutput(out string) (PingStats, error) {
	m := pingLossRe.FindStringSubmatch(out)
	if m == nil {
		return PingStats{}, fmt.Errorf("ping summary not found")
	}

	sent, err := strconv.Atoi(m[1])
	if err != nil {
		return PingStats{}, fmt.Errorf("ping sent: %w", err)
	}
	recv, err := strconv.Atoi(m[2])
	if err != nil {
		return PingStats{}, fmt.Errorf("ping recv: %w", err)
	}
	loss, err := strconv.ParseFloat(m[3], 64)
	if err != nil {
		return PingStats{}, fmt.Errorf("ping loss: %w", err)
	}

	stats := PingStats{
		Sent:    sent,
		Recv:    recv,
		LossPct: loss,
	}

	rtt := pingRTTRe.FindStringSubmatch(out)
	if rtt == nil {
		return stats, nil
	}

	vals := make([]float64, 4)
	for i := range vals {
		vals[i], err = strconv.ParseFloat(rtt[i+1], 64)
		if err != nil {
			return PingStats{}, fmt.Errorf("ping rtt: %w", err)
		}
	}

	stats.Min = vals[0]
	stats.Avg = vals[1]
	stats.Max = vals[2]
	stats.Mdev = vals[3]
	stats.HasRTT = true

	return stats, nil
}
