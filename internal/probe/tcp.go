package probe

import (
	"context"
	"net"
	"time"
)

func TCPConnect(ctx context.Context, ip, port string, timeout time.Duration) (ms float64, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(ip, port))
	if err != nil {
		return 0, false
	}
	_ = conn.Close()

	return float64(time.Since(start).Microseconds()) / 1000, true
}
