package ltc

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	neutrino "github.com/dcrlabs/neutrino-ltc"
	"github.com/dcrlabs/neutrino-ltc/chain"
	"github.com/itswisdomagain/libwallet/asset"
	"github.com/itswisdomagain/libwallet/assetlog"
	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/ltcutil/hdkeychain"
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

// CreateWallet creates and opens an SPV wallet. If recovery params is not
// provided, a new seed is generated and used. The seed is encrypted with the
// provided passphrase and can be revealed for backup later by providing the
// passphrase.
func CreateWallet[Tx any](ctx context.Context, params asset.CreateWalletParams[Tx], recovery *asset.RecoveryCfg) (*Wallet[Tx], error) {
	chainParams, err := ParseChainParams(params.Net)
	if err != nil {
		return nil, fmt.Errorf("error parsing chain: %w", err)
	}

	loader := wallet.NewLoader(chainParams, params.DataDir, true, dbTimeout, 250)
	if exists, err := loader.WalletExists(); err != nil {
		return nil, fmt.Errorf("error checking if wallet already exist: %w", err)
	} else if exists {
		return nil, fmt.Errorf("wallet at %q already exists", params.DataDir)
	}

	var seed []byte
	var walletTraits asset.WalletTrait
	if recovery != nil {
		seed = recovery.Seed
		walletTraits = asset.WalletTraitRestored
	} else {
		seed, err = hdkeychain.GenerateSeed(hdkeychain.RecommendedSeedLen)
		if err != nil {
			return nil, fmt.Errorf("unable to generate random seed: %v", err)
		}
	}

	wb, err := asset.NewWalletBase[Tx](params.OpenWalletParams, seed, params.Pass, walletTraits)
	if err != nil {
		return nil, fmt.Errorf("CreateWalletBase error: %v", err)
	}

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

	// Create the chain service DB.
	neutrinoDBPath := filepath.Join(params.DataDir, neutrinoDBName)
	db, err := walletdb.Create(params.DbDriver, neutrinoDBPath, true, dbTimeout)
	if err != nil {
		return nil, fmt.Errorf("unable to create neutrino db at %q: %w", neutrinoDBPath, err)
	}

	chainService, err := initializeChainService(params.DataDir, db, *chainParams)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize neutrino ChainService: %w", err)
	}

	bailOnWallet = false
	btcLogger := &assetlog.BTCLogger{Logger: params.Logger}
	return &Wallet[Tx]{
		WalletBase:   wb,
		mainWallet:   ltcw,
		dir:          params.DataDir,
		dbDriver:     params.DbDriver,
		log:          params.Logger,
		loader:       loader,
		db:           db,
		chainService: chainService,
		chainClient:  chain.NewNeutrinoClient(chainParams, chainService, btcLogger),
	}, nil
}

// CreateWatchOnlyWallet creates and opens a watchonly SPV wallet.
func CreateWatchOnlyWallet[Tx any](ctx context.Context, extendedPubKey string, params asset.CreateWalletParams[Tx]) (*Wallet[Tx], error) {
	chainParams, err := ParseChainParams(params.Net)
	if err != nil {
		return nil, fmt.Errorf("error parsing chain: %w", err)
	}

	if exists, err := WalletExistsAt(params.DataDir); err != nil {
		return nil, fmt.Errorf("error checking if wallet already exist: %w", err)
	} else if exists {
		return nil, fmt.Errorf("wallet at %q already exists", params.DataDir)
	}

	wb, err := asset.NewWalletBase[Tx](params.OpenWalletParams, nil, nil, asset.WalletTraitWatchOnly)
	if err != nil {
		return nil, fmt.Errorf("NewWalletBase error: %v", err)
	}

	loader := wallet.NewLoader(chainParams, params.DataDir, true, dbTimeout, 250)

	pubPass := []byte(wallet.InsecurePubPassphrase)
	ltcw, err := loader.CreateNewWatchingOnlyWallet(pubPass, params.Birthday)
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

	// The chain service DB
	neutrinoDBPath := filepath.Join(params.DataDir, neutrinoDBName)
	db, err := walletdb.Create(params.DbDriver, neutrinoDBPath, true, dbTimeout)
	if err != nil {
		return nil, fmt.Errorf("unable to create neutrino db at %q: %w", neutrinoDBPath, err)
	}

	chainService, err := initializeChainService(params.DataDir, db, *chainParams)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize neutrino ChainService: %w", err)
	}

	bailOnWallet = false
	btcLogger := &assetlog.BTCLogger{Logger: params.Logger}
	return &Wallet[Tx]{
		WalletBase:   wb,
		mainWallet:   ltcw,
		dir:          params.DataDir,
		dbDriver:     params.DbDriver,
		log:          params.Logger,
		loader:       loader,
		db:           db,
		chainService: chainService,
		chainClient:  chain.NewNeutrinoClient(chainParams, chainService, btcLogger),
	}, nil
}

// LoadWallet loads a previously created SPV wallet. The wallet must be opened
// via its OpenWallet method before it can be used.
func LoadWallet[Tx any](ctx context.Context, params asset.OpenWalletParams[Tx]) (*Wallet[Tx], error) {
	chainParams, err := ParseChainParams(params.Net)
	if err != nil {
		return nil, fmt.Errorf("error parsing chain params: %w", err)
	}

	loader := wallet.NewLoader(chainParams, params.DataDir, true, dbTimeout, 250)
	if exists, err := loader.WalletExists(); err != nil {
		return nil, err
	} else if !exists {
		return nil, fmt.Errorf("wallet at %q doesn't exist", params.DataDir)
	}

	wb, err := asset.OpenWalletBase[Tx](params)
	if err != nil {
		return nil, fmt.Errorf("OpenWalletBase error: %v", err)
	}

	// Open the chain service DB.
	neutrinoDBPath := filepath.Join(params.DataDir, neutrinoDBName)
	db, err := walletdb.Open(params.DbDriver, neutrinoDBPath, true, dbTimeout)
	if err != nil {
		return nil, fmt.Errorf("unable to open neutrino db at %q: %w", neutrinoDBPath, err)
	}

	chainService, err := initializeChainService(params.DataDir, db, *chainParams)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize neutrino ChainService: %w", err)
	}

	btcLogger := &assetlog.BTCLogger{Logger: params.Logger}
	return &Wallet[Tx]{
		WalletBase:   wb,
		dir:          params.DataDir,
		dbDriver:     params.DbDriver,
		log:          params.Logger,
		loader:       loader,
		db:           db,
		chainService: chainService,
		chainClient:  chain.NewNeutrinoClient(chainParams, chainService, btcLogger),
	}, nil
}

func initializeChainService(dataDir string, db walletdb.DB, chainParams chaincfg.Params) (*neutrino.ChainService, error) {
	return neutrino.NewChainService(neutrino.Config{
		DataDir:       dataDir,
		Database:      db,
		ChainParams:   chainParams,
		PersistToDisk: true, // keep cfilter headers on disk for efficient rescanning
		// AddPeers:      addPeers,
		// ConnectPeers:  connectPeers,
		// WARNING: PublishTransaction currently uses the entire duration
		// because if an external bug, but even if the bug is resolved, a typical
		// inv/getdata round trip is ~4 seconds, so we set this so neutrino does
		// not cancel queries too readily.
		BroadcastTimeout: 6 * time.Second,
	})
}
