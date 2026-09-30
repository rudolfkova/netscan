package probe

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Counters struct {
	RxBytes uint64
	RxErrs  uint64
	RxDrop  uint64
	TxBytes uint64
	TxErrs  uint64
	TxDrop  uint64
}

type Deltas struct {
	RxBytes uint64
	RxErrs  uint64
	RxDrop  uint64
	TxBytes uint64
	TxErrs  uint64
	TxDrop  uint64
}

func ReadIface(path, iface string) (Counters, error) {
	f, err := os.Open(path)
	if err != nil {
		return Counters{}, err
	}
	defer f.Close()

	all, err := ParseNetDev(f)
	if err != nil {
		return Counters{}, err
	}

	c, ok := all[iface]
	if !ok {
		return Counters{}, fmt.Errorf("iface %s not found", iface)
	}

	return c, nil
}

func ParseNetDev(r io.Reader) (map[string]Counters, error) {
	out := make(map[string]Counters)

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		fields := strings.Fields(rest)
		if len(fields) < 16 {
			continue
		}

		rxBytes, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("rx bytes: %w", err)
		}
		rxErrs, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("rx errs: %w", err)
		}
		rxDrop, err := strconv.ParseUint(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("rx drop: %w", err)
		}
		txBytes, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("tx bytes: %w", err)
		}
		txErrs, err := strconv.ParseUint(fields[10], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("tx errs: %w", err)
		}
		txDrop, err := strconv.ParseUint(fields[11], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("tx drop: %w", err)
		}

		out[strings.TrimSpace(name)] = Counters{
			RxBytes: rxBytes,
			RxErrs:  rxErrs,
			RxDrop:  rxDrop,
			TxBytes: txBytes,
			TxErrs:  txErrs,
			TxDrop:  txDrop,
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func CounterDelta(prev, cur Counters, hasPrev bool) Deltas {
	if !hasPrev {
		return Deltas{}
	}

	return Deltas{
		RxBytes: subCounter(cur.RxBytes, prev.RxBytes),
		RxErrs:  subCounter(cur.RxErrs, prev.RxErrs),
		RxDrop:  subCounter(cur.RxDrop, prev.RxDrop),
		TxBytes: subCounter(cur.TxBytes, prev.TxBytes),
		TxErrs:  subCounter(cur.TxErrs, prev.TxErrs),
		TxDrop:  subCounter(cur.TxDrop, prev.TxDrop),
	}
}

func subCounter(cur, prev uint64) uint64 {
	if cur < prev {
		return cur
	}

	return cur - prev
}

func LinkState(sysNet, iface string) (speed, duplex string) {
	if iface == "" {
		return "", ""
	}

	if b, err := os.ReadFile(filepath.Join(sysNet, iface, "speed")); err == nil {
		s := strings.TrimSpace(string(b))
		if s != "" && s != "-1" {
			speed = s
		}
	}
	if b, err := os.ReadFile(filepath.Join(sysNet, iface, "duplex")); err == nil {
		d := strings.TrimSpace(string(b))
		if d != "" && d != "unknown" {
			duplex = d
		}
	}

	return speed, duplex
}
