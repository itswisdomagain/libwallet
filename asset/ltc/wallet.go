package ltc

import (
	"fmt"

	neutrino "github.com/dcrlabs/neutrino-ltc"
	"github.com/dcrlabs/neutrino-ltc/chain"
	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/asset"
	"github.com/ltcsuite/ltcwallet/wallet"
	"github.com/ltcsuite/ltcwallet/walletdb"
	_ "github.com/ltcsuite/ltcwallet/walletdb/bdb"
)

var waddrmgrNamespace = []byte("waddrmgr")

type mainWallet = wallet.Wallet

type Wallet struct {
	*asset.WalletBase
	*mainWallet

	dir          string
	dbDriver     string
	log          slog.Logger
	loader       *wallet.Loader
	db           walletdb.DB
	chainService *neutrino.ChainService
	chainClient  *chain.NeutrinoClient
}

// MainWallet returns the main ltc wallet with the core wallet functionalities.
func (w *Wallet) MainWallet() *wallet.Wallet {
	return w.mainWallet
}

// WalletOpened returns true if the main wallet has been opened.
func (w *Wallet) WalletOpened() bool {
	return w.mainWallet != nil
}

// OpenWallet opens the main wallet.
func (w *Wallet) OpenWallet() error {
	if w.mainWallet != nil {
		return fmt.Errorf("wallet is already open")
	}

	w.log.Info("Opening wallet...")
	ltcw, err := w.loader.OpenExistingWallet([]byte(wallet.InsecurePubPassphrase), false)
	if err != nil {
		return fmt.Errorf("OpenExistingWallet error: %w", err)
	}

	w.mainWallet = ltcw
	return nil
}

// CloseWallet stops any active network synchronization and unloads the main
// wallet.
func (w *Wallet) CloseWallet() error {
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
func (w *Wallet) Shutdown() error {
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
