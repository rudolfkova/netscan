package worker

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"netscan/internal/probe"
	"netscan/internal/store"
)

const minRTTSamples = 5

type Config struct {
	HealthAddr    string
	ProbeInterval time.Duration
	Targets       []string
	GatewayProbe  bool
	PingCount     int
	PingInterval  time.Duration
	TCPTimeout    time.Duration
	TCPPort       string
	Iface         string

	IncidentLossPct   float64
	IncidentRTTFactor float64
	IncidentRTTFloor  time.Duration
	IncidentCooldown  time.Duration

	CleanupInterval time.Duration

	SpeedtestInterval    time.Duration
	SpeedtestURL         string
	SpeedtestMaxDuration time.Duration
	SpeedtestMaxBytes    int64

	RoutePath  string
	NetDevPath string
	SysNetPath string
}

type Worker struct {
	cfg   Config
	log   *slog.Logger
	store *store.Store
	hist  *probe.RTTHistory

	speed atomic.Bool
	mtr   atomic.Bool

	havePrev  bool
	prevIface string
	prev      probe.Counters
	lastMTR   time.Time
}

func New(cfg Config, logger *slog.Logger, samples *store.Store) *Worker {
	return &Worker{
		cfg:   cfg,
		log:   logger,
		store: samples,
		hist:  probe.NewRTTHistory(time.Hour),
	}
}

func (w *Worker) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- w.serveHealth(ctx)
	}()

	go w.probeLoop(ctx)
	go w.speedLoop(ctx)
	go w.cleanupLoop(ctx)

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func (w *Worker) serveHealth(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("ok\n"))
	})

	srv := &http.Server{
		Addr:    w.cfg.HealthAddr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()

	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func (w *Worker) probeLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		start := time.Now()
		w.probeOnce(ctx)

		wait := w.cfg.ProbeInterval - time.Since(start)
		if wait < 0 {
			wait = 0
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (w *Worker) speedLoop(ctx context.Context) {
	timer := time.NewTimer(w.cfg.SpeedtestInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			w.speedOnce(ctx)
			timer.Reset(w.cfg.SpeedtestInterval)
		}
	}
}

func (w *Worker) cleanupLoop(ctx context.Context) {
	w.cleanup()

	ticker := time.NewTicker(w.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.cleanup()
		}
	}
}

func (w *Worker) cleanup() {
	removed, err := w.store.Cleanup(time.Now().UTC())
	if err != nil {
		w.log.Error("cleanup failed", slog.String("error", err.Error()))
		return
	}
	if removed > 0 {
		w.log.Info("cleaned old samples", slog.Int("removed", removed))
	}
}

type targetResult struct {
	ip    string
	kind  string
	stats probe.PingStats
	err   error
	tcpMs float64
	tcpOK bool
}

