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

// StartSync connects the wallet to the blockchain network via SPV and returns
// immediately. The wallet stays connected in the background until the provided
// ctx is canceled or either StopSync or CloseWallet is called.
// TODO: Accept sync ntfn listeners.
func (w *Wallet) StartSync(ctx context.Context, connectPeers []string, savedPeersFilePath string) error {
	// Initialize the ctx to use for sync. Will error if sync was already
	// started.
	ctx, err := w.InitializeSyncContext(ctx)
	if err != nil {
		return err
	}

	w.log.Info("Starting sync...")
	if err = w.chainClient.Start(); err != nil { // lazily starts connmgr
		w.SyncEnded(err)
		return fmt.Errorf("couldn't start Neutrino client: %v", err)
	}

	w.SynchronizeRPC(w.chainClient)

	// Chain client is started. Connect peers.
	peerManager := asset.NewSPVPeerManager(&btcChainService{w.chainService}, connectPeers, savedPeersFilePath, w.log, w.ChainParams().DefaultPort)
	peerManager.ConnectToInitialWalletPeers()

	// Start a goroutine to monitor when the sync ctx is canceled and then
	// disconnect the sync.
	go func() {
		<-ctx.Done()
		w.log.Info("Stopping wallet synchronization")

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
		w.SyncEnded(nil)
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
	return w.ChainSynced()
}
