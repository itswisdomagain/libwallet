package btc

import (
	"context"
	"fmt"

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

// ConnectedPeersCount returns the number of peers this wallet is currently
// connected to.
func (w *Wallet[Tx]) ConnectedPeersCount() int {
	if !w.IsConnectedToNetwork() {
		return 0
	}
	return len(w.chainService.Peers())
}

// ChainClient overrides the main wallet's ChainClient method to return the
// neutrino chain client that is initialized when the wallet is loaded. The
// original method returns a nil value if sync has not been started for the
// wallet.
func (w *Wallet[Tx]) ChainClient() *maturedChainClient {
	return w.chainClient
}

// StartSync connects the wallet to the blockchain network via SPV and returns
// immediately. The wallet stays connected in the background until the provided
// ctx is canceled or either StopSync or CloseWallet is called.
func (w *Wallet[_]) StartSync(ctx context.Context, connectPeers []string, savedPeersFilePath string) error {
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

	w.log.Infof("Starting sync. Start height %d (%s)", bestBlock.Height, bestBlock.Hash)
	if err = w.chainClient.Start(); err != nil { // lazily starts connmgr
		w.SyncHasStopped(err)
		return fmt.Errorf("couldn't start Neutrino client: %v", err)
	}

	// Subscribe to chain client notifications.
	if err = w.chainClient.NotifyBlocks(); err != nil {
		w.SyncHasStopped(err)
		w.chainClient.Stop()
		return fmt.Errorf("subscribing to chain notifications failed: %v", err)
	}

	w.SynchronizeRPC(w.chainClient)

	// Chain client is started. Connect peers.
	peerManager := asset.NewSPVPeerManager(&btcChainService{w.chainService}, connectPeers, savedPeersFilePath, w.log, w.ChainParams().DefaultPort)
	peerManager.ConnectToInitialWalletPeers()

	// Monitor sync activity in background.
	syncMonitorWait := make(chan bool)
	go func() {
		w.monitorSyncActivity(ctx, bestBlock.Height, peerManager)
		close(syncMonitorWait)
	}()

	// Start a goroutine to monitor when the sync ctx is canceled and then
	// disconnect the sync.
	go func() {
		<-ctx.Done()
		w.log.Info("Stopping wallet synchronization")

		w.log.Info("Waiting for sync activity monitor to conclude")
		<-syncMonitorWait

		// Stop the synchronization and notify that sync has ended via the
		// SyncEnded() method. Stopping sync happens in 4 steps:

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
		w.SyncHasStopped(nil)
	}()

	return nil
}

// IsSyncing returns true if the wallet is catching up to the mainchain's best
// block. Returns false if the wallet is synced up to the best block on the main
// chain.
func (w *Wallet[_]) IsSyncing() bool {
	return w.IsConnectedToNetwork() && !w.ChainSynced()
}

// IsSynced returns true if the wallet has synced up to the best block on the
// mainchain.
func (w *Wallet[_]) IsSynced() bool {
	// Just w.ChainSynced() is not enough as that can return true even if the
	// wallet has been disconnected from the network.
	return w.IsConnectedToNetwork() && w.ChainSynced()
}

// temporarilyDisableSync temporarily stops the wallet's synchronization to the
// network and returns a restartSync function that can be used to reconnect the
// wallet to the blockchain network.  Any other attempt to start or stop the
// wallet sync will block until the returned function is called, even if the
// wallet wasn't synced or syncing when this method was called.
func (w *Wallet[Tx]) temporarilyDisableSync() func() {
	unblockSyncFn := w.BlockSyncAccess() // temporarily disable start/stop sync functionality.

	// Stop the chain client and dissociate it from the wallet.
	chainClient := w.ChainClient()
	if chainClient != nil {
		w.log.Info("Temporarily stopping wallet synchronization")
		w.mainWallet.Stop() // stops Wallet and chainClient (not chainService)
		w.mainWallet.WaitForShutdown()
		chainClient.WaitForShutdown()
	}

	// Return a function to restart the sync and re-enable start/stop sync
	// functionality.
	return func() {
		if chainClient != nil {
			w.log.Info("Restarting wallet and chain client...")
			w.mainWallet.Start()
			if err := chainClient.Start(); err != nil {
				w.log.Errorf("couldn't restart Neutrino client: %v", err)
			} else {
				w.mainWallet.SynchronizeRPC(chainClient)
			}
		}
		unblockSyncFn()
	}
}
