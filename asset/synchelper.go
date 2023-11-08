package asset

import (
	"context"
	"fmt"
	"sync"

	"github.com/decred/slog"
)

type syncHelper struct {
	log slog.Logger

	mtx        sync.Mutex
	cancelSync context.CancelFunc
	// syncEndedCh is opened when sync is started and closed when sync is ended.
	// Wait on this channel to know when sync has completely stopped.
	syncEndedCh     chan struct{}
	cancelRequested bool
}

// InitializeSyncContext returns a context that should be used for bacgkround
// sync processes. All sync background processes should exit when the returned
// context is canceled. Call SyncEnded() when all sync processes have ended.
func (sh *syncHelper) InitializeSyncContext(ctx context.Context) (context.Context, error) {
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

// IsSyncingOrSynced is true if the wallet synchronization was started.
func (sh *syncHelper) IsSyncingOrSynced() bool {
	sh.mtx.Lock()
	defer sh.mtx.Unlock()
	return sh.cancelSync != nil
}

// SyncEnded signals that all sync processes have been stopped.
func (sh *syncHelper) SyncEnded(err error) {
	sh.mtx.Lock()
	defer sh.mtx.Unlock()

	if sh.cancelSync == nil {
		return // sync wasn't active
	}

	close(sh.syncEndedCh)
	sh.cancelSync = nil
	sh.syncEndedCh = nil
	sh.cancelRequested = false
	sh.log.Infof("sync canceled")
}

// StopSync cancels the wallet's synchronization to the blockchain network. It
// may take a few moments for sync to completely stop. Use
func (sh *syncHelper) StopSync() {
	sh.mtx.Lock()
	defer sh.mtx.Unlock()

	if sh.cancelRequested || sh.cancelSync == nil {
		sh.log.Infof("sync is already canceling or not running")
		return
	}

	sh.log.Infof("canceling sync... this may take a moment")
	sh.cancelRequested = true
	sh.cancelSync()
}

func (sh *syncHelper) SyncIsStopping() bool {
	sh.mtx.Lock()
	defer sh.mtx.Unlock()
	return sh.cancelRequested
}

// WaitForSyncToStop blocks until the wallet synchronization is fully stopped.
func (sh *syncHelper) WaitForSyncToStop() {
	sh.mtx.Lock()
	waitCh := sh.syncEndedCh
	sh.mtx.Unlock()

	if waitCh != nil {
		<-waitCh
	}
}
