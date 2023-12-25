package main

import "C"
import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/asset"
	"github.com/itswisdomagain/libwallet/asset/dcr"
)

const emptyJsonObject = "{}"

type wallet struct {
	*dcr.Wallet
	log slog.Logger

	syncStatusMtx                                             sync.RWMutex
	syncStatusCode                                            SyncStatusCode
	targetHeight, cfiltersHeight, headersHeight, rescanHeight int
}

//export createWallet
func createWallet(cName, cDataDir, cNet, cPass *C.char) *C.char {
	walletsMtx.Lock()
	defer walletsMtx.Unlock()
	if wallets == nil {
		return errCResponse("libwallet is not initialized")
	}

	name := goString(cName)
	if _, exists := wallets[name]; exists {
		return errCResponse("wallet already exists with name: %q", name)
	}

	network, err := asset.NetFromString(goString(cNet))
	if err != nil {
		return errCResponse(err.Error())
	}

	logger := logBackend.Logger("[" + name + "]")
	logger.SetLevel(slog.LevelTrace)
	params := asset.CreateWalletParams{
		OpenWalletParams: asset.OpenWalletParams{
			Net:      network,
			DataDir:  goString(cDataDir),
			DbDriver: "bdb", // use badgerdb for mobile!
			Logger:   logger,
		},
		Pass: []byte(goString(cPass)),
	}
	w, err := dcr.CreateWallet(ctx, params, nil)
	if err != nil {
		return errCResponse(err.Error())
	}

	wallets[name] = &wallet{
		Wallet: w,
		log:    logger,
	}
	return resCResponse("wallet created")
}

//export loadWallet
func loadWallet(cName, cDataDir, cNet *C.char) *C.char {
	walletsMtx.Lock()
	defer walletsMtx.Unlock()
	if wallets == nil {
		return errCResponse("libwallet is not initialized")
	}

	name := goString(cName)
	if _, exists := wallets[name]; exists {
		return resCResponse("wallet already loaded") // not an error, already loaded
	}

	network, err := asset.NetFromString(goString(cNet))
	if err != nil {
		return errCResponse(err.Error())
	}

	logger := logBackend.Logger("[" + name + "]")
	logger.SetLevel(slog.LevelTrace)
	params := asset.OpenWalletParams{
		Net:      network,
		DataDir:  goString(cDataDir),
		DbDriver: "bdb", // use badgerdb for mobile!
		Logger:   logger,
	}
	w, err := dcr.LoadWallet(ctx, params)
	if err != nil {
		return errCResponse(err.Error())
	}

	if err = w.OpenWallet(ctx); err != nil {
		return errCResponse(err.Error())
	}

	wallets[name] = &wallet{
		Wallet: w,
		log:    logger,
	}
	return resCResponse(fmt.Sprintf("wallet %s loaded", name))
}

//export walletSeed
func walletSeed(cName, cPass *C.char) *C.char {
	w, ok := loadedWallet(cName)
	if !ok {
		return errCResponse("wallet not loaded")
	}

	seed, err := w.DecryptSeed([]byte(goString(cPass)))
	if err != nil {
		return errCResponse("w.DecryptSeed error: %v", err)
	}

	return resCResponse(seed)
}

//export walletBalance
func walletBalance(cName *C.char) *C.char {
	w, ok := loadedWallet(cName)
	if !ok {
		return resCResponse(emptyJsonObject)
	}

	bals, err := w.AccountBalances(ctx, 0)
	if err != nil {
		return errCResponse("w.AccountBalances error: %v", err)
	}

	balMap := map[string]float64{
		"confirmed":   0,
		"unconfirmed": 0,
	}

	for _, bal := range bals {
		balMap["confirmed"] += bal.Spendable.ToCoin()
		balMap["unconfirmed"] += bal.Total.ToCoin() - bal.Spendable.ToCoin()
	}

	balJson, err := json.Marshal(balMap)
	if err != nil {
		return errCResponse("marshal balMap error: %v", err)
	}

	return resCResponse(string(balJson))
}
