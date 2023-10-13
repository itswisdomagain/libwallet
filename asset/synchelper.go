package asset

import (
	"context"
)

type SyncHelper struct {
	cancelSync context.CancelFunc
	// syncEndedCh is opened when sync is started and closed when sync is ended.
	// Wait on this channel to know when sync has completely stopped.
	syncEndedCh chan struct{}
}

// InitSyncHelper initializes a SyncHelper for managing a wallet synchronization
// process. The returned ctx should be used by the caller to determine when the
// sync process should be stopped.
func InitSyncHelper(ctx context.Context) (context.Context, *SyncHelper) {
	syncCtx, cancelSync := context.WithCancel(ctx)
	return syncCtx, &SyncHelper{
		cancelSync:  cancelSync,
		syncEndedCh: make(chan struct{}),
	}
}

// IsActive is true unless the ShutdownComplete() was called on this helper to
// signal that the sync process(es) have all stopped.
func (sh *SyncHelper) IsActive() bool {
	select {
	case <-sh.syncEndedCh:
		return true
	default:
		return false
	}
}

// Shutdown cancels the syncCtx and begins the process of stopping the network
// synchronization. When all sub-processes have ended, call ShutdownComplete()
// to signal that sync has fully stopped.
func (sh *SyncHelper) Shutdown() {
	sh.cancelSync()
}

// ShutdownComplete signals that the wallet synchronization has been completely
// stopped.
func (sh *SyncHelper) ShutdownComplete() {
	close(sh.syncEndedCh)
}

// WaitForShutdown blocks until the wallet synchronization is fully stopped.
func (sh *SyncHelper) WaitForShutdown() {
	<-sh.syncEndedCh
}
