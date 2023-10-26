package asset

import (
	"sync"
	"time"

	"github.com/btcsuite/btcwallet/wtxmgr"
	"github.com/decred/slog"
)

type SyncProgressReporter struct {
	activeSyncLogTicker *time.Ticker
	log                 slog.Logger

	mtx                      sync.RWMutex
	syncStage                SyncStage
	generalProgress          *GeneralSyncProgress
	headersFetchStat         *HeadersFetchStat
	cfiltersFetchProgress    *CFiltersFetchProgressReport
	headersRescanProgress    *HeadersRescanProgressReport
	addressDiscoveryProgress *AddressDiscoveryProgressReport

	wg        sync.WaitGroup
	listeners map[string]SyncProgressListener
}

func InitSyncProgressReporter(activeSyncLogFrequency time.Duration, log slog.Logger) *SyncProgressReporter {
	return &SyncProgressReporter{
		activeSyncLogTicker: time.NewTicker(activeSyncLogFrequency),
		log:                 log,
		syncStage:           InvalidSyncStage,
		generalProgress:     &GeneralSyncProgress{},
		listeners:           make(map[string]SyncProgressListener),
	}
}

func (reporter *SyncProgressReporter) SyncStarted(currentHeight, targetHeight int32) {
	reporter.mtx.Lock()
	defer reporter.mtx.Unlock()

	reporter.syncStage = HeadersFetchSyncStage
	reporter.headersFetchStat = &HeadersFetchStat{
		StatTimeTracker: StatTimeTracker{
			BeginTimeStamp: time.Now(),
		},
		StartHeight:       currentHeight,
		TargetHeight:      targetHeight,
		LastFetchedHeight: currentHeight,
	}

	for _, listener := range reporter.listeners {
		reporter.runInBackground(listener.OnSyncStarted)
	}
}

func (reporter *SyncProgressReporter) HandleBlockConnected(blockHeight int32, relevantTxs []*wtxmgr.TxRecord) {
	reporter.mtx.Lock()
	defer reporter.mtx.Unlock()

	switch {
	case reporter.syncStage == SyncCompleteSyncStage:
		// TODO: notify txs attached
		// TODO: notify block attached
		return

	case reporter.syncStage != HeadersFetchSyncStage:
		reporter.log.Errorf("HandleBlockConnected called on wrong sync stage: %v", reporter.syncStage)
		return
	}

	reporter.headersFetchStat.LastFetchedHeight = blockHeight
	reporter.logProgress(blockHeight, reporter.headersFetchStat.TargetHeight)

	for _, listener := range reporter.listeners {
		reporter.runInBackground(func() {
			listener.OnHeadersFetchProgress(reporter.headersFetchStat)
		})
	}
}

func (reporter *SyncProgressReporter) SyncCompleted() {
	reporter.mtx.Lock()
	defer reporter.mtx.Unlock()

	reporter.syncStage = SyncCompleteSyncStage
	reporter.activeSyncLogTicker.Stop()
	reporter.activeSyncLogTicker = nil

	for _, listener := range reporter.listeners {
		reporter.runInBackground(listener.OnSyncCompleted)
	}
}

func (reporter *SyncProgressReporter) WaitForBacgkroundProcesses() {
	reporter.wg.Wait()
}

func (reporter *SyncProgressReporter) logProgress(currentHeight, targetHeight int32) {
	if reporter.activeSyncLogTicker != nil {
		select {
		case <-reporter.activeSyncLogTicker.C:
			break
		default:
			return
		}
	}

	reporter.log.Infof("Current sync progress update is on block %v, target sync block is %v", currentHeight, targetHeight)
}

func (reporter *SyncProgressReporter) runInBackground(fn func()) {
	reporter.wg.Add(1)
	go func() {
		fn()
		reporter.wg.Done()
	}()
}
