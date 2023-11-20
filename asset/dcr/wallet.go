package dcr

import (
	"context"
	"fmt"
	"path/filepath"

	"decred.org/dcrwallet/v3/spv"
	"decred.org/dcrwallet/v3/wallet"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/asset"
)

type mainWallet = wallet.Wallet

type Wallet[Tx any] struct {
	*asset.WalletBase[Tx]
	dir         string
	dbDriver    string
	chainParams *chaincfg.Params
	log         slog.Logger

	db wallet.DB
	*mainWallet

	syncer *spv.Syncer
}

// MainWallet returns the main dcr wallet with the core wallet functionalities.
func (w *Wallet[_]) MainWallet() *wallet.Wallet {
	return w.mainWallet
}

// WalletOpened returns true if the main wallet has been opened.
func (w *Wallet[_]) WalletOpened() bool {
	return w.mainWallet != nil
}

// OpenWallet opens the wallet database and the main wallet.
func (w *Wallet[_]) OpenWallet(ctx context.Context) error {
	if w.mainWallet != nil {
		return fmt.Errorf("wallet is already open")
	}

	w.log.Info("Opening wallet...")
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

// CloseWallet stops any active network synchronization and closes the wallet
// database.
func (w *Wallet[_]) CloseWallet() error {
	w.log.Info("Closing wallet")
	w.StopSync()
	w.WaitForSyncToStop()

	w.log.Trace("Closing wallet db")
	if err := w.db.Close(); err != nil {
		return fmt.Errorf("Close wallet db error: %w", err)
	}

	w.log.Info("Wallet closed")
	w.mainWallet = nil
	w.db = nil
	return nil
}

// Shutdown closes the main wallet and any other resources in use.
func (w *Wallet[_]) Shutdown() error {
	return w.CloseWallet()
}

// ChangePassphrase changes the wallet's private passphrase. If the wallet's
// seed has not been backed up, the seed will be re-encrypted using the new
// passphrase.
func (w *Wallet[Tx]) ChangePassphrase(ctx context.Context, oldPass, newPass []byte) (err error) {
	err = w.ChangePrivatePassphrase(ctx, oldPass, newPass)
	if err != nil {
		return err
	}

	if err = w.ReEncryptSeed(oldPass, newPass); err != nil {
		// revert passphrase change
		if err = w.ChangePrivatePassphrase(ctx, newPass, oldPass); err != nil {
			w.log.Errorf("failed to undo wallet passphrase change: %w", err)
		}
		return fmt.Errorf("error re-encrypting wallet seed: %v", err)
	}

	return nil
}
