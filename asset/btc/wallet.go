package btc

import (
	"fmt"

	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/asset"
	"github.com/lightninglabs/neutrino"
)

var wAddrMgrBkt = []byte("waddrmgr")

type mainWallet = wallet.Wallet

type Wallet[Tx any] struct {
	*asset.WalletBase[Tx]
	*mainWallet

	dir          string
	dbDriver     string
	log          slog.Logger
	loader       *wallet.Loader
	db           walletdb.DB
	chainService *neutrino.ChainService
	chainClient  *chain.NeutrinoClient
}

// MainWallet returns the main btc wallet with the core wallet functionalities.
func (w *Wallet[_]) MainWallet() *wallet.Wallet {
	return w.mainWallet
}

// WalletOpened returns true if the main wallet has been opened.
func (w *Wallet[_]) WalletOpened() bool {
	return w.mainWallet != nil
}

// OpenWallet opens the main wallet.
func (w *Wallet[_]) OpenWallet() error {
	if w.mainWallet != nil {
		return fmt.Errorf("wallet is already open")
	}

	w.log.Debug("Opening wallet...")
	btcw, err := w.loader.OpenExistingWallet([]byte(wallet.InsecurePubPassphrase), false)
	if err != nil {
		return fmt.Errorf("OpenExistingWallet error: %w", err)
	}

	w.mainWallet = btcw
	return nil
}

// CloseWallet stops any active network synchronization and unloads the main
// wallet.
func (w *Wallet[_]) CloseWallet() error {
	w.log.Info("Closing wallet")
	w.StopSync()
	w.WaitForSyncToStop()

	w.log.Trace("Unloading wallet")
	if err := w.loader.UnloadWallet(); err != nil {
		return err
	}

	w.log.Info("Wallet closed")
	w.mainWallet = nil
	return nil
}

// Shutdown closes the main wallet and any other resources in use.
func (w *Wallet[_]) Shutdown() error {
	if err := w.CloseWallet(); err != nil {
		return err
	}

	// Closing the wallet does not stop the neutrino chain service, stop it now
	// before closing the neutrino DB.
	w.log.Trace("Stopping neutrino chain service")
	if err := w.chainService.Stop(); err != nil {
		w.log.Errorf("error stopping neutrino chain service: %v", err)
	}

	w.log.Trace("Closing neutrino db")
	if err := w.db.Close(); err != nil {
		w.log.Errorf("error closing neutrino db: %v", err)
	}

	w.log.Info("Wallet shutdown complete")
	w.chainClient = nil
	w.chainService = nil
	w.db = nil
	return nil
}
