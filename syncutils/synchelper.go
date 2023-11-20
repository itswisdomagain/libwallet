package syncutils

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/decred/slog"
)

type SyncHelper[Tx any] struct {
	log slog.Logger

	mtx             sync.RWMutex
	cancelSync      context.CancelFunc
	syncEndedCh     chan struct{} // TODO
	cancelRequested bool

	rescanMtx         sync.RWMutex
	rescanStartTime   time.Time
	rescanStartHeight int32
	rescanEndHeight   int32

	syncProgressListeners   *lister[SyncProgressListener]
	txAndBlockNtfnListeners *lister[TxAndBlockNtfnListener[Tx]]
	blocksRescanListeners   *lister[BlocksRescanListener]
}

func NewSyncHelper[Tx any](log slog.Logger) *SyncHelper[Tx] {
	return &SyncHelper[Tx]{
		log: log,

		syncProgressListeners:   newLister[SyncProgressListener](),
		txAndBlockNtfnListeners: newLister[TxAndBlockNtfnListener[Tx]](),
		blocksRescanListeners:   newLister[BlocksRescanListener](),
	}
}

// InitializeSyncContext indicates that the wallet synchronization is about to
// start and returns a context that should be used for all sync activity. All
// sync activities should be stopped when the returned context is canceled.
// Returns an error if wallet synchronization is already ongoing.
func (sh *SyncHelper[Tx]) InitializeSyncContext(ctx context.Context) (context.Context, error) {
	sh.mtx.Lock()
	defer sh.mtx.Unlock()

	if sh.cancelSync != nil {
		return nil, fmt.Errorf("already syncing")
	}

	syncCtx, cancel := context.WithCancel(ctx)
	sh.cancelSync = cancel
	sh.syncEndedCh = make(chan struct{})
	sh.cancelRequested = false
	return syncCtx, nil
}

// IsConnectedToNetwork is true if the wallet synchronization is ongoing.
func (sh *SyncHelper[Tx]) IsConnectedToNetwork() bool {
	sh.mtx.RLock()
	defer sh.mtx.RUnlock()
	return sh.cancelSync != nil
}

// StopSync signals that wallet synchronization to the blockchain network should
// be stopped. This method returns immediately but it might take a few moments
// for the wallet synchronization to actually stop.
func (sh *SyncHelper[Tx]) StopSync() {
	sh.mtx.Lock()
	defer sh.mtx.Unlock()

	if sh.cancelRequested || sh.cancelSync == nil {
		sh.log.Infof("Sync is already canceling or not running")
		return
	}

	sh.cancelSyncCtx()
}

// cancelSyncCtx requires a write-lock on sh.mtx. The cancelSync function must
// also be non-nil.
func (sh *SyncHelper[Tx]) cancelSyncCtx() {
	sh.log.Infof("Canceling sync... this may take a moment")
	sh.cancelRequested = true
	sh.cancelSync()
}

// SyncHasStopped signals that the wallet has stopped synchronizing with the
// network and all sync processes have stopped.
func (sh *SyncHelper[Tx]) SyncHasStopped(err error) {
	sh.mtx.Lock()
	defer sh.mtx.Unlock()

	if sh.cancelSync == nil {
		return // sync wasn't active
	}

	if !sh.cancelRequested {
		sh.log.Warnf("Call to SyncEnded() isn't preceded by call to StopSync()!")
		sh.cancelSyncCtx() // cancel the sync ctx now just to ensure necessary cleanup is done
	}

	// Sync has ended so sync progress reports won't be published to listeners
	// anymore. Wait for the progress listeners to finish processing previous
	// progress reports.
	sh.syncProgressListeners.wg.Wait()
	sh.txAndBlockNtfnListeners.wg.Wait()

	close(sh.syncEndedCh)
	sh.cancelSync = nil
	sh.syncEndedCh = nil
	sh.cancelRequested = false
	sh.log.Infof("Sync canceled")
}

// SyncIsStopping returns whether there is a pending request to stop the wallet
// synchronization.
func (sh *SyncHelper[Tx]) SyncIsStopping() bool {
	sh.mtx.RLock()
	defer sh.mtx.RUnlock()
	return sh.cancelRequested
}

// WaitForSyncToStop blocks until the wallet synchronization is fully stopped.
func (sh *SyncHelper[Tx]) WaitForSyncToStop() {
	sh.mtx.RLock()
	waitCh := sh.syncEndedCh
	sh.mtx.RUnlock()

	if waitCh != nil {
		<-waitCh
	}
}

