package main

import "C"
import "decred.org/dcrwallet/v3/wallet/udb"

//export currentReceiveAddress
func currentReceiveAddress(walletName *C.char) *C.char {
	w, ok := loadedWallet(walletName)
	if !ok {
		return nil
	}

	// Don't return an address if not synced!
	if !w.IsSynced() {
		w.log.Trace("currentReceiveAddress requested on an unsynced wallet")
		return nil
	}

	addr, err := w.CurrentAddress(udb.DefaultAccountNum)
	if err != nil {
		w.log.Errorf("w.CurrentAddress error: %v", err)
		return nil
	}

	return cString(addr.String())
}
