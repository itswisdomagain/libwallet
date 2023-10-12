package btc

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
	"github.com/decred/slog"
)

type mainWallet = wallet.Wallet

type Wallet struct {
	dir         string
	dbDriver    string
	chainParams *chaincfg.Params
	log         slog.Logger

	loader *wallet.Loader
	db     walletdb.DB
	*mainWallet
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

// CloseWallet stops any active network syncrhonization, unloads the wallet and
// closes the neutrino chain db.
func (w *Wallet) CloseWallet() error {
	// TODO: If sync is ongoing, stop the sync first.

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
