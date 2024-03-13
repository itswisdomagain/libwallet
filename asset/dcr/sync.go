package dcr

import (
	"context"
	"net"
	"time"

	"decred.org/dcrwallet/v3/p2p"
	"decred.org/dcrwallet/v3/spv"
	"github.com/decred/dcrd/addrmgr/v2"
)

// StartSync connects the wallet to the blockchain network via SPV and returns
// immediately. The wallet stays connected in the background until the provided
// ctx is canceled or either StopSync or CloseWallet is called.
func (w *Wallet) StartSync(ctx context.Context, ntfns *spv.Notifications, connectPeers ...string) error {
	// Initialize the ctx to use for sync. Will error if sync was already
	// started.
	ctx, err := w.InitializeSyncContext(ctx)
	if err != nil {
		return err
	}

	w.log.Info("Starting sync...")

	addr := &net.TCPAddr{IP: net.ParseIP("::1"), Port: 0}
	amgr := addrmgr.New(w.dir, net.LookupIP)
	lp := p2p.NewLocalPeer(w.ChainParams(), addr, amgr)
	syncer := spv.NewSyncer(w.mainWallet, lp)
	if len(connectPeers) > 0 {
		syncer.SetPersistentPeers(connectPeers)
	}
	syncer.SetNotifications(ntfns)

	w.syncer = syncer
	w.SetNetworkBackend(syncer)

	// Start the syncer in a goroutine, monitor when the sync ctx is canceled
	// and then disconnect the sync.
	go func() {
		for {
			err := syncer.Run(ctx)
			if ctx.Err() != nil {
				// sync ctx canceled, quit syncing
				w.syncer = nil
				w.SetNetworkBackend(nil)
				w.SyncEnded(nil)
				return
			}

			w.log.Errorf("SPV synchronization ended. Trying again in 10 seconds: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second * 10):
			}
		}
	}()

	return nil
}

// IsSyncing returns true if the wallet is catching up to the mainchain's best
// block.
func (w *Wallet) IsSyncing() bool {
	if w.IsSynced() {
		return false
	}
	return w.IsSyncingOrSynced()
}

// IsSynced returns true if the wallet has synced up to the best block on the
// mainchain.
func (w *Wallet) IsSynced() bool {
	if w.syncer != nil {
		return w.syncer.Synced()
	}
	return false
}

// RescanFromHeight rescans the wallet from the specified height.
func (w *Wallet) RescanFromHeight(ctx context.Context, startHeight int32) error {
	return w.mainWallet.RescanFromHeight(ctx, w.syncer, startHeight)
}
