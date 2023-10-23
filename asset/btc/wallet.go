package btc

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/asset"
)

var wAddrMgrBkt = []byte("waddrmgr")

type mainWallet = wallet.Wallet

type Wallet struct {
	dir         string
	dbDriver    string
	chainParams *chaincfg.Params
	log         slog.Logger

	loader *wallet.Loader
	db     walletdb.DB
	*asset.SeededWallet
	*mainWallet

	syncMtx sync.Mutex
	syncer  *asset.SyncHelper
}

// OpenWallet opens the wallet database and the wallet.
func (w *Wallet) OpenWallet() error {
	if w.mainWallet != nil {
		return fmt.Errorf("wallet is already open")
	}

	// timeout and recoverWindow arguments borrowed from btcwallet directly.
	w.loader = wallet.NewLoader(w.chainParams, w.dir, true, dbTimeout, 250)

	exists, err := w.loader.WalletExists()
	if err != nil {
		return fmt.Errorf("error verifying wallet existence: %v", err)
	}
	if !exists {
		return errors.New("wallet not found")
	}

	w.log.Debug("Opening BTC wallet...")
	btcw, err := w.loader.OpenExistingWallet([]byte(wallet.InsecurePubPassphrase), false)
	if err != nil {
		return fmt.Errorf("couldn't load wallet: %w", err)
	}

	neutrinoDBPath := filepath.Join(w.dir, neutrinoDBName)
	db, err := walletdb.Open(w.dbDriver, neutrinoDBPath, true, dbTimeout) // TODO: DEX uses Create!
	if err != nil {
		if unloadErr := w.loader.UnloadWallet(); unloadErr != nil {
			w.log.Errorf("Error unloading wallet after OpenWallet error:", unloadErr)
		}
		return fmt.Errorf("unable to open wallet db at %q: %v", neutrinoDBPath, err)
	}

	w.db = db
	w.mainWallet = btcw
	return nil
}

// WalletOpened returns true if the wallet is opened and ready for use.
func (w *Wallet) WalletOpened() bool {
	return w.mainWallet != nil
}

// CloseWallet stops any active network syncrhonization, unloads the wallet and
// closes the neutrino chain db.
func (w *Wallet) CloseWallet() error {
	if err := w.StopSync(); err != nil {
		return fmt.Errorf("StopSync error: %w", err)
	}

	if err := w.loader.UnloadWallet(); err != nil {
		return err
	}

	if err := w.db.Close(); err != nil {
		return err
	}

	w.log.Debug("BTC wallet closed")
	w.mainWallet = nil
	w.db = nil
	return nil
}

// SetBirthday updates the wallet's birthday to the provided value. If the
// newBday is before the current birthday, rescan will be performed. If the
// wallet is currently synced or syncing, the wallet will be disconnected first
func (w *Wallet) SetBirthday(newBday time.Time) error {
	oldBday := w.Manager.Birthday()
	if newBday.Equal(oldBday) {
		return nil // nothing to update
	}

	rescanRequired := newBday.Before(oldBday)
	if rescanRequired {
		// Temporarily disable syncing until this birthday update completes.
		restartSync := make(chan struct{})
		w.temporarilyDisableSync(restartSync)
		defer close(restartSync) // will trigger sync restart after this bday update completes.
	}

	// Update the birthday in the wallet database. If rescan is required, also
	// mark the wallet as needing rescan. The rescan will be performed when the
	// wallet synchronization starts or restarts.
	return walletdb.Update(w.Database(), func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wAddrMgrBkt)
		if err := w.Manager.SetBirthday(ns, newBday); err != nil {
			return err
		}
		if rescanRequired {
			return w.Manager.SetSyncedTo(ns, nil) // never synced, forcing recover from birthday
		}
		return nil
	})
}

// ChangePassphrase changes the wallet's private passphrase. If provided, the
// finalize function would be called after the passphrase change is complete. If
// that function returns an error, the passphrase change will be reverted.
func (w *Wallet) ChangePassphrase(oldPass, newPass []byte, finalize func() error) (err error) {
	err = w.ChangePrivatePassphrase(oldPass, newPass)
	if err != nil {
		return err
	}

	revertPassphraseChange := func() {
		if err = w.ChangePrivatePassphrase(newPass, oldPass); err != nil {
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
