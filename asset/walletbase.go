package asset

import (
	"bytes"
	"fmt"
	"sync"

	"decred.org/dcrwallet/v3/walletseed"
	"github.com/asdine/storm"
	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/walletdata"
)

const (
	encryptedSeedDBKey            = "encryptedSeed"
	walletTraitsDBKey             = "walletTraits"
	accountDiscoveryRequiredDBKey = "accountDiscoveryRequired"
)

type WalletBase struct {
	// UserConfigDB is publicly embedded, so consumers can read from/write to
	// the user config part of the walletdata db.
	walletdata.UserConfigDB

	// db is private, for internal use only.
	db      walletdata.WalletConfigDB
	log     slog.Logger
	dataDir string
	network Network

	mtx                      sync.Mutex
	encryptedSeed            []byte
	traits                   WalletTrait
	accountDiscoveryRequired bool

	*syncHelper
	*SyncProgressReporter
}

// CreateWalletBase initializes a WalletBase using the information provided. The
// wallet's seed is encrypted and saved, along with other basic wallet info.
func CreateWalletBase(params OpenWalletParams, seed, walletPass []byte, traits WalletTrait) (*WalletBase, error) {
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

	// Account discovery is only required for restored wallets.
	accountDiscoveryRequired := isRestored

	var encryptedSeed []byte
	var err error
	if !isWatchOnly {
		encryptedSeed, err = EncryptData(seed, walletPass)
		if err != nil {
			return nil, fmt.Errorf("seed encryption error: %v", err)
		}
	}

	// Save wallet data to db.
	dbData := map[string]any{
		walletTraitsDBKey:             traits,
		accountDiscoveryRequiredDBKey: accountDiscoveryRequired,
	}
	if len(encryptedSeed) > 0 {
		dbData[encryptedSeedDBKey] = encryptedSeed
	}
	for key, value := range dbData {
		if err := params.ConfigDB.SaveWalletConfigValue(key, value); err != nil {
			return nil, fmt.Errorf("error saving wallet.%s to db: %v", key, err)
		}
	}

	return &WalletBase{
		UserConfigDB:             params.ConfigDB,
		db:                       params.ConfigDB,
		log:                      params.Logger,
		dataDir:                  params.DataDir,
		network:                  params.Net,
		encryptedSeed:            encryptedSeed,
		traits:                   traits,
		accountDiscoveryRequired: accountDiscoveryRequired,
		syncHelper:               &syncHelper{log: params.Logger},
	}, nil
}

// OpenWalletBase loads basic information for an existing wallet from the
// provided params.
func OpenWalletBase(params OpenWalletParams) (*WalletBase, error) {
	w := &WalletBase{
		UserConfigDB: params.ConfigDB,
		db:           params.ConfigDB,
		log:          params.Logger,
		dataDir:      params.DataDir,
		network:      params.Net,
		syncHelper:   &syncHelper{log: params.Logger},
	}

	readFromDB := func(key string, wFieldPtr any, optional ...bool) error {
		err := w.db.ReadWalletConfigValue(key, wFieldPtr)
		if err == storm.ErrNotFound && len(optional) > 0 && optional[0] {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error reading wallet.%s from db: %v", key, err)
		}
		return nil
	}
	if err := readFromDB(encryptedSeedDBKey, &w.encryptedSeed, true); err != nil {
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

	if err = w.db.DeleteWalletConfigValue(encryptedSeedDBKey); err != nil {
		w.log.Errorf("db.DeleteWalletConfigValue(encryptedSeed) error: %v", err)
		return false, fmt.Errorf("database error")
	}

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
	if err := w.db.SaveWalletConfigValue(accountDiscoveryRequiredDBKey, w.accountDiscoveryRequired); err != nil {
		w.log.Errorf("error marking wallet discovery complete: %v", err)
	}
}

func (w *WalletBase) ReadUserConfigBoolValue(key string, defaultValue ...bool) bool {
	var v bool
	if err := w.ReadUserConfigValue(key, &v); err != nil {
		if len(defaultValue) > 0 {
			v = defaultValue[0]
		}
		if err != storm.ErrNotFound {
			w.log.Errorf("ReadUserConfigValue error: %v", err)
		}
	}
	return v
}

func (w *WalletBase) ReadUserConfigStringValue(key string, defaultValue ...string) string {
	var v string
	if err := w.ReadUserConfigValue(key, &v); err != nil {
		if len(defaultValue) > 0 {
			v = defaultValue[0]
		}
		if err != storm.ErrNotFound {
			w.log.Errorf("ReadUserConfigValue error: %v", err)
		}
	}
	return v
}
