package main

import "C"
import (
	"decred.org/dcrwallet/v3/wallet/udb"
)

//export currentReceiveAddress
func currentReceiveAddress(cName *C.char) *C.char {
	w, ok := loadedWallet(cName)
	if !ok {
		return errCResponse("wallet with name %q is not loaded", goString(cName))
	}

	// Don't return an address if not synced!
	if !w.IsSynced() {
		return errCResponseWithCode(ErrCodeNotSynced, "currentReceiveAddress requested on an unsynced wallet")
	}

	addr, err := w.CurrentAddress(udb.DefaultAccountNum)
	if err != nil {
		return errCResponse("w.CurrentAddress error: %v", err)
	}

	return successCResponse(addr.String())
}
