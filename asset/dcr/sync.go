package dcr

import (
	"context"
	"errors"
	"net"
	"time"

	"decred.org/dcrwallet/v3/p2p"
	"decred.org/dcrwallet/v3/spv"
	"github.com/decred/dcrd/addrmgr/v2"
	"github.com/itswisdomagain/libwallet/asset"
)

type spvSyncer struct {
	*spv.Syncer
	*asset.SyncHelper
}

// StartSync connects the wallet to the blockchain network via SPV and returns
// immediately. The wallet stays connected in the background until the provided
// ctx is canceled or either StopSync or CloseWallet is called.
// TODO: Accept sync ntfn listeners.
func (w *Wallet) StartSync(ctx context.Context, connectPeers ...string) error {
	w.syncMtx.Lock()
	defer w.syncMtx.Unlock()
	if w.syncer != nil && w.syncer.IsActive() {
		return errors.New("wallet is already synchronized to the network")
	}

	addr := &net.TCPAddr{IP: net.ParseIP("::1"), Port: 0}
	amgr := addrmgr.New(w.dir, net.LookupIP)
	lp := p2p.NewLocalPeer(w.ChainParams(), addr, amgr)
	syncer := spv.NewSyncer(w.mainWallet, lp)
	if len(connectPeers) > 0 {
		syncer.SetPersistentPeers(connectPeers)
	}

	ctx, syncHelper := asset.InitSyncHelper(ctx) // below this point, ctx=syncCtx
	w.syncer = &spvSyncer{syncer, syncHelper}
	w.SetNetworkBackend(syncer)

	go func() {
		// Defer a fn to signal that the sync has been fully stopped.
		defer func() {
			w.syncMtx.Lock()
			w.syncer.ShutdownComplete()
			w.syncMtx.Unlock()
			w.SetNetworkBackend(nil)
		}()

		for {
			err := w.syncer.Run(ctx)
			if ctx.Err() != nil {
				return
			}
			w.log.Errorf("SPV synchronization ended. trying again in 10 seconds: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second * 10):
			}
		}
	}()

	return nil
}

// StopSync cancels the wallet's synchronization to the blockchain network. It
// may take a few moments for sync to completely stop, before this method will
// return.
func (w *Wallet) StopSync() error {
	var waitForShutdown func()
	w.syncMtx.Lock()
	if w.syncer != nil && w.syncer.IsActive() {
		w.syncer.Shutdown()
		waitForShutdown = w.syncer.WaitForShutdown
	}
	w.syncMtx.Unlock()

	// Call the waitForShutdown fn outside of the syncMtx lock.
	if waitForShutdown != nil {
		waitForShutdown()
	}
	return nil
}
