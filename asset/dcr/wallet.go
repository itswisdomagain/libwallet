package dcr

import (
	"context"
	"fmt"
	"path/filepath"

	"decred.org/dcrwallet/v3/errors"
	"decred.org/dcrwallet/v3/wallet"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/asset"
)

type mainWallet = wallet.Wallet

type Wallet struct {
	dir         string
	dbDriver    string
	chainParams *chaincfg.Params
	log         slog.Logger

	db wallet.DB
	*asset.WalletBase
	*mainWallet

	// syncMtx sync.Mutex
	// syncer  *spv.Syncer
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

// MainWallet returns the actual dcr *wallet.Wallet.
func (w *Wallet) MainWallet() *wallet.Wallet {
	return w.mainWallet
}

// CloseWallet stops any active network syncrhonization and closes the wallet
// database.
func (w *Wallet) CloseWallet() error {
	w.StopSync()
	w.WaitForSyncToStop()

	w.log.Info("Closing wallet")
	if err := w.db.Close(); err != nil {
		return fmt.Errorf("Close wallet db error: %w", err)
	}

	w.log.Debug("DCR wallet closed")
	w.mainWallet = nil
	w.db = nil
	return nil
}

// ChangePassphrase changes the wallet's private passphrase. If provided, the
// finalize function would be called after the passphrase change is complete. If
// that function returns an error, the passphrase change will be reverted.
func (w *Wallet) ChangePassphrase(ctx context.Context, oldPass, newPass []byte, finalize func() error) (err error) {
	err = w.ChangePrivatePassphrase(ctx, oldPass, newPass)
	if err != nil {
		if err, ok := err.(*errors.Error); ok && err.Kind == errors.Passphrase {
			return asset.ErrInvalidPassphrase
		}
		return err
	}

	revertPassphraseChange := func() {
		if err = w.ChangePrivatePassphrase(ctx, newPass, oldPass); err != nil {
			w.log.Errorf("failed to undo wallet passphrase change: %w", err)
		}
	}

	if err = w.ReEncryptSeed(oldPass, newPass); err != nil {
		revertPassphraseChange()
		return fmt.Errorf("error re-encrypting wallet seed: %v", err)
	}

	if finalize != nil {
		if err = finalize(); err != nil {
			revertPassphraseChange()
			w.log.Errorf("error finalizing passphrase change: %v", err)
			return err
		}
	}

	return nil
}
