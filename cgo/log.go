package main

import (
	"github.com/decred/slog"
	"github.com/jrick/logrotate/rotator"
)

type parentLogger struct {
	*slog.Backend
	rotator *rotator.Rotator
}

func newParentLogger(rotator *rotator.Rotator) *parentLogger {
	return &parentLogger{
		Backend: slog.NewBackend(rotator),
		rotator: rotator,
	}
}

func (pl *parentLogger) SubLogger(name string) slog.Logger {
	return pl.Logger(name)
}

func (pl *parentLogger) Close() error {
	return pl.rotator.Close()
}
