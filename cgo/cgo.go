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
	"encoding/json"
	"strings"
	"sync"

	"decred.org/dcrwallet/v3/spv"
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

	walletsMtx sync.RWMutex
	wallets    map[string]*wallet
)

//export initialize
func initialize(cLogDir *C.char) *C.char {
	walletsMtx.Lock()
	defer walletsMtx.Unlock()
	if wallets != nil {
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
	wallets = make(map[string]*wallet)

	return resCResponse("libwallet cgo initialized")
}

//export shutdown
func shutdown() *C.char {
	logMtx.RLock()
	log.Debug("libwallet cgo shutting down")
	logMtx.RUnlock()
	walletsMtx.Lock()
	for _, wallet := range wallets {
		if err := wallet.CloseWallet(); err != nil {
			errCResponse("close wallet error: %v", err)
		}
	}
	wallets = nil // cannot be reused unless initialize is called again.
	walletsMtx.Unlock()

	// Stop all remaining background processes and wait for them to stop.
	cancelCtx()
	wg.Wait()

	// Close the logger backend as the last step.
	logMtx.Lock()
	log.Debug("libwallet cgo shutdown")
	logBackend.Close()
	log = nil
	logMtx.Unlock()

	return resCResponse("libwallet cgo shutdown")
}

//export syncWallet
func syncWallet(cName, cPeers *C.char) *C.char {
	name := goString(cName)
	w, exists := wallets[name]
	if !exists {
		return errCResponse("wallet with name %q does not exist", name)
	}
	peersCDS := goString(cPeers)
	peers := strings.Split(peersCDS, ",")
	ntfns := &spv.Notifications{
		Synced: func(sync bool) {
			w.syncStatusMtx.Lock()
			w.syncStatusCode = SSCComplete
			w.syncStatusMtx.Unlock()
			log.Info("Sync completed.")
		},
		PeerConnected: func(peerCount int32, addr string) {
			log.Infof("Connected to peer at %s. %d total peers.", addr, peerCount)
		},
		PeerDisconnected: func(peerCount int32, addr string) {
			log.Infof("Disconnected from peer at %s. %d total peers.", addr, peerCount)
		},
		FetchMissingCFiltersStarted: func() {
			w.syncStatusMtx.Lock()
			w.syncStatusCode = SSCFetchingCFilters
			w.syncStatusMtx.Unlock()
			log.Info("Fetching missing cfilters started.")
		},
		FetchMissingCFiltersProgress: func(startCFiltersHeight, endCFiltersHeight int32) {
			w.syncStatusMtx.Lock()
			w.cfiltersHeight = int(endCFiltersHeight)
			w.syncStatusMtx.Unlock()
			log.Infof("Fetching cfilters from %d to %d.", startCFiltersHeight, endCFiltersHeight)
		},
		FetchMissingCFiltersFinished: func() {
			log.Info("Finished fetching missing cfilters.")
		},
		FetchHeadersStarted: func() {
			w.syncStatusMtx.Lock()
			w.syncStatusCode = SSCFetchingHeaders
			w.syncStatusMtx.Unlock()
			log.Info("Fetching headers started.")
		},
		FetchHeadersProgress: func(lastHeaderHeight int32, lastHeaderTime int64) {
			w.syncStatusMtx.Lock()
			w.headersHeight = int(lastHeaderHeight)
			w.syncStatusMtx.Unlock()
			log.Infof("Fetching headers to %d.", lastHeaderHeight)
		},
		FetchHeadersFinished: func() {
			log.Info("Fetching headers finished.")
		},
		DiscoverAddressesStarted: func() {
			w.syncStatusMtx.Lock()
			w.syncStatusCode = SSCDiscoveringAddrs
			w.syncStatusMtx.Unlock()
			log.Info("Discover addresses started.")
		},
		DiscoverAddressesFinished: func() {
			log.Info("Discover addresses finished.")
		},
		RescanStarted: func() {
			w.syncStatusMtx.Lock()
			w.syncStatusCode = SSCRescanning
			w.syncStatusMtx.Unlock()
			log.Info("Rescan started.")
		},
		RescanProgress: func(rescannedThrough int32) {
			w.syncStatusMtx.Lock()
			w.rescanHeight = int(rescannedThrough)
			w.syncStatusMtx.Unlock()
			log.Infof("Rescanned through block %d.", rescannedThrough)
		},
		RescanFinished: func() {
			log.Info("Rescan finished.")
		},
	}
	if err := w.StartSync(ctx, ntfns, peers...); err != nil {
		return errCResponse(err.Error())
	}
	return resCResponse("sync started")
}

//export syncWalletStatus
func syncWalletStatus(cName *C.char) *C.char {
	name := goString(cName)
	w, exists := wallets[name]
	if !exists {
		return errCResponse("wallet with name %q does not exist", name)
	}

	w.syncStatusMtx.RLock()
	var ssc, cfh, hh, rh = w.syncStatusCode, w.cfiltersHeight, w.headersHeight, w.rescanHeight
	w.syncStatusMtx.RUnlock()

	nb, err := w.NetworkBackend()
	if err != nil {
		return errCResponse("unable to get network backend", err)
	}
	spvSyncer, is := nb.(*spv.Syncer)
	if !is {
		return errCResponse("backend is not an spv syncer")
	}
	targetHeight := spvSyncer.EstimateMainChainTip(ctx)

	ss := &SyncStatusRes{
		SyncStatusCode: int(ssc),
		SyncStatus:     ssc.String(),
		TargetHeight:   int(targetHeight),
	}
	switch ssc {
	case SSCFetchingCFilters:
		ss.CFiltersHeight = cfh
	case SSCFetchingHeaders:
		ss.HeadersHeight = hh
	case SSCRescanning:
		ss.RescanHeight = rh
	}
	b, err := json.Marshal(ss)
	if err != nil {
		return errCResponse("unable to marshal sync status result", err)
	}
	return resCResponse(string(b))
}

func main() {}
