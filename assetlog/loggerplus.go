package assetlog

import (
	"github.com/btcsuite/btclog"
	"github.com/decred/slog"
)

type ParentLogger interface {
	SubLogger(name string) slog.Logger
}

// LoggerPlus logs everything to a main logger, and everything with level >= warn
// to a second logger.
type LoggerPlus struct {
	slog.Logger
	log slog.Logger
}

func NewLoggerPlus(mainLogger, errorLogger slog.Logger) *LoggerPlus {
	return &LoggerPlus{mainLogger, errorLogger}
}

func (f *LoggerPlus) Warnf(format string, params ...interface{}) {
	f.log.Warnf(format, params...)
	f.Logger.Warnf(format, params...)
}

func (f *LoggerPlus) Errorf(format string, params ...interface{}) {
	f.log.Errorf(format, params...)
	f.Logger.Errorf(format, params...)
}

func (f *LoggerPlus) Criticalf(format string, params ...interface{}) {
	f.log.Criticalf(format, params...)
	f.Logger.Criticalf(format, params...)
}

func (f *LoggerPlus) Warn(v ...interface{}) {
	f.log.Warn(v...)
	f.Logger.Warn(v...)
}

func (f *LoggerPlus) Error(v ...interface{}) {
	f.log.Error(v...)
	f.Logger.Error(v...)
}

func (f *LoggerPlus) Critical(v ...interface{}) {
	f.log.Critical(v...)
	f.Logger.Critical(v...)
}

type BTCLogger struct {
	slog.Logger
}

func (btcLog *BTCLogger) Level() btclog.Level {
	lvl := btcLog.Logger.Level().String()
	btcLvl, _ := btclog.LevelFromString(lvl)
	return btcLvl
}

func (btcLog *BTCLogger) SetLevel(btcLvl btclog.Level) {
	lvl, _ := slog.LevelFromString(btcLvl.String())
	btcLog.Logger.SetLevel(lvl)
}
