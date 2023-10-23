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
	encryptedSeedDBKey            = "encryptedSeed"
	isRestoredDBKey               = "isRestored"
	accountDiscoveryRequiredDBKey = "accountDiscoveryRequired"
)

type SeededWallet struct {
	// UserConfigDB is publicly embedded, so consumers can read from/write to
	// the user config part of the walletdata db.
	// TODO: Also allow users read/write transaction data.
	walletdata.UserConfigDB

	// db is private, for internal use only.
	db  walletdata.WalletConfigDB
	log slog.Logger

	// ID is temporary. Remove!
	ID int

	mtx                      sync.Mutex
	encryptedSeed            []byte
	isRestored               bool
	accountDiscoveryRequired bool
}

// NewSeededWallet uses the provided information to initialize a SeededWallet
// instance. TODO: Remove id parameter!
func NewSeededWallet(id int, seed, walletPass []byte, isRestored bool, db WalletDataDB, log slog.Logger) (*SeededWallet, error) {
	if len(seed) == 0 {
		return nil, fmt.Errorf("seed is required")
	}

	encryptedSeed, err := EncryptData(seed, walletPass)
	if err != nil {
		return nil, fmt.Errorf("seed encryption error: %v", err)
	}

	// Account discovery is only required for restored wallets.
	accountDiscoveryRequired := isRestored

	// Save the initial data to db.
	dbData := map[string]any{
		encryptedSeedDBKey:            encryptedSeed,
		isRestoredDBKey:               isRestored,
		accountDiscoveryRequiredDBKey: accountDiscoveryRequired,
	}
	for key, value := range dbData {
		if err := db.SaveWalletConfigValue(key, value); err != nil {
			return nil, fmt.Errorf("error saving wallet.%s to db: %v", key, err)
		}
	}

	return &SeededWallet{
		ID:                       id,
		UserConfigDB:             db,
		db:                       db,
		log:                      log,
		encryptedSeed:            encryptedSeed,
		isRestored:               isRestored,
		accountDiscoveryRequired: accountDiscoveryRequired,
	}, nil
}

// SeededWalletFromDB loads wallet information from db and uses it to initialize
// a SeededWallet instance. TODO: Remove id parameter!
func SeededWalletFromDB(id int, db WalletDataDB, log slog.Logger) (*SeededWallet, error) {
	w := &SeededWallet{
		ID:           id,
		UserConfigDB: db,
		db:           db,
		log:          log,
	}

	readFromDB := func(key string, wFieldPtr any) error {
		if err := db.ReadWalletConfigValue(key, wFieldPtr); err != nil {
			return fmt.Errorf("error reading wallet.%s from db: %v", key, err)
		}
		return nil
	}
	if err := readFromDB(encryptedSeedDBKey, &w.encryptedSeed); err != nil {
		return nil, err
	}
	if err := readFromDB(isRestoredDBKey, &w.isRestored); err != nil {
		return nil, err
	}
	if err := readFromDB(accountDiscoveryRequiredDBKey, &w.accountDiscoveryRequired); err != nil {
		return nil, err
	}

	return w, nil
}

// DecryptSeed decrypts the encrypted wallet seed using the provided passphrase.
func (w *SeededWallet) DecryptSeed(passphrase []byte) (string, error) {
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

func (w *SeededWallet) ReEncryptSeed(oldPass, newPass []byte) error {
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

// VerifySeed decrypts the encrypted wallet seed using the provided passphrase
// and compares it with the provided seedMnemonic. If it's a match, the wallet
// seed will no longer be saved.
func (w *SeededWallet) VerifySeed(seedMnemonic string, passphrase []byte) (bool, error) {
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

func (w *SeededWallet) IsRestored() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.isRestored
}

func (w *SeededWallet) AccountDiscoveryRequired() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.accountDiscoveryRequired
}

func (w *SeededWallet) MarkAccountDiscoveryComplete() {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	w.accountDiscoveryRequired = false
	if err := w.db.SaveWalletConfigValue(accountDiscoveryRequiredDBKey, w.accountDiscoveryRequired); err != nil {
		w.log.Errorf("error marking wallet discovery complete: %v", err)
	}
}
