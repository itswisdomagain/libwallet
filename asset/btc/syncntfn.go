package btc

import (
	"context"
	"time"

	"github.com/btcsuite/btcwallet/chain"
	"github.com/itswisdomagain/libwallet/asset"
)

// monitorSyncActivity listens to sync activity updates from the chain client
// and updates the wallet synchronization state, as well as calculating a sync
// progress report if the wallet is still catching up to the network.
func (w *Wallet) monitorSyncActivity(ctx context.Context, syncReporter *asset.SyncProgressReporter) {
	select {
	// Wait briefly to allow btcwallet's handleChainNotifications() goroutine
	// to start first, before we set up our own chain ntfn handler below. This
	// 5 seconds delay is arbitrarily chosen, and if found inadequate in future,
	// it could be increased.
	case <-time.After(time.Second * 5):
	case <-ctx.Done():
		return
	}

	if syncReporter != nil {
		w.handleAllSyncNtfnUpdates(ctx, syncReporter)
	} else {
		w.handleBlockConnectedSyncNtfnUpdates(ctx)
	}

}

func (w *Wallet) handleAllSyncNtfnUpdates(ctx context.Context, syncReporter *asset.SyncProgressReporter) {
	for {
		select {
		case n, ok := <-w.ChainClient().Notifications():
			if !ok {
				continue
			}

			switch n := n.(type) {
			case chain.BlockConnected:
				w.log.Infof("BlockConnected %d", n.Height)
				w.setSyncedTo(n.Height)
				syncReporter.HandleBlockConnected(n.Height, nil)

			case chain.BlockDisconnected:
				// TODO: Handle reorg!

			case chain.FilteredBlockConnected:
				// TODO: Duplicate of chain.BlockConnected??
				w.log.Infof("BlockConnected %d", n.Block.Height)
				w.setSyncedTo(n.Block.Height)
				syncReporter.HandleBlockConnected(n.Block.Height, n.RelevantTxs)

			case *chain.RescanProgress:
				// Notifications sent at interval of 10k blocks
				w.log.Infof("rescan progress %d", n.Height)

			case *chain.RescanFinished:
				w.setSyncedTo(n.Height)
				syncReporter.HandleBlockConnected(n.Height, nil)
				syncReporter.SyncCompleted()

				// Only run the listener once the chain is synced and ready to listen
				// for newly mined block. This prevents unnecessary CPU use spikes
				// on startup when a wallet is syncing from scratch.
				// go asset.listenForTransactions()

				// Since the initial run on a restored wallet, address discovery
				// is complete, mark discovered accounts as true.
				// if asset.IsRestored && !asset.ContainsDiscoveredAccounts() {
				// 	// Update the assets birthday from genesis block to a date closer
				// 	// to when the privatekey was first used.
				// 	asset.updateAssetBirthday()
				// 	asset.MarkWalletAsDiscoveredAccounts()
				// }

				// asset.syncData.mu.Lock()
				// asset.syncData.isRescan = false
				// asset.syncData.mu.Unlock()

				// if asset.blocksRescanProgressListener != nil {
				// 	asset.blocksRescanProgressListener.OnBlocksRescanEnded(asset.ID, nil)
				// }
			}

		case <-ctx.Done():
			return
		}
	}
}

// handleBlockConnectedSyncNtfnUpdates listens to chain.BlockConnected ntfns
// from the chain client and updates the wallet's SyncedTo height. Ignores all
// other ntfns from the chain client.
func (w *Wallet) handleBlockConnectedSyncNtfnUpdates(ctx context.Context) {
	for {
		select {
		case n, ok := <-w.ChainClient().Notifications():
			if !ok {
				continue
			}

			if n, ok := n.(chain.BlockConnected); ok {
				w.setSyncedTo(n.Height)
			}

		case <-ctx.Done():
			return
		}
	}
}
