package main

import "C"
import (
	"encoding/base64"

	"decred.org/dcrwallet/v3/wallet/udb"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
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

//export newExternalAddress
func newExternalAddress(cName *C.char) *C.char {
	w, ok := loadedWallet(cName)
	if !ok {
		return errCResponse("wallet with name %q is not loaded", goString(cName))
	}

	// Don't return an address if not synced!
	if !w.IsSynced() {
		return errCResponseWithCode(ErrCodeNotSynced, "newExternalAddress requested on an unsynced wallet")
	}

	_, err := w.NewExternalAddress(ctx, udb.DefaultAccountNum)
	if err != nil {
		return errCResponse("w.NewExternalAddress error: %v", err)
	}

	// NewExternalAddress will take the current address before increasing
	// the index. Get the current address after increasing the index.
	addr, err := w.CurrentAddress(udb.DefaultAccountNum)
	if err != nil {
		return errCResponse("w.CurrentAddress error: %v", err)
	}

	return successCResponse(addr.String())
}

//export signMessage
func signMessage(cName, cMessage, cAddress, cPassword *C.char) *C.char {
	w, ok := loadedWallet(cName)
	if !ok {
		return errCResponse("wallet with name %q is not loaded", goString(cName))
	}

	addr, err := stdaddr.DecodeAddress(goString(cAddress), w.MainWallet().ChainParams())
	if err != nil {
		return errCResponse("unable to decode address: %v", err)
	}

	if err := w.MainWallet().Unlock(ctx, []byte(goString(cPassword)), nil); err != nil {
		return errCResponse("cannot unlock wallet: %v", err)
	}

	sig, err := w.MainWallet().SignMessage(ctx, goString(cMessage), addr)
	if err != nil {
		return errCResponse("unable to sign message: %v", err)
	}

	sEnc := base64.StdEncoding.EncodeToString(sig)

	return successCResponse(sEnc)
}
