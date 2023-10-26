package btc

import (
	"context"
	"fmt"

	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet"
)

func (w *Wallet[Tx]) RescanBlocks(ctx context.Context, startHeight int32) error {
	if !w.IsSynced() {
		return fmt.Errorf("wallet isn't synced")
	}

	bestBlock, err := w.chainService.BestBlock()
	if err != nil {
		return fmt.Errorf("chainService.BestBlock error: %v", err)
	}

	if err := w.InitializeRescan(startHeight, bestBlock.Height); err != nil {
		return err
	}

	job := &wallet.RescanJob{
		BlockStamp: waddrmgr.BlockStamp{
			Height: 0,
			Hash:   *w.ChainParams().GenesisHash,
		},
	}

	errChan := w.SubmitRescan(job)
	go func() {
		err := <-errChan
		if err == nil {
			err = w.indexTransactions(ctx, &startHeight, bestBlock.Height)
		}
		println("ended")
		w.RescanEnded(err)
	}()

	return nil
}
