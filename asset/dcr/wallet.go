package dcr

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

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

	syncMtx sync.Mutex
	syncer  *spvSyncer
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

	return nil
}

// WalletOpened returns true if the wallet is opened and ready for use.
func (w *Wallet) WalletOpened() bool {
	return w.mainWallet != nil
}

// CloseWallet stops any active network syncrhonization and closes the wallet
// database.
func (w *Wallet) CloseWallet() error {
	if err := w.StopSync(); err != nil {
		return fmt.Errorf("StopSync error: %w", err)
	}

	if err := w.db.Close(); err != nil {
		return fmt.Errorf("Close wallet db error: %w", err)
	}

	w.log.Debug("DCR wallet closed")
	w.mainWallet = nil
	w.db = nil
	return nil
}