// IsRescanning is true if the wallet is rescanning blocks.
func (sh *SyncHelper[Tx]) IsRescanning() bool {
	sh.rescanMtx.RLock()
	defer sh.rescanMtx.RUnlock()
	return !sh.rescanStartTime.IsZero()
}

// InitializeRescan signals that rescan is about to start and initializes rescan
// variables.
func (sh *SyncHelper[Tx]) InitializeRescan(from, to int32) error {
	sh.rescanMtx.Lock()
	defer sh.rescanMtx.Unlock()

	if !sh.rescanStartTime.IsZero() {
		return fmt.Errorf("already rescanning")
	}

	sh.log.Infof("Rescan started from block %d to %d", from, to)
	sh.rescanStartTime = time.Now()
	sh.rescanStartHeight = from
	sh.rescanEndHeight = to
	return nil
}

func (sh *SyncHelper[Tx]) PublishRescanProgress(scannedThrough int32, logProgress bool) {
	sh.mtx.RLock()
	startTime, startHeight, endHeight := sh.rescanStartTime, sh.rescanStartHeight, sh.rescanEndHeight
	sh.mtx.RUnlock()

	if startTime.IsZero() {
		sh.log.Warnf("PublishRescanProgress called without an active rescan process")
		return
	}

	report := &SyncActivityReport{
		StartTimeStamp: startTime,
		StartHeight:    startHeight,
		TargetHeight:   endHeight,
		LastHeight:     scannedThrough,
	}
	sh.blocksRescanListeners.RangeAsync(func(listener BlocksRescanListener) {
		listener.OnBlocksRescanProgress(report, nil)
	})

	if logProgress || scannedThrough == endHeight {
		sh.log.Infof("Rescanned %d blocks. Remaining %d.", scannedThrough, endHeight-scannedThrough)
	}
}

func (sh *SyncHelper[Tx]) RescanEnded(err error) {
	sh.mtx.Lock()
	rescanInactive := sh.rescanStartTime.IsZero()
	sh.rescanStartTime = time.Time{}
	sh.mtx.Unlock()

	if rescanInactive {
		sh.log.Warnf("RescanEnded called without an active rescan process")
		return
	}

	if err == nil {
		sh.log.Info("Rescan ended without any error")
	} else {
		sh.log.Infof("Rescan ended with error: %v", err)
	}

	sh.blocksRescanListeners.RangeAsync(func(listener BlocksRescanListener) {
		listener.OnBlocksRescanProgress(nil, err)
	})
}

// BlockSyncAccess blocks any attempt to start or stop sync until the returned
// function is called.
func (sh *SyncHelper[Tx]) BlockSyncAccess() func() {
	sh.mtx.Lock()
	return sh.mtx.Unlock
}

func (sh *SyncHelper[Tx]) AddSyncProgressListener(id string, listener SyncProgressListener) error {
	return sh.syncProgressListeners.Add(id, listener)
}

func (sh *SyncHelper[Tx]) RemoveSyncProgressListener(id string) {
	sh.syncProgressListeners.Remove(id)
}

func (sh *SyncHelper[Tx]) PublishSyncProgressReport(notifyFn func(SyncProgressListener)) {
	sh.syncProgressListeners.RangeAsync(notifyFn)
}

func (sh *SyncHelper[Tx]) AddTxAndBlockNtfnListener(id string, listener TxAndBlockNtfnListener[Tx]) error {
	return sh.txAndBlockNtfnListeners.Add(id, listener)
}

func (sh *SyncHelper[Tx]) RemoveTxAndBlockNtfnListener(id string) {
	sh.txAndBlockNtfnListeners.Remove(id)
}

func (sh *SyncHelper[Tx]) NotifyTxAndBlockNtfnListeners(notifyFn func(TxAndBlockNtfnListener[Tx])) {
	sh.txAndBlockNtfnListeners.RangeAsync(notifyFn)
}

func (sh *SyncHelper[Tx]) AddBlocksRescanListener(id string, listener BlocksRescanListener) error {
	return sh.blocksRescanListeners.Add(id, listener)
}

func (sh *SyncHelper[Tx]) RemoveBlocksRescanListener(id string) {
	sh.blocksRescanListeners.Remove(id)
}

func (sh *SyncHelper[Tx]) NotifyBlocksRescanListeners(notifyFn func(BlocksRescanListener)) {
	sh.blocksRescanListeners.RangeAsync(notifyFn)
}
