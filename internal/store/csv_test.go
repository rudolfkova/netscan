package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndCleanup(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	old := now.Add(-25 * time.Hour)
	fresh := now.Add(-time.Minute)

	if err := s.AppendProbes([]ProbeRow{
		{
			TS: old, Target: "1.1.1.1", Kind: "external",
			Sent: 10, Recv: 0, LossPct: 100,
		},
		{
			TS: fresh, Target: "1.1.1.1", Kind: "external",
			Sent: 10, Recv: 10, LossPct: 0,
			RTTMin: 1.25, RTTAvg: 2.5, RTTMax: 4, Jitter: 0.3, HasRTT: true,
			TCPConnect: "12.5", DuringSpeedtest: true, Iface: "eth0",
			RxBytesDelta: 1000, LinkSpeedMbps: "1000", Duplex: "full",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendSpeed(SpeedRow{
		TS: old, URL: "https://example.test/down", Bytes: 10, Duration: time.Second, Mbps: 1, LimitedBy: "time",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendSpeed(SpeedRow{
		TS: fresh, URL: "https://example.test/down", Bytes: 4096, Duration: 500 * time.Millisecond, Mbps: 8, LimitedBy: "bytes",
	}); err != nil {
		t.Fatal(err)
	}

	probes, err := readCSV(filepath.Join(dir, "probes.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != 3 {
		t.Fatalf("probes rows = %d", len(probes))
	}
	if len(probes[0]) != len(probeHeader) || len(probes[2]) != len(probeHeader) {
		t.Fatalf("header %d row %d", len(probes[0]), len(probes[2]))
	}
	if probes[1][5] != "100" || probes[1][6] != "" || probes[1][11] != "0" {
		t.Fatalf("old probe = %v", probes[1])
	}
	if probes[2][7] != "2.500" || probes[2][10] != "12.5" || probes[2][11] != "1" || probes[2][17] != "1000" {
		t.Fatalf("fresh probe = %v", probes[2])
	}

	oldName := old.Format("20060102T150405Z") + "_1.1.1.1.txt"
	if err := os.WriteFile(filepath.Join(dir, incidentsDir, oldName), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteIncident(fresh, "8.8.8.8", []byte("fresh\n")); err != nil {
		t.Fatal(err)
	}

	removed, err := s.Cleanup(now)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 3 {
		t.Fatalf("removed = %d", removed)
	}

	probes, err = readCSV(filepath.Join(dir, "probes.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != 2 || probes[1][0] != fresh.Format(time.RFC3339) {
		t.Fatalf("probes after cleanup = %v", probes)
	}

	speeds, err := readCSV(filepath.Join(dir, "speed.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(speeds) != 2 || speeds[1][5] != "bytes" {
		t.Fatalf("speeds after cleanup = %v", speeds)
	}

	entries, err := os.ReadDir(filepath.Join(dir, incidentsDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() == oldName {
		t.Fatalf("incidents = %v", entries)
	}
}
