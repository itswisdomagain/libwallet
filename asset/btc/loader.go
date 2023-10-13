package btc

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
	"github.com/itswisdomagain/libwallet/asset"
)

const (
	dbTimeout      = 20 * time.Second
	neutrinoDBName = "neutrino.db"
)

// WalletExistsAt checks if a wallet exists at the specified directory.
func WalletExistsAt(dir string) (bool, error) {
	// only the dir argument is needed to check wallet existence.
	loader := wallet.NewLoader(nil, dir, true, dbTimeout, 250)
	return loader.WalletExists()
}

// CreateWallet creates and returns a new, loaded but unsynced SPV wallet. If
// recovery seed is not provided, a new seed is generated and used. The wallet
// seed is encrypted with the provided passphrase and can be revealed for backup
// later by providing the passphrase.
func CreateWallet(ctx context.Context, params asset.CreateWalletParams, recovery *asset.RecoveryCfg) (*Wallet, error) {
	chainParams, err := ParseChainParams(params.Net)
	if err != nil {
		return nil, fmt.Errorf("error parsing chain: %w", err)
	}

	if exists, err := WalletExistsAt(params.DataDir); err != nil {
		return nil, fmt.Errorf("error checking if wallet already exist: %w", err)
	} else if exists {
		return nil, fmt.Errorf("wallet at %q already exists", params.DataDir)
	}

	var seed []byte
	if recovery != nil {
		seed = recovery.Seed
	} else {
		// TODO: Generate seed.
	}

	// TODO: Encrypt the seed with the private passphrase and save. Should be
	// able to reveal the seed for this wallet later by providing the private
	// passphrase. NOTE: cake wallet might require storing the seed without
	// encrypting first. Insecure, but ...

	loader := wallet.NewLoader(chainParams, params.DataDir, true, dbTimeout, 250)

	pubPass := []byte(wallet.InsecurePubPassphrase)

	btcw, err := loader.CreateNewWallet(pubPass, params.Pass, seed, params.Birthday)
	if err != nil {
		return nil, err
	}

	bailOnWallet := true // changed to false if there are no errors below
	defer func() {
		if bailOnWallet {
			if err := loader.UnloadWallet(); err != nil {
				params.Logger.Errorf("Error unloading wallet after CreateWallet error:", err)
			}
		}
	}()

	if recovery != nil && (recovery.NumExternalAddresses > 0 || recovery.NumInternalAddresses > 0) {
		err = extendAddresses(recovery.NumExternalAddresses, recovery.NumInternalAddresses, btcw)
		if err != nil {
			return nil, fmt.Errorf("failed to set starting address indexes: %w", err)
		}
	}

	// Create the chain service DB.
	neutrinoDBPath := filepath.Join(params.DataDir, neutrinoDBName)
	db, err := walletdb.Create(params.DbDriver, neutrinoDBPath, true, dbTimeout)
	if err != nil {
		return nil, fmt.Errorf("unable to create neutrino db at %q: %w", neutrinoDBPath, err)
	}

	bailOnWallet = false
	return &Wallet{
		dir:         params.DataDir,
		dbDriver:    params.DbDriver,
		chainParams: chainParams,
		log:         params.Logger,
		loader:      loader,
		db:          db,
		mainWallet:  btcw,
	}, nil
}

// LoadWallet loads a previously created native SPV wallet. The wallet must be
// opened via its OpenWallet method before it can be used.
func LoadWallet(ctx context.Context, params asset.OpenWalletParams) (*Wallet, error) {
	if exists, err := WalletExistsAt(params.DataDir); err != nil {
		return nil, err
	} else if !exists {
		return nil, fmt.Errorf("wallet at %q doesn't exist", params.DataDir)
	}

	chainParams, err := ParseChainParams(params.Net)
	if err != nil {
		return nil, fmt.Errorf("error parsing chain params: %w", err)
	}

	// TODO: Fetch the (encrypted) seed as well.

	return &Wallet{
		dir:         params.DataDir,
		dbDriver:    params.DbDriver,
		chainParams: chainParams,
		log:         params.Logger,
	}, nil
}