func (w *Worker) probeOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	now := time.Now().UTC()
	iface, gateway, err := probe.DefaultRoute(w.routePath())
	if err != nil {
		w.log.Error("failed to read default route", slog.String("error", err.Error()))
	}
	if w.cfg.Iface != "" {
		iface = w.cfg.Iface
	}

	var deltas probe.Deltas
	if iface != "" {
		cur, err := probe.ReadIface(w.netDevPath(), iface)
		if err != nil {
			w.log.Error("failed to read interface counters", slog.String("iface", iface), slog.String("error", err.Error()))
		} else {
			deltas = probe.CounterDelta(w.prev, cur, w.havePrev && w.prevIface == iface)
			w.prev = cur
			w.prevIface = iface
			w.havePrev = true
		}
	}

	linkSpeed, duplex := "", ""
	if iface != "" {
		linkSpeed, duplex = probe.LinkState(w.sysNetPath(), iface)
	}

	type job struct {
		ip   string
		kind string
	}
	var jobs []job
	if w.cfg.GatewayProbe && gateway != "" {
		jobs = append(jobs, job{ip: gateway, kind: probe.KindGateway})
	}
	for _, ip := range w.cfg.Targets {
		jobs = append(jobs, job{ip: ip, kind: probe.KindExternal})
	}
	if len(jobs) == 0 {
		w.log.Error("no probe targets")
		return
	}

	duringStart := w.speed.Load()
	results := make([]targetResult, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, j job) {
			defer wg.Done()

			var (
				tcpMs float64
				tcpOK bool
				inner sync.WaitGroup
			)
			if j.kind == probe.KindExternal {
				inner.Add(1)
				go func() {
					defer inner.Done()
					tcpMs, tcpOK = probe.TCPConnect(ctx, j.ip, w.cfg.TCPPort, w.cfg.TCPTimeout)
				}()
			}

			stats, err := probe.Ping(ctx, j.ip, w.cfg.PingCount, w.cfg.PingInterval)
			inner.Wait()
			results[i] = targetResult{
				ip:    j.ip,
				kind:  j.kind,
				stats: stats,
				err:   err,
				tcpMs: tcpMs,
				tcpOK: tcpOK,
			}
		}(i, j)
	}
	wg.Wait()

	during := duringStart || w.speed.Load()
	thr := probe.Threshold{
		LossPct:    w.cfg.IncidentLossPct,
		RTTFactor:  w.cfg.IncidentRTTFactor,
		RTTFloorMs: float64(w.cfg.IncidentRTTFloor.Milliseconds()),
		MinSamples: minRTTSamples,
	}

	rows := make([]store.ProbeRow, 0, len(results))
	samples := make([]probe.Sample, 0, len(results))
	worstLoss := 0.0

	for _, res := range results {
		if res.err != nil {
			w.log.Error("ping failed", slog.String("target", res.ip), slog.String("error", res.err.Error()))
			continue
		}

		median, n := 0.0, 0
		if res.stats.HasRTT {
			median, n = w.hist.Median(res.ip, now)
		}
		tripped := thr.Tripped(res.stats.LossPct, res.stats.Avg, res.stats.HasRTT, median, n)
		if res.stats.HasRTT {
			w.hist.Add(res.ip, now, res.stats.Avg)
		}

		tcp := ""
		if res.kind == probe.KindExternal {
			tcp = "-1"
			if res.tcpOK {
				tcp = strconv.FormatFloat(res.tcpMs, 'f', 1, 64)
			}
		}

		if res.stats.LossPct > worstLoss {
			worstLoss = res.stats.LossPct
		}

		w.log.Debug("target",
			slog.String("target", res.ip),
			slog.String("kind", res.kind),
			slog.Float64("loss_pct", res.stats.LossPct),
			slog.Float64("rtt_avg_ms", res.stats.Avg),
			slog.Bool("incident", tripped),
		)

		rows = append(rows, store.ProbeRow{
			TS:              now,
			Target:          res.ip,
			Kind:            res.kind,
			Sent:            res.stats.Sent,
			Recv:            res.stats.Recv,
			LossPct:         res.stats.LossPct,
			RTTMin:          res.stats.Min,
			RTTAvg:          res.stats.Avg,
			RTTMax:          res.stats.Max,
			Jitter:          res.stats.Mdev,
			HasRTT:          res.stats.HasRTT,
			TCPConnect:      tcp,
			DuringSpeedtest: during,
			Iface:           iface,
			RxErrDelta:      deltas.RxErrs,
			TxErrDelta:      deltas.TxErrs,
			RxDropDelta:     deltas.RxDrop,
			TxDropDelta:     deltas.TxDrop,
			RxBytesDelta:    deltas.RxBytes,
			TxBytesDelta:    deltas.TxBytes,
			LinkSpeedMbps:   linkSpeed,
			Duplex:          duplex,
		})
		samples = append(samples, probe.Sample{
			Target:   res.ip,
			Kind:     res.kind,
			Loss:     res.stats.LossPct,
			RTT:      res.stats.Avg,
			HasRTT:   res.stats.HasRTT,
			Incident: tripped,
		})
	}

	if err := w.store.AppendProbes(rows); err != nil {
		w.log.Error("failed to write probes", slog.String("error", err.Error()))
	}

	w.maybeMTR(ctx, now, samples, during)
	w.log.Info("probe complete",
		slog.Int("rows", len(rows)),
		slog.String("iface", iface),
		slog.Float64("worst_loss_pct", worstLoss),
		slog.Bool("during_speedtest", during),
	)
}

