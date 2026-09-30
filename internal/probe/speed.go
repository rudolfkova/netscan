package probe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	LimitedByBytes = "bytes"
	LimitedByTime  = "time"
	LimitedByEOF   = "eof"
)

type SpeedResult struct {
	Bytes     int64
	Duration  time.Duration
	Mbps      float64
	LimitedBy string
}

func Download(ctx context.Context, rawURL string, maxDur time.Duration, maxBytes int64) (SpeedResult, error) {
	if maxDur <= 0 || maxBytes <= 0 {
		return SpeedResult{}, fmt.Errorf("speedtest limits must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, maxDur)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return SpeedResult{}, err
	}
	req.Header.Set("User-Agent", "netscan/1.0")
	req.Header.Set("Accept-Encoding", "identity")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return SpeedResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return SpeedResult{}, fmt.Errorf("speedtest status %s", resp.Status)
	}

	buf := make([]byte, 32*1024)
	var read int64
	hitTime := false
	hitEOF := false

	for read < maxBytes {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return SpeedResult{}, err
			}
			hitTime = true
			break
		}
		if time.Since(start) >= maxDur {
			hitTime = true
			break
		}

		remain := maxBytes - read
		chunk := buf
		if int64(len(chunk)) > remain {
			chunk = chunk[:remain]
		}

		n, err := resp.Body.Read(chunk)
		read += int64(n)
		if read >= maxBytes {
			break
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				hitEOF = true
				break
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return SpeedResult{}, ctx.Err()
			}
			hitTime = true
			break
		}
	}

	elapsed := time.Since(start)
	limited := ClassifyLimit(read, maxBytes, elapsed, maxDur, hitTime, hitEOF)

	return SpeedResult{
		Bytes:     read,
		Duration:  elapsed,
		Mbps:      Mbps(read, elapsed),
		LimitedBy: limited,
	}, nil
}

func ClassifyLimit(read, maxBytes int64, elapsed, maxDur time.Duration, hitTime, hitEOF bool) string {
	if maxBytes > 0 && read >= maxBytes {
		return LimitedByBytes
	}
	if hitTime || (maxDur > 0 && elapsed >= maxDur) {
		return LimitedByTime
	}
	if hitEOF {
		return LimitedByEOF
	}

	return LimitedByTime
}

func Mbps(bytes int64, d time.Duration) float64 {
	if bytes <= 0 || d <= 0 {
		return 0
	}

	return float64(bytes) * 8 / d.Seconds() / 1_000_000
}
