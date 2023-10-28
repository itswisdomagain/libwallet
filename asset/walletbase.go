package asset

import (
	"bytes"
	"fmt"
	"sync"

	"decred.org/dcrwallet/v3/walletseed"
	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/walletdata"
)

const (
	walletTraitsDBKey             = "traits"
	encryptedSeedDBKey            = "encryptedSeed"
	accountDiscoveryRequiredDBKey = "accountDiscoveryRequired"
)

type WalletBase[Tx any] struct {
	// UserConfigDB is publicly embedded, so consumers can directly read from or
	// write to the user config db.
	walletdata.UserConfigDB
	// TxIndexDB is embedded so consumers can call tx indexing methods directly.
	// TxIndexDB may be nil and calling tx indexing methods when this field is
	// nil may panic.
	walletdata.TxIndexDB[Tx]

	// db is private, for internal use only.
	db      walletdata.WalletConfigDB
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
func NewWalletBase[Tx any](params OpenWalletParams[Tx], seed, walletPass []byte, traits WalletTrait) (*WalletBase[Tx], error) {
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

	// Save the initial data to db.
	dbData := map[string]any{
		walletTraitsDBKey:             traits,
		accountDiscoveryRequiredDBKey: accountDiscoveryRequired,
	}
	if len(encryptedSeed) > 0 {
		dbData[encryptedSeedDBKey] = encryptedSeed
	}
	for key, value := range dbData {
		if err := params.WalletConfigDB.SaveWalletConfigValue(key, value); err != nil {
			return nil, fmt.Errorf("error saving wallet.%s to db: %v", key, err)
		}
	}

	return &WalletBase[Tx]{
		UserConfigDB:             params.UserConfigDB,
		TxIndexDB:                params.TxIndexDB,
		db:                       params.WalletConfigDB,
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
func OpenWalletBase[Tx any](params OpenWalletParams[Tx]) (*WalletBase[Tx], error) {
	w := &WalletBase[Tx]{
		UserConfigDB: params.UserConfigDB,
		TxIndexDB:    params.TxIndexDB,
		db:           params.WalletConfigDB,
		log:          params.Logger,
		dataDir:      params.DataDir,
		network:      params.Net,
		syncHelper:   &syncHelper{log: params.Logger},
	}

	readFromDB := func(key string, wFieldPtr any) error {
		if err := params.WalletConfigDB.ReadWalletConfigValue(key, wFieldPtr); err != nil {
			return fmt.Errorf("error reading wallet.%s from db: %v", key, err)
		}
		return nil
	}
	if err := readFromDB(encryptedSeedDBKey, &w.encryptedSeed); err != nil {
		return nil, err
	}
	if err := readFromDB(walletTraitsDBKey, &w.traits); err != nil {
		return nil, err
	}
	if err := readFromDB(accountDiscoveryRequiredDBKey, &w.accountDiscoveryRequired); err != nil {
		return nil, err
	}

	return w, nil
}

func (w *WalletBase[_]) DataDir() string {
	return w.dataDir
}

func (w *WalletBase[_]) Network() Network {
	return w.network
}

// DecryptSeed decrypts the encrypted wallet seed using the provided passphrase.
func (w *WalletBase[_]) DecryptSeed(passphrase []byte) (string, error) {
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

func (w *WalletBase[_]) ReEncryptSeed(oldPass, newPass []byte) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.encryptedSeed == nil {
		return nil
	}

	reEncryptedSeed, err := ReEncryptData(w.encryptedSeed, oldPass, newPass)
	if err != nil {
		return err
	}

	if err = w.db.SaveWalletConfigValue(encryptedSeedDBKey, reEncryptedSeed); err != nil {
		w.log.Errorf("db.SaveWalletConfigValue(encryptedSeed) error: %v", err)
		return fmt.Errorf("database error")
	}

	w.encryptedSeed = reEncryptedSeed
	return nil
}

// SeedVerificationRequired is true if the seed for this wallet is yet to be
// verified by the owner. The wallet's seed is saved in an encrypted form until
// it is verified.
func (w *WalletBase[_]) SeedVerificationRequired() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return len(w.encryptedSeed) > 0
}

// VerifySeed decrypts the encrypted wallet seed using the provided passphrase
// and compares it with the provided seedMnemonic. If it's a match, the wallet
// seed will no longer be saved.
func (w *WalletBase[_]) VerifySeed(seedMnemonic string, passphrase []byte) (bool, error) {
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

	if err = w.db.DeleteWalletConfigValue(encryptedSeedDBKey); err != nil {
		w.log.Errorf("db.DeleteWalletConfigValue(encryptedSeed) error: %v", err)
		return false, fmt.Errorf("database error")
	}

	w.encryptedSeed = nil
	return true, nil
}

func (w *WalletBase[_]) IsWatchOnly() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return isWatchOnly(w.traits)
}

func (w *WalletBase[_]) IsRestored() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return isRestored(w.traits)
}

func (w *WalletBase[_]) AccountDiscoveryRequired() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.accountDiscoveryRequired
}

func (w *WalletBase[_]) MarkAccountDiscoveryComplete() {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	w.accountDiscoveryRequired = false
	if err := w.db.SaveWalletConfigValue(accountDiscoveryRequiredDBKey, w.accountDiscoveryRequired); err != nil {
		w.log.Errorf("error marking wallet discovery complete: %v", err)
	}
}

// ReadUserConfigBoolValue is a helper method for reading a bool user config
// value from the wallet's config db.
func (w *WalletBase[_]) ReadUserConfigBoolValue(key string, defaultValue ...bool) bool {
	return walletdata.ReadUserConfigValue(w, key, defaultValue...)
}

// ReadUserConfigStringValue is a helper method for reading a string user config
// value from the wallet's config db.
func (w *WalletBase[_]) ReadUserConfigStringValue(key string, defaultValue ...string) string {
	return walletdata.ReadUserConfigValue(w, key, defaultValue...)
}
