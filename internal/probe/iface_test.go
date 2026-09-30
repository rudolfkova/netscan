package probe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNetDev(t *testing.T) {
	in := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1 2 3 4 0 0 0 0 5 6 7 8 0 0 0 0
  eth0: 1000 20 1 2 0 0 0 0 3000 30 3 4 0 0 0 0
`
	all, err := ParseNetDev(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}

	eth := all["eth0"]
	if eth.RxBytes != 1000 || eth.RxErrs != 1 || eth.RxDrop != 2 || eth.TxBytes != 3000 || eth.TxErrs != 3 || eth.TxDrop != 4 {
		t.Fatalf("eth0 = %+v", eth)
	}
	if all["lo"].RxBytes != 1 || all["lo"].TxDrop != 8 {
		t.Fatalf("lo = %+v", all["lo"])
	}
}

func TestCounterDelta(t *testing.T) {
	if d := CounterDelta(Counters{}, Counters{RxBytes: 10}, false); d != (Deltas{}) {
		t.Fatalf("first sample = %+v", d)
	}

	prev := Counters{RxBytes: 100, RxErrs: 1, TxBytes: 50, TxDrop: 2}
	cur := Counters{RxBytes: 140, RxErrs: 4, TxBytes: 80, TxDrop: 2}
	d := CounterDelta(prev, cur, true)
	if d.RxBytes != 40 || d.RxErrs != 3 || d.TxBytes != 30 || d.TxDrop != 0 {
		t.Fatalf("delta = %+v", d)
	}

	wrapped := CounterDelta(Counters{RxBytes: 1000}, Counters{RxBytes: 5}, true)
	if wrapped.RxBytes != 5 {
		t.Fatalf("wrap = %+v", wrapped)
	}
}

func TestLinkState(t *testing.T) {
	dir := t.TempDir()
	writeLink := func(iface, speed, duplex string) {
		t.Helper()
		path := filepath.Join(dir, iface)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "speed"), []byte(speed+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "duplex"), []byte(duplex+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeLink("eth0", "1000", "full")
	writeLink("eth1", "-1", "unknown")

	speed, duplex := LinkState(dir, "eth0")
	if speed != "1000" || duplex != "full" {
		t.Fatalf("eth0 = %q %q", speed, duplex)
	}

	speed, duplex = LinkState(dir, "eth1")
	if speed != "" || duplex != "" {
		t.Fatalf("eth1 = %q %q", speed, duplex)
	}
}
