package ltc

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/itswisdomagain/libwallet/asset"
	"github.com/ltcsuite/ltcwallet/wallet"
	"github.com/ltcsuite/ltcwallet/walletdb"
	_ "github.com/ltcsuite/ltcwallet/walletdb/bdb"
)

const (
	dbTimeout      = 20 * time.Second
	neutrinoDBName = "neutrino.db"
)

// WalletExistsAt checks the existence of the wallet.
func WalletExistsAt(dir string) (bool, error) {
	// only the dir argument is needed to check wallet existence.
	loader := wallet.NewLoader(nil, dir, true, dbTimeout, 250)
	return loader.WalletExists()
}

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

	ltcw, err := loader.CreateNewWallet(pubPass, params.Pass, seed, params.Birthday)
	if err != nil {
		return nil, fmt.Errorf("CreateNewWallet error: %w", err)
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
		err = extendAddresses(recovery.NumExternalAddresses, recovery.NumInternalAddresses, ltcw)
		if err != nil {
			return nil, fmt.Errorf("failed to set starting address indexes: %w", err)
		}
	}

	// The chain service DB
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
		mainWallet:  ltcw,
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

	// TODO: Load the (encrypted) seed as well.

	return &Wallet{
		dir:         params.DataDir,
		dbDriver:    params.DbDriver,
		chainParams: chainParams,
		log:         params.Logger,
	}, nil
}