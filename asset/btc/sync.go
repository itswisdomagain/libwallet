package btc

import (
	"context"
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

func (w *Wallet) ChainClient() *chain.NeutrinoClient {
	return w.chainClient
}

// StartSync connects the wallet to the blockchain network via SPV and returns
// immediately. The wallet stays connected in the background until the provided
// ctx is canceled or either StopSync or CloseWallet is called.
func (w *Wallet) StartSync(ctx context.Context, connectPeers []string, savedPeersFilePath string, reportProgress bool) error {
	bestBlock, err := w.chainService.BestBlock()
	if err != nil {
		return fmt.Errorf("chainService.BestBlock error: %v", err)
	}

	// Initialize the ctx to use for sync. Will error if sync was already
	// started.
	ctx, err = w.InitializeSyncContext(ctx)
	if err != nil {
		return err
	}

	w.log.Debug("Starting sync...")
	if err = w.chainClient.Start(); err != nil { // lazily starts connmgr
		w.SyncEnded(err)
		return fmt.Errorf("couldn't start Neutrino client: %v", err)
	}

	// Subscribe to chainclient notifications.
	if err := w.chainClient.NotifyBlocks(); err != nil {
		w.chainClient.Stop()
		w.SyncEnded(err)
		return fmt.Errorf("subscribing to notifications failed: %v", err)
	}

	// Chain client is started. Connect peers.
	peerManager := asset.NewSPVPeerManager(&btcChainService{w.chainService}, connectPeers, savedPeersFilePath, w.log, w.ChainParams().DefaultPort)
	peerManager.ConnectToInitialWalletPeers()

	w.log.Info("Synchronizing wallet with network...")
	w.SynchronizeRPC(w.chainClient)

	// Sync is fully started now. Start a goroutine to monitor when the syncCtx
	// is canceled and then stop the sync.
	ctx, err = w.InitializeSyncContext(ctx)
	if err != nil {
		w.chainClient.Stop()
		return err
	}

	var syncReporter *asset.SyncProgressReporter
	if reportProgress {
		syncReporter = asset.InitSyncProgressReporter(3*time.Second, w.log)
		syncReporter.SyncStarted(bestBlock.Height, peerManager.BestPeerHeight())
	}

	// Goroutine to monitor sync progress.
	go w.monitorSyncActivity(ctx, syncReporter)

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
		w.chainClient.Stop()
		w.chainClient.WaitForShutdown()

		// 3. Stop the chain service.
		// TODO: Don't stop chain service because it is difficult to restart later?
		// w.log.Trace("Stopping neutrino chain sync service")
		// if err := w.chainService.Stop(); err != nil {
		// 	w.log.Errorf("error stopping neutrino chain service: %v", err)
		// }

		// 4. Restart the wallet. Ensures that wallet features not requiring
		// sync can continue to work.
		w.mainWallet.Start()

		// Finally, signal that the sync has ended without any error.
		w.SyncEnded(nil)
	}()

	return nil
}

// IsSynced returns true if the wallet has synced up to the best block on the
// main chain.
func (w *Wallet) IsSynced() bool {
	return w.ChainSynced()
}

// temporarilyDisableSync checks if the wallet is currently connected to a chain
// client, stops the chain client and then dissociates the chain client from the
// wallet. The chain client is restarted and re-associated with the wallet when
// the provided restartSyncTrigger is activated. Any other attempt to start or
// stop the wallet sync will block until the restartSyncTrigger is activated,
// even if the wallet wasn't synced or syncing when this method was called.
func (w *Wallet) temporarilyDisableSync(restartSyncTrigger chan struct{}) {
	// Prevent other attempts to start/stop sync until the restartSyncTrigger is
	// fired.
	w.BlockSync()

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
		w.UnblockSync() // safe to release this lock now
	}()
}
