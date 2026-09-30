package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"netscan/internal/store"
	"netscan/internal/worker"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	config := loadConfig()

	logLevel := slog.LevelInfo
	if level, ok := logLevels[config.System.LogLevel]; ok {
		logLevel = level
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	samples, err := store.New(config.System.DataDir, config.Retention.Keep)
	if err != nil {
		logger.Error("failed to open data dir", slog.String("error", err.Error()))
		return err
	}

	w := worker.New(worker.Config{
		HealthAddr:           config.System.HealthAddr,
		ProbeInterval:        config.Probe.Interval,
		Targets:              config.Targets,
		GatewayProbe:         config.Probe.Gateway,
		PingCount:            config.Probe.PingCount,
		PingInterval:         config.Probe.PingInterval,
		TCPTimeout:           config.Probe.TCPTimeout,
		TCPPort:              config.Probe.TCPPort,
		Iface:                config.Probe.Iface,
		IncidentLossPct:      config.Incident.LossPct,
		IncidentRTTFactor:    config.Incident.RTTFactor,
		IncidentRTTFloor:     config.Incident.RTTFloor,
		IncidentCooldown:     config.Incident.Cooldown,
		CleanupInterval:      config.Retention.Interval,
		SpeedtestInterval:    config.Speed.Interval,
		SpeedtestURL:         config.Speed.URL,
		SpeedtestMaxDuration: config.Speed.MaxDuration,
		SpeedtestMaxBytes:    config.Speed.MaxBytes,
	}, logger, samples)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("netscan started",
		slog.String("health", config.System.HealthAddr),
		slog.String("data", config.System.DataDir),
	)

	if err := w.Run(ctx); err != nil {
		logger.Error("netscan stopped", slog.String("error", err.Error()))
		return err
	}

	logger.Info("netscan stopped")
	return nil
}
