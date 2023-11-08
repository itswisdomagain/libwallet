package dcr

import (
	"context"
	"fmt"
	"os"

	"decred.org/dcrwallet/v3/wallet"
	"decred.org/dcrwallet/v3/wallet/udb"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/itswisdomagain/libwallet/asset"
)

func ParseChainParams(network asset.Network) (*chaincfg.Params, error) {
	// Get network settings. Zero value is mainnet, but unknown non-zero cfg.Net
	// is an error.
	switch network {
	case asset.Simnet:
		return chaincfg.SimNetParams(), nil
	case asset.Testnet:
		return chaincfg.TestNet3Params(), nil
	case asset.Mainnet:
		return chaincfg.MainNetParams(), nil
	default:
		return nil, fmt.Errorf("unknown network ID: %d", uint8(network))
	}
}

// extendAddresses ensures that the internal and external branches have been
// extended to the specified indices. This can be used at wallet restoration to
// ensure that no duplicates are encountered with existing but unused addresses.
func extendAddresses(ctx context.Context, extIdx, intIdx uint32, dcrw *wallet.Wallet) error {
	if err := dcrw.SyncLastReturnedAddress(ctx, udb.DefaultAccountNum, udb.ExternalBranch, extIdx); err != nil {
		return fmt.Errorf("error syncing external branch index: %w", err)
	}

	if err := dcrw.SyncLastReturnedAddress(ctx, udb.DefaultAccountNum, udb.InternalBranch, intIdx); err != nil {
		return fmt.Errorf("error syncing internal branch index: %w", err)
	}

	return nil
}

func fileExists(filePath string) (bool, error) {
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func checkCreateDir(path string) error {
	if fi, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			// Attempt data directory creation
			if err = os.MkdirAll(path, 0700); err != nil {
				return fmt.Errorf("cannot create directory: %s", err)
			}
		} else {
			return fmt.Errorf("error checking directory: %s", err)
		}
	} else if !fi.IsDir() {
		return fmt.Errorf("path '%s' is not a directory", path)
	}

	return nil
}
