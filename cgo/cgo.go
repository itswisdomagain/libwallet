// A pacakge that exports Decred wallet functionalities as go code that can be
// compiled into a c-shared libary. Must be a main package, with an empty main
// function. And functions to be exported must have an "//export {fnName}"
// comment.
//
// Build cmd: go build -buildmode=c-archive -o {path_to_generated_library} ./cgo
// E.g. go build -buildmode=c-archive -o ./build/libdcrwallet.a ./cgo.

package main

import "C"
import (
	"context"
	"sync"

	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/asset/dcr"
	"github.com/itswisdomagain/libwallet/assetlog"
)

var (
	ctx       context.Context
	cancelCtx context.CancelFunc
	wg        sync.WaitGroup

	logBackend *parentLogger
	log        slog.Logger

	walletsMtx sync.RWMutex
	wallets    map[string]*wallet
)

//export initialize
func initialize(cLogDir *C.char) *C.char {
	walletsMtx.Lock()
	defer walletsMtx.Unlock()
	if wallets != nil {
		return cString("duplicate initialization")
	}

	logDir := goString(cLogDir)
	logSpinner, err := assetlog.NewRotator(logDir, "dcrwallet.log")
	if err != nil {
		return cStringF("error initializing log rotator: %v", err)
	}

	logBackend = newParentLogger(logSpinner)
	err = dcr.InitGlobalLogging(logDir, logBackend)
	if err != nil {
		return cStringF("error initializing logger for external pkgs: %v", err)
	}

	log = logBackend.SubLogger("[APP]")
	log.SetLevel(slog.LevelTrace)

	ctx, cancelCtx = context.WithCancel(context.Background())
	wallets = make(map[string]*wallet)

	log.Info("libwallet cgo initialized")
	return nil
}

//export shutdown
func shutdown() {
	walletsMtx.Lock()
	for _, wallet := range wallets {
		if err := wallet.CloseWallet(); err != nil {
			wallet.log.Errorf("close wallet error: %v", err)
		}
	}
	wallets = nil // cannot be reused unless initialize is called again.
	walletsMtx.Unlock()

	// Stop all remaining background processes and wait for them to stop.
	cancelCtx()
	wg.Wait()

	// Close the logger backend as the last step.
	logBackend.Close()
}

func main() {}
