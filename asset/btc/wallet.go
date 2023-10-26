package btc

import (
	"fmt"
	"time"

	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/asset"
	"github.com/lightninglabs/neutrino"
)

var wAddrMgrBkt = []byte("waddrmgr")

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

// OpenWallet opens the wallet database and the wallet.
func (w *Wallet) OpenWallet() error {
	if w.mainWallet != nil {
		return fmt.Errorf("wallet is already open")
	}

	w.log.Debug("Opening BTC wallet...")
	btcw, err := w.loader.OpenExistingWallet([]byte(wallet.InsecurePubPassphrase), false)
	if err != nil {
		return fmt.Errorf("OpenExistingWallet error: %w", err)
	}

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
	w.StopSync()
	w.WaitForSyncToStop()

	w.log.Info("Unloading wallet")
	if err := w.loader.UnloadWallet(); err != nil {
		return err
	}
	w.mainWallet = nil

	w.log.Trace("Stopping neutrino chain client")
	w.chainClient.Stop()
	w.chainClient.WaitForShutdown()
	w.chainClient = nil

	w.log.Trace("Stopping neutrino chain service")
	if err := w.chainService.Stop(); err != nil {
		w.log.Errorf("error stopping neutrino chain service: %v", err)
	}
	w.chainService = nil

	w.log.Trace("Stopping neutrino DB.")
	if err := w.db.Close(); err != nil {
		return err
	}
	w.db = nil

	w.log.Info("BTC wallet closed")
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

// setSyncedTo manually sets the wallet as synced up to the specified block
// height. This is done because btcwallet sometimes fails to update the synced
// to block and when that happens, wallet transactions may not be found until
// the wallet is rescanned. TODO: Verify this.
//
// NOTE: This update attempt will be ignored if the wallet is NOT already synced
// or wallet rescan is in progress.
func (w *Wallet) setSyncedTo(blockHeight int32) error {
	isRescanning := false // TODO
	if !w.ChainSynced() || isRescanning {
		return nil
	}

	bs, err := w.getBlockStamp(blockHeight)
	if err != nil {
		return err
	}

	return walletdb.Update(w.Database(), func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wAddrMgrBkt)
		return w.Manager.SetSyncedTo(ns, bs)
	})
}

func (w *Wallet) getBlockStamp(blockHeight int32) (*waddrmgr.BlockStamp, error) {
	blockHash, err := w.GetBlockHash(int64(blockHeight))
	if err != nil {
		return nil, fmt.Errorf("GetBlockHash error: %w", err)
	}

	block, err := w.ChainClient().GetBlock(blockHash)
	if err != nil {
		return nil, fmt.Errorf("GetBlock error: %w", err)
	}

	return &waddrmgr.BlockStamp{
		Hash:      block.BlockHash(),
		Height:    blockHeight,
		Timestamp: block.Header.Timestamp,
	}, nil
}
