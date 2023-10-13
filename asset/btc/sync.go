package btc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcwallet/chain"
	"github.com/itswisdomagain/libwallet/asset"
	"github.com/lightninglabs/neutrino"
)

// btcChainService wraps *neutrino.ChainService in order to translate the
// neutrino.ServerPeer to the SPVPeer interface type.
type btcChainService struct {
	*neutrino.ChainService
}

func (s *btcChainService) Peers() []asset.SPVPeer {
	rawPeers := s.ChainService.Peers()
	peers := make([]asset.SPVPeer, 0, len(rawPeers))
	for _, p := range rawPeers {
		peers = append(peers, p)
	}
	return peers
}

// StartSync connects the wallet to the blockchain network via SPV and returns
// immediately. The wallet stays connected in the background until the provided
// ctx is canceled or either StopSync or CloseWallet is called.
// TODO: Accept sync ntfn listeners.
func (w *Wallet) StartSync(ctx context.Context, connectPeers []string, savedPeersFilePath string) error {
	w.syncMtx.Lock()
	defer w.syncMtx.Unlock()
	if w.syncer != nil && w.syncer.IsActive() {
		return errors.New("wallet is already synchronized to the network")
	}

	w.log.Debug("Starting neutrino chain service...")
	chainService, err := neutrino.NewChainService(neutrino.Config{
		DataDir:       w.dir,
		Database:      w.db,
		ChainParams:   *w.chainParams,
		PersistToDisk: true, // keep cfilter headers on disk for efficient rescanning
		// AddPeers:      addPeers,
		// ConnectPeers:  connectPeers,
		// WARNING: PublishTransaction currently uses the entire duration
		// because if an external bug, but even if the resolved, a typical
		// inv/getdata round trip is ~4 seconds, so we set this so neutrino does
		// not cancel queries too readily.
		BroadcastTimeout: 6 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("couldn't create Neutrino ChainService: %w", err)
	}

	chainClient := chain.NewNeutrinoClient(w.chainParams, chainService)
	if err = chainClient.Start(); err != nil { // lazily starts connmgr
		return fmt.Errorf("couldn't start Neutrino client: %v", err)
	}

	// Chain client is started. Connect peers.
	peerManager := asset.NewSPVPeerManager(&btcChainService{chainService}, connectPeers, savedPeersFilePath, w.log, w.chainParams.DefaultPort)
	peerManager.ConnectToInitialWalletPeers()

	w.log.Info("Synchronizing wallet with network...")
	w.SynchronizeRPC(chainClient)

	// Sync is fully started now. Initialize syncCtx and syncHelper and start a
	// goroutine to monitor when the syncCtx is canceled and then stop the sync.
	ctx, w.syncer = asset.InitSyncHelper(ctx) // below this point, ctx=syncCtx.
	go func() {
		// Wait for the ctx to be canceled.
		<-ctx.Done()
		w.log.Info("Stopping wallet synchronization")

		// Stop the synchronization and notify that sync has ended via the
		// syncEndedCh. Stopping sync happens in 4 steps:

		// 1. Stop the wallet. This is necessary to dissociate the chain client
		// from the wallet.
		w.mainWallet.Stop()
		w.mainWallet.WaitForShutdown()

		// 2. Stop the chain client.
		w.log.Trace("Stopping neutrino chain client")
		chainClient.Stop()
		chainClient.WaitForShutdown()

		// 3. Stop the chain service.
		w.log.Trace("Stopping neutrino chain sync service")
		if err := chainService.Stop(); err != nil {
			w.log.Errorf("error stopping neutrino chain service: %v", err)
		}

		// 4. Restart the wallet. Ensures that wallet features not requiring
		// sync can continue to work.
		w.mainWallet.Start()

		// Finally, signal that the sync has been fully stopped.
		w.syncMtx.Lock()
		w.syncer.ShutdownComplete()
		w.syncMtx.Unlock()
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

// temporarilyDisableSync checks if the wallet is currently connected to a chain
// client, stops the chain client and then dissociate the chain client from the
// wallet. The chain client is restarted and re-associated with the wallet when
// the provided restartSyncTrigger is activated. Any attempts to start or stop
// the wallet sync will block until the restartSyncTrigger is activated, even if
// the wallet wasn't synced or syncing when this method was called.
func (w *Wallet) temporarilyDisableSync(restartSyncTrigger chan struct{}) {
	// Lock the syncMtx to prevent concurrent attempts to start/stop sync until
	// this temporary disabling is complete.
	w.syncMtx.Lock()

	chainClient := w.ChainClient()
	if chainClient != nil {
		w.log.Info("Temporarily stopping wallet and chain client...")
		w.mainWallet.Stop() // stops Wallet and chainClient (not chainService)
		w.mainWallet.WaitForShutdown()
		chainClient.WaitForShutdown()
	}

	// Wait for the restartSyncTrigger in background, then restart the chain
	// client and release the syncMtx lock.
	go func() {
		<-restartSyncTrigger // wait for trigger
		if chainClient != nil {
			w.log.Info("Restarting wallet and chain client...")
			w.mainWallet.Start()
			if err := chainClient.Start(); err != nil {
				w.log.Errorf("couldn't restart Neutrino client: %v", err)
			} else {
				w.mainWallet.SynchronizeRPC(chainClient)
			}
		}
		w.syncMtx.Unlock() // safe to release this lock now
	}()
}
