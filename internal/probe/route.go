package probe

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
)

func DefaultRoute(path string) (iface, gateway string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	return ParseDefaultRoute(f)
}

func ParseDefaultRoute(r io.Reader) (iface, gateway string, err error) {
	var (
		found      bool
		bestMetric = uint64(math.MaxUint64)
	)

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 7 {
			continue
		}
		if fields[1] != "00000000" {
			continue
		}

		flags, err := strconv.ParseUint(fields[3], 16, 32)
		if err != nil || flags&0x2 == 0 {
			continue
		}

		gw, err := parseRouteIPv4(fields[2])
		if err != nil || gw == "0.0.0.0" {
			continue
		}

		metric, err := strconv.ParseUint(fields[6], 10, 64)
		if err != nil {
			continue
		}
		if !found || metric < bestMetric {
			found = true
			bestMetric = metric
			iface = fields[0]
			gateway = gw
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if !found {
		return "", "", fmt.Errorf("no default route")
	}

	return iface, gateway, nil
}

func parseRouteIPv4(hexIP string) (string, error) {
	v, err := strconv.ParseUint(hexIP, 16, 32)
	if err != nil {
		return "", err
	}

	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))

	return net.IP(b[:]).String(), nil
}
