package dcr

import (
	"context"
	"errors"

	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/hdkeychain/v3"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
)

// AddressesByAccount handles a getaddressesbyaccount request by returning
// all addresses for an account, or an error if the requested account does
// not exist. Does not include the current address that can be retrieved with
// w.mainWallet.CurrentAddress.
func (w *Wallet) AddressesByAccount(ctx context.Context, account string) ([]string, error) {
	if account == "imported" {
		addrs, err := w.mainWallet.ImportedAddresses(ctx, account)
		if err != nil {
			return nil, err
		}
		addrStrs := make([]string, len(addrs))
		for i := range addrs {
			addrStrs[i] = addrs[i].String()
		}
		return addrStrs, nil
	}
	accountNum, err := w.mainWallet.AccountNumber(ctx, account)
	if err != nil {
		return nil, err
	}
	xpub, err := w.mainWallet.AccountXpub(ctx, accountNum)
	if err != nil {
		return nil, err
	}
	extBranch, err := xpub.Child(0)
	if err != nil {
		return nil, err
	}
	intBranch, err := xpub.Child(1)
	if err != nil {
		return nil, err
	}
	endExt, endInt, err := w.mainWallet.BIP0044BranchNextIndexes(ctx, accountNum)
	if err != nil {
		return nil, err
	}
	params := w.mainWallet.ChainParams()
	addrs := make([]string, 0, endExt+endInt)
	appendAddrs := func(branchKey *hdkeychain.ExtendedKey, n uint32) error {
		for i := uint32(0); i < n; i++ {
			child, err := branchKey.Child(i)
			if errors.Is(err, hdkeychain.ErrInvalidChild) {
				continue
			}
			if err != nil {
				return err
			}
			pkh := dcrutil.Hash160(child.SerializedPubKey())
			addr, _ := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
				pkh, params)
			addrs = append(addrs, addr.String())
		}
		return nil
	}
	err = appendAddrs(extBranch, endExt)
	if err != nil {
		return nil, err
	}
	err = appendAddrs(intBranch, endInt)
	if err != nil {
		return nil, err
	}
	return addrs, nil
}
