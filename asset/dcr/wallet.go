package dcr

import (
	"context"
	"fmt"
	"path/filepath"

	"decred.org/dcrwallet/v3/wallet"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/slog"
)

type mainWallet = wallet.Wallet

type Wallet struct {
	dir         string
	dbDriver    string
	chainParams *chaincfg.Params
	log         slog.Logger

	db wallet.DB
	*mainWallet
}

// OpenWallet opens the wallet database and the wallet.
func (w *Wallet) OpenWallet(ctx context.Context) error {
	if w.mainWallet != nil {
		return fmt.Errorf("wallet is already open")
	}

	w.log.Debug("Opening DCR wallet...")
	db, err := wallet.OpenDB(w.dbDriver, filepath.Join(w.dir, walletDbName))
	if err != nil {
		return fmt.Errorf("wallet.OpenDB error: %w", err)
	}

	dcrw, err := wallet.Open(ctx, newWalletConfig(db, w.chainParams))
	if err != nil {
		// If this function does not return to completion the database must be
		// closed.  Otherwise, because the database is locked on open, any
		// other attempts to open the wallet will hang, and there is no way to
		// recover since this db handle would be leaked.
		if err := db.Close(); err != nil {
			w.log.Errorf("Failed to close wallet database after OpenWallet error: %v", err)
		}
		return fmt.Errorf("wallet.Open error: %w", err)
	}

	w.db = db
	w.mainWallet = dcrw

	// var connectPeers []string
	// switch w.chainParams.Net {
	// case wire.SimNet:
	// 	connectPeers = []string{"localhost:19560"}
	// }

	// spv := newSpvSyncer(dcrw, w.dir, connectPeers)
	// w.spv = spv

	// w.wg.Add(2)
	// go func() {
	// 	defer w.wg.Done()
	// 	w.spvLoop(ctx, spv)
	// }()
	// go func() {
	// 	defer w.wg.Done()
	// 	w.notesLoop(ctx, dcrw)
	// }()

	// w.initializeSimnetTspends(ctx)

	return nil
}

// CloseWallet stops any active network syncrhonization and closes the wallet
// database.
func (w *Wallet) CloseWallet() error {
	// TODO: If sync is ongoing, stop the sync first.

	if err := w.db.Close(); err != nil {
		return err
	}

	w.log.Debug("DCR wallet closed")
	w.mainWallet = nil
	w.db = nil
	return nil
}