func (w *Worker) maybeMTR(ctx context.Context, now time.Time, samples []probe.Sample, duringSpeedtest bool) {
	fallback := ""
	if len(w.cfg.Targets) > 0 {
		fallback = w.cfg.Targets[0]
	}

	target, ok := probe.MTRTarget(samples, fallback)
	if !ok {
		return
	}

	if duringSpeedtest || w.speed.Load() {
		w.log.Warn("network degraded", slog.String("target", target), slog.Bool("during_speedtest", true), slog.Bool("mtr", false))
		return
	}

	if !w.lastMTR.IsZero() && now.Sub(w.lastMTR) < w.cfg.IncidentCooldown {
		w.log.Warn("network degraded", slog.String("target", target), slog.Bool("mtr", false))
		return
	}
	if !w.mtr.CompareAndSwap(false, true) {
		w.log.Warn("network degraded", slog.String("target", target), slog.Bool("mtr_busy", true))
		return
	}
	w.lastMTR = now

	go func() {
		defer w.mtr.Store(false)

		out, err := probe.MTR(ctx, target)
		body := out
		if len(body) == 0 && err != nil {
			body = []byte(err.Error() + "\n")
		}

		path, werr := w.store.WriteIncident(time.Now().UTC(), target, body)
		if werr != nil {
			w.log.Error("failed to write mtr", slog.String("error", werr.Error()))
			return
		}
		if err != nil {
			w.log.Warn("network degraded", slog.String("target", target), slog.String("mtr", path), slog.String("error", err.Error()))
			return
		}

		w.log.Warn("network degraded", slog.String("target", target), slog.String("mtr", path))
	}()
}

func (w *Worker) speedOnce(ctx context.Context) {
	if w.mtr.Load() {
		w.log.Info("skip speedtest while mtr is running")
		return
	}

	w.speed.Store(true)
	defer w.speed.Store(false)

	res, err := probe.Download(ctx, w.cfg.SpeedtestURL, w.cfg.SpeedtestMaxDuration, w.cfg.SpeedtestMaxBytes)
	if err != nil {
		w.log.Error("speedtest failed", slog.String("error", err.Error()))
		return
	}

	if err := w.store.AppendSpeed(store.SpeedRow{
		TS:        time.Now().UTC(),
		URL:       w.cfg.SpeedtestURL,
		Bytes:     res.Bytes,
		Duration:  res.Duration,
		Mbps:      res.Mbps,
		LimitedBy: res.LimitedBy,
	}); err != nil {
		w.log.Error("failed to write speedtest", slog.String("error", err.Error()))
		return
	}

	w.log.Info("speedtest",
		slog.Float64("mbps", res.Mbps),
		slog.String("limited_by", res.LimitedBy),
		slog.Int64("bytes", res.Bytes),
	)
}

func (w *Worker) routePath() string {
	if w.cfg.RoutePath != "" {
		return w.cfg.RoutePath
	}

	return "/proc/net/route"
}

func (w *Worker) netDevPath() string {
	if w.cfg.NetDevPath != "" {
		return w.cfg.NetDevPath
	}

	return "/proc/net/dev"
}

func (w *Worker) sysNetPath() string {
	if w.cfg.SysNetPath != "" {
		return w.cfg.SysNetPath
	}

	return "/sys/class/net"
}
