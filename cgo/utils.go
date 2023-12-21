package main

import "C"
import (
	"fmt"
)

func loadedWallet(cName *C.char) (*wallet, bool) {
	walletsMtx.Lock()
	defer walletsMtx.Unlock()

	name := goString(cName)
	w, ok := wallets[name]
	if !ok {
		log.Debugf("attempted to use an unloaded wallet %q", name)
	}
	return w, ok
}

func cString(str string) *C.char {
	return C.CString(str)
}

func cStringF(format string, a ...any) *C.char {
	return C.CString(fmt.Sprintf(format, a...))
}

func cError(err error) *C.char {
	return C.CString(err.Error())
}

func goString(cstr *C.char) string {
	return C.GoString(cstr)
}
