package btc

import (
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
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
	chainClient  *maturedChainClient
}

type maturedChainClient struct {
	*chain.NeutrinoClient

	mtx                sync.Mutex
	ntfnHandlerStarted bool
	defaultNtfnChan    chan interface{}
	otherNtfnChans     map[string]chan interface{}
}

func newMaturedChainClient(chainParams *chaincfg.Params, chainService *neutrino.ChainService) *maturedChainClient {
	return &maturedChainClient{
		NeutrinoClient: chain.NewNeutrinoClient(chainParams, chainService),
	}
}

func (mcl *maturedChainClient) Start() error {
	if err := mcl.NeutrinoClient.Start(); err != nil {
		return err
	}

	mcl.mtx.Lock()
	defer mcl.mtx.Unlock()

	if !mcl.ntfnHandlerStarted {
		mcl.defaultNtfnChan = make(chan interface{})
		mcl.otherNtfnChans = make(map[string]chan interface{})
		go mcl.notificationHandler()
		mcl.ntfnHandlerStarted = true
	}

	return nil
}

// Notifications replicates the RPC client's Notifications method.
func (mcl *maturedChainClient) Notifications() <-chan interface{} {
	mcl.mtx.Lock()
	defer mcl.mtx.Unlock()
	return mcl.defaultNtfnChan
}

// Notifications replicates the RPC client's Notifications method.
func (mcl *maturedChainClient) NotificationWithID(id string) <-chan interface{} {
	mcl.mtx.Lock()
	defer mcl.mtx.Unlock()

	if _, exists := mcl.otherNtfnChans[id]; exists {
		panic("multiple attempts to use a ntfn channel with id: " + id)
	}

	mcl.otherNtfnChans[id] = make(chan interface{})
	return mcl.otherNtfnChans[id]
}

func (mcl *maturedChainClient) notificationHandler() {
	var pendingNtfn interface{}
	var pendingNtfns []interface{}
	var pendingNtfnCh chan interface{}
	var pendingResumed time.Time

	for {
		select {
		case n, ok := <-mcl.NeutrinoClient.Notifications():
			if !ok {
				// Upstream channel closed, close the sub channels. Any pending
				// ntfns to the defaultNtfnChan remain unsent. Same behaviour as
				// when the wallet uses NeutrinoClient.Notifications() directly;
				// pending ntfns are discarded when the chain client is stopped.
				mcl.mtx.Lock()
				close(mcl.defaultNtfnChan)
				for _, ch := range mcl.otherNtfnChans {
					close(ch)
				}
				mcl.ntfnHandlerStarted = false
				mcl.mtx.Unlock()
				return
			}

			// Re-broadcast ntfn from upstream channel to sub channels.
			mcl.mtx.Lock()
			// The defaultNtfnChan used by btc wallet blocks when it receives
			// the initial ClientConnected ntfn and does not receive newer ntfns
			// immediately. If this happens, queue this ntfn to send later.
			select {
			case mcl.defaultNtfnChan <- n:
			default:
				if pendingNtfn == nil {
					pendingNtfn = n
					pendingNtfnCh = mcl.defaultNtfnChan
				}
				pendingNtfns = append(pendingNtfns, n)
			}

			for _, ch := range mcl.otherNtfnChans {
				ch <- n
			}
			mcl.mtx.Unlock()

		case pendingNtfnCh <- pendingNtfn:
			if pendingResumed.IsZero() {
				pendingResumed = time.Now()
				fmt.Printf("resuming send of %d queued ntfns\n", len(pendingNtfns))
			}
			pendingNtfns[0] = nil
			pendingNtfns = pendingNtfns[1:]
			if len(pendingNtfns) > 0 {
				pendingNtfn = pendingNtfns[0]
			} else {
				println("done with pendings in " + time.Since(pendingResumed).String())
				pendingNtfn, pendingNtfnCh, pendingResumed = nil, nil, time.Time{}
			}
		}
	}
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

// SetBirthday updates the wallet's birthday to the provided value. If the
// newBday is before the current birthday, rescan will be performed. If the
// wallet is currently synced or syncing, the wallet will be disconnected first.
// TODO: Test.
func (w *Wallet[Tx]) SetBirthday(newBday time.Time) error {
	oldBday := w.Manager.Birthday()
	if newBday.Equal(oldBday) {
		return nil // nothing to update
	}

	rescanRequired := newBday.Before(oldBday)
	if rescanRequired {
		// Temporarily disable syncing until this birthday update completes.
		restartSyncFn := w.temporarilyDisableSync()
		// Restart sync after this bday update completes.
		defer restartSyncFn()
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

// ChangePassphrase changes the wallet's private passphrase. If the wallet's
// seed has not been backed up, the seed will be re-encrypted using the new
// passphrase.
func (w *Wallet[Tx]) ChangePassphrase(oldPass, newPass []byte) (err error) {
	err = w.ChangePrivatePassphrase(oldPass, newPass)
	if err != nil {
		return err
	}

	if err = w.ReEncryptSeed(oldPass, newPass); err != nil {
		// revert passphrase change
		if err = w.ChangePrivatePassphrase(newPass, oldPass); err != nil {
			w.log.Errorf("failed to undo wallet passphrase change: %w", err)
		}
		return fmt.Errorf("error re-encrypting wallet seed: %v", err)
	}

	return nil
}
