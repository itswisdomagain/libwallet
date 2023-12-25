package main

import "C"
import (
	"decred.org/dcrwallet/v3/wallet/udb"
)

//export currentReceiveAddress
func currentReceiveAddress(walletName *C.char) *C.char {
	w, ok := loadedWallet(walletName)
	if !ok {
		return errCResponse("wallet with name %s no loaded", goString(walletName))
	}

	// Don't return an address if not synced!
	if !w.IsSynced() {
		return errCResponse("currentReceiveAddress requested on an unsynced wallet")
	}

	addr, err := w.CurrentAddress(udb.DefaultAccountNum)
	if err != nil {
		return errCResponse("w.CurrentAddress error: %v", err)
	}

	return resCResponse(addr.String())
}
