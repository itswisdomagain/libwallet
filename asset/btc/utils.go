package btc

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/itswisdomagain/libwallet/asset"
)

func ParseChainParams(net asset.Network) (*chaincfg.Params, error) {
	switch net {
	case asset.Mainnet:
		return &chaincfg.MainNetParams, nil
	case asset.Testnet:
		return &chaincfg.TestNet3Params, nil
	case asset.Regtest:
		return &chaincfg.RegressionNetParams, nil
	}
	return nil, fmt.Errorf("unknown network ID %v", net)
}

func extendAddresses(extIdx, intIdx uint32, btcw *wallet.Wallet) error {
	scopedKeyManager, err := btcw.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeBIP0084)
	if err != nil {
		return err
	}

	return walletdb.Update(btcw.Database(), func(dbtx walletdb.ReadWriteTx) error {
		ns := dbtx.ReadWriteBucket(wAddrMgrBkt)
		if extIdx > 0 {
			if err := scopedKeyManager.ExtendExternalAddresses(ns, waddrmgr.DefaultAccountNum, extIdx); err != nil {
				return err
			}
		}
		if intIdx > 0 {
			return scopedKeyManager.ExtendInternalAddresses(ns, waddrmgr.DefaultAccountNum, intIdx)
		}
		return nil
	})
}
