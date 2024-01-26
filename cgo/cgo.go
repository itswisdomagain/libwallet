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
	logMtx     sync.RWMutex
	log        slog.Logger

	// walletsMtx protects wallets and initialized.
	walletsMtx  sync.RWMutex
	wallets     = make(map[string]*wallet)
	initialized bool
)

//export initialize
func initialize(cLogDir *C.char) *C.char {
	walletsMtx.Lock()
	defer walletsMtx.Unlock()
	if initialized {
		return errCResponse("duplicate initialization")
	}

	logDir := goString(cLogDir)
	logSpinner, err := assetlog.NewRotator(logDir, "dcrwallet.log")
	if err != nil {
		return errCResponse("error initializing log rotator: %v", err)
	}

	logBackend = newParentLogger(logSpinner)
	err = dcr.InitGlobalLogging(logDir, logBackend)
	if err != nil {
		return errCResponse("error initializing logger for external pkgs: %v", err)
	}

	logMtx.Lock()
	log = logBackend.SubLogger("[APP]")
	log.SetLevel(slog.LevelTrace)
	logMtx.Unlock()

	ctx, cancelCtx = context.WithCancel(context.Background())

	initialized = true
	return successCResponse("libwallet cgo initialized")
}

//export shutdown
func shutdown() *C.char {
	logMtx.RLock()
	log.Debug("libwallet cgo shutting down")
	logMtx.RUnlock()
	walletsMtx.Lock()
	defer walletsMtx.Unlock()
	if !initialized {
		return errCResponse("not initialized")
	}
	for _, wallet := range wallets {
		if err := wallet.CloseWallet(); err != nil {
			wallet.log.Errorf("close wallet error: %v", err)
		}
	}
	wallets = make(map[string]*wallet)

	// Stop all remaining background processes and wait for them to stop.
	cancelCtx()
	wg.Wait()

	// Close the logger backend as the last step.
	logMtx.Lock()
	log.Debug("libwallet cgo shutdown")
	logBackend.Close()
	log = nil
	logMtx.Unlock()

	initialized = false
	return successCResponse("libwallet cgo shutdown")
}

func main() {}
