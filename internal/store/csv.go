package store

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const incidentsDir = "incidents"

var probeHeader = []string{
	"ts", "target", "kind", "sent", "recv", "loss_pct",
	"rtt_min_ms", "rtt_avg_ms", "rtt_max_ms", "jitter_ms", "tcp_connect_ms",
	"during_speedtest", "iface",
	"rx_err_delta", "tx_err_delta", "rx_drop_delta", "tx_drop_delta",
	"rx_bytes_delta", "tx_bytes_delta", "link_speed_mbps", "duplex",
}

var speedHeader = []string{
	"ts", "url", "bytes", "duration_ms", "mbps", "limited_by",
}

type ProbeRow struct {
	TS              time.Time
	Target          string
	Kind            string
	Sent            int
	Recv            int
	LossPct         float64
	RTTMin          float64
	RTTAvg          float64
	RTTMax          float64
	Jitter          float64
	HasRTT          bool
	TCPConnect      string
	DuringSpeedtest bool
	Iface           string
	RxErrDelta      uint64
	TxErrDelta      uint64
	RxDropDelta     uint64
	TxDropDelta     uint64
	RxBytesDelta    uint64
	TxBytesDelta    uint64
	LinkSpeedMbps   string
	Duplex          string
}

type SpeedRow struct {
	TS        time.Time
	URL       string
	Bytes     int64
	Duration  time.Duration
	Mbps      float64
	LimitedBy string
}

type Store struct {
	mu   sync.Mutex
	dir  string
	keep time.Duration
}

func New(dir string, keep time.Duration) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, incidentsDir), 0o755); err != nil {
		return nil, err
	}

	return &Store{dir: dir, keep: keep}, nil
}

func (s *Store) AppendProbes(rows []ProbeRow) error {
	records := make([][]string, 0, len(rows))
	for _, row := range rows {
		records = append(records, probeRecord(row))
	}

	return s.appendCSV("probes.csv", probeHeader, records)
}

func (s *Store) AppendSpeed(row SpeedRow) error {
	return s.appendCSV("speed.csv", speedHeader, [][]string{speedRecord(row)})
}

func (s *Store) WriteIncident(ts time.Time, target string, body []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := ts.UTC().Format("20060102T150405Z") + "_" + safeName(target) + ".txt"
	path := filepath.Join(s.dir, incidentsDir, name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}

	return path, nil
}

func (s *Store) Cleanup(now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed, err := s.cleanupCSV(filepath.Join(s.dir, "probes.csv"), now)
	if err != nil {
		return removed, err
	}

	speedRemoved, err := s.cleanupCSV(filepath.Join(s.dir, "speed.csv"), now)
	removed += speedRemoved
	if err != nil {
		return removed, err
	}

	incidents, err := s.cleanupIncidents(now)
	removed += incidents

	return removed, err
}

func (s *Store) appendCSV(name string, header []string, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(filepath.Join(s.dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	w := csv.NewWriter(f)
	if info.Size() == 0 {
		if err := w.Write(header); err != nil {
			return err
		}
	}
	if err := w.WriteAll(rows); err != nil {
		return err
	}

	w.Flush()
	return w.Error()
}

func (s *Store) cleanupCSV(path string, now time.Time) (int, error) {
	records, err := readCSV(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}

	header := records[0]
	kept, removed := filterRecent(records[1:], now, s.keep)
	if removed == 0 {
		return 0, nil
	}

	if err := writeCSV(path, header, kept); err != nil {
		return 0, err
	}

	return removed, nil
}

func (s *Store) cleanupIncidents(now time.Time) (int, error) {
	dir := filepath.Join(s.dir, incidentsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ts, ok := incidentTime(entry.Name())
		if !ok || now.Sub(ts) <= s.keep {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return removed, err
		}
		removed++
	}

	return removed, nil
}

func filterRecent(rows [][]string, now time.Time, keep time.Duration) (kept [][]string, removed int) {
	for _, row := range rows {
		if len(row) == 0 {
			removed++
			continue
		}

		ts, err := time.Parse(time.RFC3339, row[0])
		if err != nil || now.Sub(ts) > keep {
			removed++
			continue
		}

		kept = append(kept, row)
	}

	return kept, removed
}

func writeCSV(path string, header []string, rows [][]string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	w := csv.NewWriter(tmp)
	if err := w.Write(header); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := w.WriteAll(rows); err != nil {
		_ = tmp.Close()
		return err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}

	success = true
	return nil
}

func probeRecord(row ProbeRow) []string {
	during := "0"
	if row.DuringSpeedtest {
		during = "1"
	}

	return []string{
		row.TS.UTC().Format(time.RFC3339),
		row.Target,
		row.Kind,
		strconv.Itoa(row.Sent),
		strconv.Itoa(row.Recv),
		strconv.FormatFloat(row.LossPct, 'f', -1, 64),
		formatRTT(row.RTTMin, row.HasRTT),
		formatRTT(row.RTTAvg, row.HasRTT),
		formatRTT(row.RTTMax, row.HasRTT),
		formatRTT(row.Jitter, row.HasRTT),
		row.TCPConnect,
		during,
		row.Iface,
		strconv.FormatUint(row.RxErrDelta, 10),
		strconv.FormatUint(row.TxErrDelta, 10),
		strconv.FormatUint(row.RxDropDelta, 10),
		strconv.FormatUint(row.TxDropDelta, 10),
		strconv.FormatUint(row.RxBytesDelta, 10),
		strconv.FormatUint(row.TxBytesDelta, 10),
		row.LinkSpeedMbps,
		row.Duplex,
	}
}

func speedRecord(row SpeedRow) []string {
	return []string{
		row.TS.UTC().Format(time.RFC3339),
		row.URL,
		strconv.FormatInt(row.Bytes, 10),
		strconv.FormatInt(row.Duration.Milliseconds(), 10),
		strconv.FormatFloat(row.Mbps, 'f', 3, 64),
		row.LimitedBy,
	}
}

func formatRTT(v float64, ok bool) string {
	if !ok {
		return ""
	}

	return strconv.FormatFloat(v, 'f', 3, 64)
}

func incidentTime(name string) (time.Time, bool) {
	if len(name) < 16 {
		return time.Time{}, false
	}

	ts, err := time.Parse("20060102T150405Z", name[:16])
	if err != nil {
		return time.Time{}, false
	}

	return ts, true
}

func safeName(target string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			return r
		default:
			return '_'
		}
	}, target)
}

// readCSV is used by tests in this package.
func readCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return records, nil
}
