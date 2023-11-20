package btc

import (
	"fmt"
	"sync/atomic"

	"github.com/btcsuite/btclog"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/wtxmgr"
	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/assetlog"
	"github.com/lightninglabs/neutrino"
)

// loggingInited will be set when the log rotator has been initialized.
var loggingInited uint32

const LogFileName = "external.log"

// InitGlobalLogging initializes logging in the dcrwallet packages, writing the
// logs to a log file in the specified logDir. If desired, log messages with
// level >= warn can be additionally written to a separate logger. To achieve
// this, pass a non-nil ParentLogger that can be used to create error-only
// loggers.
//
// Logging only has to be initialized once, so an atomic flag is used internally
// to return early on subsequent invocations.
//
// In theory, the rotating file logger must be Closed at some point, but
// there are concurrency issues with that since btcd and btcwallet have
// unsupervised goroutines still running after shutdown. So we leave the rotator
// running at the risk of losing some logs.
func InitGlobalLogging(externalLogDir string, errorLogger assetlog.ParentLogger) error {
	if !atomic.CompareAndSwapUint32(&loggingInited, 0, 1) {
		return nil
	}

	backendLog, err := assetlog.NewLogger(externalLogDir, LogFileName, true)
	if err != nil {
		return fmt.Errorf("error initializing logger: %w", err)
	}

	logger := func(name string, lvl slog.Level) btclog.Logger {
		l := backendLog.SubLogger(name)
		l.SetLevel(lvl)
		if errorLogger != nil {
			l = assetlog.NewLoggerPlus(l, errorLogger.SubLogger(name))
		}
		return &assetlog.BTCLogger{Logger: l}
	}

	neutrino.UseLogger(logger("NTRNO", slog.LevelDebug))
	wallet.UseLogger(logger("BTCW", slog.LevelInfo))
	wtxmgr.UseLogger(logger("TXMGR", slog.LevelInfo))
	chain.UseLogger(logger("CHAIN", slog.LevelInfo))

	return nil
}
