package main

import (
	"log"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

var logLevels = map[uint8]slog.Level{
	0: slog.LevelDebug,
	1: slog.LevelInfo,
	2: slog.LevelWarn,
	3: slog.LevelError,
}

type System struct {
	LogLevel   uint8  `env:"SYSTEM_LOG_LEVEL" envDefault:"1"` // 0 - debug, 1 - info, 2 - warn, 3 - error
	HealthAddr string `env:"HEALTH_ADDR" envDefault:"127.0.0.1:19090"`
	DataDir    string `env:"DATA_DIR" envDefault:"data"`
}

type Probe struct {
	Interval     time.Duration `env:"PROBE_INTERVAL" envDefault:"1m"`
	Targets      string        `env:"PROBE_TARGETS" envDefault:"1.1.1.1,8.8.8.8,9.9.9.9"`
	Gateway      bool          `env:"GATEWAY_PROBE" envDefault:"true"`
	PingCount    int           `env:"PING_COUNT" envDefault:"10"`
	PingInterval time.Duration `env:"PING_INTERVAL" envDefault:"200ms"`
	TCPPort      string        `env:"TCP_PORT" envDefault:"443"`
	TCPTimeout   time.Duration `env:"TCP_TIMEOUT" envDefault:"2s"`
	Iface        string        `env:"IFACE" envDefault:""`
}

type Incident struct {
	LossPct   float64       `env:"INCIDENT_LOSS_PCT" envDefault:"1"`
	RTTFactor float64       `env:"INCIDENT_RTT_FACTOR" envDefault:"3"`
	RTTFloor  time.Duration `env:"INCIDENT_RTT_FLOOR" envDefault:"50ms"`
	Cooldown  time.Duration `env:"INCIDENT_COOLDOWN" envDefault:"10m"`
}

type Retention struct {
	Keep     time.Duration `env:"RETENTION" envDefault:"24h"`
	Interval time.Duration `env:"CLEANUP_INTERVAL" envDefault:"1h"`
}

type Speed struct {
	Interval    time.Duration `env:"SPEEDTEST_INTERVAL" envDefault:"30m"`
	URL         string        `env:"SPEEDTEST_URL" envDefault:"https://speed.cloudflare.com/__down?bytes=100000000"`
	MaxDuration time.Duration `env:"SPEEDTEST_MAX_DURATION" envDefault:"8s"`
	MaxBytes    int64         `env:"SPEEDTEST_MAX_BYTES" envDefault:"33554432"`
}

type Config struct {
	System    System
	Probe     Probe
	Incident  Incident
	Retention Retention
	Speed     Speed
	Targets   []string
}

func loadConfig() *Config {
	cfg := &Config{}
	if err := env.Parse(&cfg.System); err != nil {
		log.Fatalf("Failed to parse system config: %v", err)
	}
	if err := env.Parse(&cfg.Probe); err != nil {
		log.Fatalf("Failed to parse probe config: %v", err)
	}
	if err := env.Parse(&cfg.Incident); err != nil {
		log.Fatalf("Failed to parse incident config: %v", err)
	}
	if err := env.Parse(&cfg.Retention); err != nil {
		log.Fatalf("Failed to parse retention config: %v", err)
	}
	if err := env.Parse(&cfg.Speed); err != nil {
		log.Fatalf("Failed to parse speed config: %v", err)
	}

	cfg.Targets = parseTargets(cfg.Probe.Targets)
	if len(cfg.Targets) == 0 && !cfg.Probe.Gateway {
		log.Fatal("no probe targets: set PROBE_TARGETS or GATEWAY_PROBE=true")
	}
	if cfg.Probe.PingCount < 1 {
		log.Fatal("PING_COUNT must be positive")
	}
	if cfg.Probe.PingInterval <= 0 {
		log.Fatal("PING_INTERVAL must be positive")
	}
	if cfg.Probe.Interval <= 0 {
		log.Fatal("PROBE_INTERVAL must be positive")
	}
	if cfg.Speed.URL == "" {
		log.Fatal("SPEEDTEST_URL is empty")
	}
	if cfg.Speed.MaxDuration <= 0 || cfg.Speed.MaxBytes <= 0 {
		log.Fatal("SPEEDTEST_MAX_DURATION and SPEEDTEST_MAX_BYTES must be positive")
	}
	if cfg.Retention.Keep <= 0 || cfg.Retention.Interval <= 0 {
		log.Fatal("RETENTION and CLEANUP_INTERVAL must be positive")
	}

	return cfg
}

func parseTargets(raw string) []string {
	parts := strings.Split(raw, ",")
	targets := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if net.ParseIP(part) == nil {
			log.Fatalf("invalid probe target %q", part)
		}
		targets = append(targets, part)
	}

	return targets
}
