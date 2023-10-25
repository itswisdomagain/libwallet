package assetlog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/decred/slog"
	"github.com/jrick/logrotate/rotator"
)

const maxLogRolls = 8

// filePlusStdOutLogWriter implements an io.Writer that outputs to both standard
// output and the write-end pipe of an initialized log rotator.
type filePlusStdOutLogWriter struct {
	logRotator *rotator.Rotator
}

func (w *filePlusStdOutLogWriter) Write(p []byte) (n int, err error) {
	os.Stdout.Write(p)
	w.logRotator.Write(p)
	return len(p), nil
}

type Logger struct {
	backend     *slog.Backend
	rotator     *rotator.Rotator
	logFilePath string

	mtx        sync.RWMutex
	level      slog.Level
	subloggers map[string]slog.Logger
}

func NewLogger(logDir, logFileName string, fileOnly ...bool) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0744); err != nil {
		return nil, fmt.Errorf("error creating log directory: %w", err)
	}

	logFilePath := filepath.Join(logDir, logFileName)
	r, err := rotator.New(logFilePath, 32*1024, false, maxLogRolls)
	if err != nil {
		return nil, err
	}

	var logWriter io.Writer
	if len(fileOnly) > 0 && fileOnly[0] {
		logWriter = r
	} else {
		logWriter = &filePlusStdOutLogWriter{r}
	}

	return &Logger{
		backend:     slog.NewBackend(logWriter),
		rotator:     r,
		logFilePath: logFilePath,
		level:       slog.LevelInfo,
		subloggers:  make(map[string]slog.Logger),
	}, nil
}

func (l *Logger) FilePath() string {
	return l.logFilePath
}

func (l *Logger) SetAllLevels(lvl string) {
	level, ok := slog.LevelFromString(lvl)
	if !ok {
		return
	}

	l.mtx.Lock()
	defer l.mtx.Unlock()

	l.level = level
	for _, sublogger := range l.subloggers {
		sublogger.SetLevel(level)
	}
}

func (l *Logger) SubLogger(name string) slog.Logger {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	if subLogger, ok := l.subloggers[name]; ok {
		return subLogger
	}

	subLogger := l.backend.Logger(name)
	subLogger.SetLevel(l.level)
	l.subloggers[name] = subLogger
	return subLogger
}

func (l *Logger) Close() error {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	if err := l.rotator.Close(); err != nil {
		return err
	}

	l.backend = nil
	l.rotator = nil
	l.subloggers = nil
	return nil
}
