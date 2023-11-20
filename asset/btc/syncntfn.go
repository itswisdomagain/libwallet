package btc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/asset"
	"github.com/itswisdomagain/libwallet/syncutils"
)

type intermittentLogger struct {
	ticker *time.Ticker
	log    slog.Logger
}

func newIntermittentLogger(freqeuncy time.Duration, log slog.Logger) *intermittentLogger {
	return &intermittentLogger{
		ticker: time.NewTicker(freqeuncy),
		log:    log,
	}
}

func (logger *intermittentLogger) Infof(format string, params ...any) {
	if logger.ticker != nil {
		select {
		case <-logger.ticker.C:
		default:
			return
		}
	}

	logger.log.Infof(format, params...)
}

func (logger *intermittentLogger) Disable() {
	logger.ticker.Stop()
	logger.ticker = nil
}

// monitorSyncActivity listens to sync activity updates from the chain client
// and sends sync progress reports as well as rescan progress reports and tx and
// block updates to registered listeners. This method blocks until the chain
// client is stopped.
func (w *Wallet[Tx]) monitorSyncActivity(ctx context.Context, currentTip int32, peerManager *asset.SPVPeerManager) {
	// Initialize a syncReporter to calculate and report sync progress to
	// listeners.
	syncReporter := syncutils.NewSyncProgressReporter(w, w.log)
	syncReporter.SyncStarted(currentTip, peerManager.BestPeerHeight())

	// Only log sync progress reports once every 3 seconds until sync is
	// complete. TODO: Is this logging necessary?? If yes, update code and
	// comment to reflect that it is only used during sync catch up.
	logTicker := time.NewTicker(3 * time.Second)
	checkLogTicker := func() bool {
		select {
		case <-logTicker.C:
			return true
		default:
			return false
		}
	}
	defer logTicker.Stop()

	// Ensure sub goroutines complete before this method is exited. Increment wg
	// when starting a goroutine below and decrement when the goroutine ends.
	var wg sync.WaitGroup
	defer func() {
		wg.Wait()
	}()

	ch := w.chainClient.NotificationWithID("temp")
	for {
		select {
		case n, ok := <-ch:
			if !ok {
				w.log.Infof("Chain client stopped")
				return
			}

			switch n := n.(type) {
			// case chain.BlockConnected is deprecated. FilteredBlockConnected
			// is used instead.
			case chain.FilteredBlockConnected:
				// We only care about this when the wallet is still playing
				// catchup to the network. Once the catchup is complete, new
				// blocks ntfn are processed in the monitorTxAndBlockUpdates
				// goroutine.
				if !w.ChainSynced() {
					targetHeight := peerManager.BestPeerHeight()
					logProgress := checkLogTicker()
					syncReporter.HandleBlockConnected(n.Block.Height, targetHeight, n.RelevantTxs, logProgress)
				}

			case chain.BlockDisconnected:
				// TODO: chain.BlockDisconnected is deprecated in neutrino pkg
				// but wallet doesn't yet support FilteredBlockDisconnected.
				// TODO: Handle reorg! Basically rollback the tx index last
				// block height to the re-org height. The txs will be reindexed
				// if they are reported in new block ntfns.
				targetHeight := peerManager.BestPeerHeight()
				w.log.Infof("BlockDisconnected %v, target is %v", n.Height, targetHeight)

			case *chain.RescanProgress:
				// Only received during manual rescan. Not part of initial
				// wallet sync.
				logProgress := checkLogTicker()
				w.PublishRescanProgress(n.Height, logProgress)

			case *chain.RescanFinished:
				isre := w.IsRescanning()
				w.log.Infof("RescanFinished, IsRescanning: %v", isre)
				if isre {
					continue
				}

				// The main wallet may get this RescanFinished ntfn a little
				// later. Ensure the wallet processes the ntfn first before
				// proceeding here.
				if !w.ChainSynced() {
					w.log.Info("Waiting for wallet to consider itself synced")
					w.pollWalletChainSynced(ctx)
				}

				bestBlockHeight := peerManager.BestPeerHeight()
				if bestBlock, err := w.chainService.BestBlock(); err == nil && bestBlock.Height > bestBlockHeight {
					bestBlockHeight = bestBlock.Height
				}

				// Sync or rescan has completed, index transactions before
				// notifying listeners of sync/rescan completion.
				if err := w.indexTransactions(ctx, nil, bestBlockHeight); err != nil {
					w.log.Errorf("unable to index transactions: %v", err)
				}

				// Not a manual rescan = wallet start up sync. Report that the
				// sync has completed.
				syncReporter.HandleBlockConnected(n.Height, bestBlockHeight, nil, false) // don't log a report here
				syncReporter.HandleSyncCompleted()

				// Monitor tx and block updates now to get notified of new txs
				// and blocks after the wallet has processed them.
				wg.Add(1)
				go func() {
					w.monitorTxAndBlockUpdates(ctx)
					wg.Done()
				}()

				// Address discovery is now complete for this wallet, if it
				// wasn't previously completed.
				if w.AccountDiscoveryRequired() {
					w.MarkAccountDiscoveryComplete()
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

// pollWalletChainSynced calls the wallet's ChainSynced() method periodically
// until it returns true or the provided context is canceled.
func (w *Wallet[Tx]) pollWalletChainSynced(ctx context.Context) {
	checkSyncTicker := time.NewTicker(100 * time.Millisecond)
	defer checkSyncTicker.Stop()

	for {
		select {
		case <-checkSyncTicker.C:
			if w.ChainSynced() {
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

func (w *Wallet[Tx]) indexTransactions(ctx context.Context, startHeight *int32, bestBlockHeight int32) error {
	if !w.CanTransformTx() || w.TxIndexDB == nil {
		w.log.Debug("tx indexing not configured") // not an error
		return nil
	}

	lastIndexedHeight, err := w.TxIndexLastBlock()
	if err != nil {
		return err
	}

	if startHeight != nil && *startHeight < lastIndexedHeight {
		w.log.Infof("Re-indexing transactions from block %d to %d", *startHeight, bestBlockHeight)
		if err := w.RollbackTxIndexLastBlock(*startHeight); err != nil {
			return err
		}
		lastIndexedHeight = *startHeight
	} else {
		w.log.Infof("Indexing transactions from block %d to %d", lastIndexedHeight, bestBlockHeight)
	}

	startBlock := wallet.NewBlockIdentifierFromHeight(lastIndexedHeight)
	endBlock := wallet.NewBlockIdentifierFromHeight(bestBlockHeight)
	getTxResult, err := w.GetTransactions(startBlock, endBlock, "", ctx.Done())
	if err != nil {
		return err
	}

	var succeeded, failed int

	for _, tx := range getTxResult.UnminedTransactions {
		if _, err = w.transformAndIndexTx(-1, tx); err != nil {
			failed++
			w.log.Error(err)
		} else {
			succeeded++
		}
	}

	for _, block := range getTxResult.MinedTransactions {
		for _, tx := range block.Transactions {
			if _, err = w.transformAndIndexTx(block.Height, tx); err != nil {
				failed++
				w.log.Error(err)
			} else {
				succeeded++
			}
		}
	}

	if err = w.SaveTxIndexLastBlock(bestBlockHeight); err != nil {
		w.log.Errorf("SaveTxIndexLastBlock error: %v", err)
	}

	if totalTxCount, err := w.CountTransactions(); err == nil {
		w.log.Infof("Transaction indexing complete. Indexed %d transactions, %d failed. %d txs total.",
			succeeded, failed, totalTxCount)
	} else {
		w.log.Infof("Transaction indexing complete. Indexed %d transactions, %d failed.", succeeded, failed)
	}

	return nil
}

func (w *Wallet[Tx]) monitorTxAndBlockUpdates(ctx context.Context) {
	if !w.CanTransformTx() {
		w.log.Debug("not monitoring transaction and block updates") // not an error
		return
	}

	ntfns := w.NtfnServer.TransactionNotifications()
	defer ntfns.Done()

	w.log.Infof("starting tx block ntfn handler")

	for {
		select {
		case n, ok := <-ntfns.C:
			if !ok || ctx.Err() != nil {
				return
			}

			// We get a ton of these after rescanning. Why?
			if w.IsRescanning() {
				println("ignoring tx ntfn when rescanning")
				continue
			}

			unminedTxs := make([]*Tx, 0, len(n.UnminedTransactions))
			for _, txSummary := range n.UnminedTransactions {
				w.log.Infof("Incoming unmined tx %s", txSummary.Hash)
				tx, err := w.transformAndIndexTx(-1, txSummary)
				if err == nil {
					unminedTxs = append(unminedTxs, tx)
				} else {
					w.log.Error(err)
				}
			}

			var bestHeight, minedTxs int32
			blks := make([]*syncutils.BlockWithTxs[Tx], len(n.AttachedBlocks))
			for _, block := range n.AttachedBlocks {
				blk := &syncutils.BlockWithTxs[Tx]{
					Height: block.Height,
					Hash:   block.Hash.String(),
				}
				for _, txSummary := range block.Transactions {
					minedTxs++
					w.log.Infof("Incoming mined tx %s in block %d", txSummary.Hash, block.Height)
					if tx, err := w.transformAndIndexTx(block.Height, txSummary); err == nil {
						blk.Txs = append(blk.Txs, tx)
					} else {
						w.log.Error(err)
					}
				}
				blks = append(blks, blk)
				bestHeight = blk.Height

				// TODO: Should probably call w.SaveTxIndexLastBlock(blk.Height)
				// but that would be problematic if we missed any block ntfn
				// before this one. Also, some block txs may not have been
				// indexed in the block.Transactions loop above.
			}

			w.log.Infof("tx/block ntfn: %d unmined txs, %d blocks with %d txs, new best %d",
				len(n.UnminedTransactions), len(n.AttachedBlocks), minedTxs, bestHeight)

			w.NotifyTxAndBlockNtfnListeners(func(listener syncutils.TxAndBlockNtfnListener[Tx]) {
				listener.OnTxOrBlockUpdate(unminedTxs, blks)
			})

		case <-ctx.Done():
			return
		}
	}
}

func (w *Wallet[Tx]) transformAndIndexTx(blockHeight int32, txSummary wallet.TransactionSummary) (*Tx, error) {
	tx, err := w.TransformTx(blockHeight, txSummary)
	if err != nil {
		return nil, fmt.Errorf("Error parsing tx %s: %v", txSummary.Hash, err)
	}
	if updated, err := w.IndexTransaction(tx); err != nil {
		return nil, fmt.Errorf("Error indexing tx %s: %v", txSummary.Hash, err)
	} else if updated {
		w.log.Infof("Updated tx %s in tx index database", txSummary.Hash)
	} else {
		w.log.Infof("Added tx %s to tx index database", txSummary.Hash)
	}

	return tx, nil
}
