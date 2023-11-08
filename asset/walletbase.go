package asset

import (
	"bytes"
	"fmt"
	"sync"

	"decred.org/dcrwallet/v3/walletseed"
	"github.com/decred/slog"
)

type WalletBase struct {
	log     slog.Logger
	dataDir string
	network Network

	mtx                      sync.Mutex
	traits                   WalletTrait
	encryptedSeed            []byte
	accountDiscoveryRequired bool

	*syncHelper
}

// NewWalletBase initializes a WalletBase using the information provided. The
// wallet's seed is encrypted and saved, along with other basic wallet info.
func NewWalletBase(params OpenWalletParams, seed, walletPass []byte, traits WalletTrait) (*WalletBase, error) {
	isWatchOnly, isRestored := isWatchOnly(traits), isRestored(traits)
	if isWatchOnly && isRestored {
		return nil, fmt.Errorf("invalid wallet traits: restored wallet cannot be watch only")
	}

	hasSeedAndWalletPass := len(seed) > 0 || len(walletPass) > 0

	switch {
	case isWatchOnly && hasSeedAndWalletPass:
		return nil, fmt.Errorf("invalid arguments for watch only wallet")
	case !isWatchOnly && !hasSeedAndWalletPass:
		return nil, fmt.Errorf("seed AND private passphrase are required")
	}

	var encryptedSeed []byte
	var err error
	if !isWatchOnly {
		encryptedSeed, err = EncryptData(seed, walletPass)
		if err != nil {
			return nil, fmt.Errorf("seed encryption error: %v", err)
		}
	}

	// Account discovery is only required for restored wallets.
	accountDiscoveryRequired := isRestored

	// TODO: Save wallet data to db.

	return &WalletBase{
		log:                      params.Logger,
		dataDir:                  params.DataDir,
		network:                  params.Net,
		traits:                   traits,
		encryptedSeed:            encryptedSeed,
		accountDiscoveryRequired: accountDiscoveryRequired,
		syncHelper:               &syncHelper{log: params.Logger},
	}, nil
}

// OpenWalletBase loads basic information for an existing wallet from the
// provided params.
func OpenWalletBase(params OpenWalletParams) (*WalletBase, error) {
	w := &WalletBase{
		log:        params.Logger,
		dataDir:    params.DataDir,
		network:    params.Net,
		syncHelper: &syncHelper{log: params.Logger},
	}

	// TODO: Read wallet data from db.

	return w, nil
}

func (w *WalletBase) DataDir() string {
	return w.dataDir
}

func (w *WalletBase) Network() Network {
	return w.network
}

// DecryptSeed decrypts the encrypted wallet seed using the provided passphrase.
func (w *WalletBase) DecryptSeed(passphrase []byte) (string, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.encryptedSeed == nil {
		return "", fmt.Errorf("seed has been verified")
	}

	seed, err := DecryptData(w.encryptedSeed, passphrase)
	if err != nil {
		return "", err
	}

	return walletseed.EncodeMnemonic(seed), nil
}

func (w *WalletBase) ReEncryptSeed(oldPass, newPass []byte) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.encryptedSeed == nil {
		return nil
	}

	reEncryptedSeed, err := ReEncryptData(w.encryptedSeed, oldPass, newPass)
	if err != nil {
		return err
	}

	// TODO: Save reEncryptedSeed to db.

	w.encryptedSeed = reEncryptedSeed
	return nil
}

// SeedVerificationRequired is true if the seed for this wallet is yet to be
// verified by the owner. The wallet's seed is saved in an encrypted form until
// it is verified.
func (w *WalletBase) SeedVerificationRequired() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return len(w.encryptedSeed) > 0
}

// VerifySeed decrypts the encrypted wallet seed using the provided passphrase
// and compares it with the provided seedMnemonic. If it's a match, the wallet
// seed will no longer be saved.
func (w *WalletBase) VerifySeed(seedMnemonic string, passphrase []byte) (bool, error) {
	seedToCompare, err := walletseed.DecodeUserInput(seedMnemonic)
	if err != nil {
		return false, fmt.Errorf("invalid seed provided")
	}

	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.encryptedSeed == nil {
		return false, fmt.Errorf("seed has been verified")
	}

	seed, err := DecryptData(w.encryptedSeed, passphrase)
	if err != nil {
		return false, err
	}

	if !bytes.Equal(seed, seedToCompare) {
		return false, fmt.Errorf("incorrect seed provided")
	}

	// TODO: Delete encryptedSeed from db.

	w.encryptedSeed = nil
	return true, nil
}

func (w *WalletBase) IsWatchOnly() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return isWatchOnly(w.traits)
}

func (w *WalletBase) IsRestored() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return isRestored(w.traits)
}

func (w *WalletBase) AccountDiscoveryRequired() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.accountDiscoveryRequired
}

func (w *WalletBase) MarkAccountDiscoveryComplete() {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	w.accountDiscoveryRequired = false
	// TODO: Update accountDiscoveryRequired value in db.
}
