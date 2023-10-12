package ltc

import (
	"fmt"

	"github.com/itswisdomagain/libwallet/asset"
	"github.com/ltcsuite/ltcd/chaincfg"
	ltcwaddrmgr "github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/ltcsuite/ltcwallet/wallet"
	"github.com/ltcsuite/ltcwallet/walletdb"
)

func ParseChainParams(net asset.Network) (*chaincfg.Params, error) {
	switch net {
	case asset.Mainnet:
		return &chaincfg.MainNetParams, nil
	case asset.Testnet:
		return &chaincfg.TestNet4Params, nil
	case asset.Regtest:
		return &chaincfg.RegressionNetParams, nil
	}
	return nil, fmt.Errorf("unknown network ID %v", net)
}

func extendAddresses(extIdx, intIdx uint32, ltcw *wallet.Wallet) error {
	scopedKeyManager, err := ltcw.Manager.FetchScopedKeyManager(ltcwaddrmgr.KeyScopeBIP0084)
	if err != nil {
		return err
	}

	return walletdb.Update(ltcw.Database(), func(dbtx walletdb.ReadWriteTx) error {
		var waddrmgrNamespace = []byte("waddrmgr")
		ns := dbtx.ReadWriteBucket(waddrmgrNamespace)
		if extIdx > 0 {
			scopedKeyManager.ExtendExternalAddresses(ns, ltcwaddrmgr.DefaultAccountNum, extIdx)
		}
		if intIdx > 0 {
			scopedKeyManager.ExtendInternalAddresses(ns, ltcwaddrmgr.DefaultAccountNum, intIdx)
		}
		return nil
	})
}
