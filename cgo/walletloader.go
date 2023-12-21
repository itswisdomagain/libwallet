package main

import "C"
import (
	"encoding/json"

	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/asset"
	"github.com/itswisdomagain/libwallet/asset/dcr"
)

const emptyJsonObject = "{}"

type wallet struct {
	*dcr.Wallet
	log slog.Logger
}

//export createWallet
func createWallet(cName, cDataDir, cNet, cPass *C.char) *C.char {
	walletsMtx.Lock()
	defer walletsMtx.Unlock()
	if wallets == nil {
		return cString("libwallet is not initialized")
	}

	name := goString(cName)
	if _, exists := wallets[name]; exists {
		return cStringF("wallet already exists with name: %q", name)
	}

	network, err := asset.NetFromString(goString(cNet))
	if err != nil {
		return cError(err)
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
		return cError(err)
	}

	wallets[name] = &wallet{
		Wallet: w,
		log:    logger,
	}
	return nil
}

//export loadWallet
func loadWallet(cName, cDataDir, cNet *C.char) *C.char {
	walletsMtx.Lock()
	defer walletsMtx.Unlock()
	if wallets == nil {
		return cString("libwallet is not initialized")
	}

	name := goString(cName)
	if _, exists := wallets[name]; exists {
		return nil // not an error, already loaded
	}

	network, err := asset.NetFromString(goString(cNet))
	if err != nil {
		return cError(err)
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
		return cError(err)
	}

	if err = w.OpenWallet(ctx); err != nil {
		return cError(err)
	}

	wallets[name] = &wallet{
		Wallet: w,
		log:    logger,
	}
	return nil
}

//export walletSeed
func walletSeed(name, pass *C.char) *C.char {
	w, ok := loadedWallet(name)
	if !ok {
		return nil
	}

	seed, err := w.DecryptSeed([]byte(goString(pass)))
	if err != nil {
		w.log.Errorf("w.RevealSeed error: %v", err)
		return nil
	}

	return cString(seed)
}

//export walletBalance
func walletBalance(name *C.char) *C.char {
	w, ok := loadedWallet(name)
	if !ok {
		return cString(emptyJsonObject)
	}

	balMap := map[string]float64{
		"confirmed":   0,
		"unconfirmed": 0,
	}

	bals, err := w.AccountBalances(ctx, 0)
	if err != nil {
		w.log.Errorf("w.AccountBalances error: %v", err)
	} else {
		for _, bal := range bals {
			balMap["confirmed"] += bal.Spendable.ToCoin()
			balMap["unconfirmed"] += bal.Total.ToCoin() - bal.Spendable.ToCoin()
		}
	}

	balJson, err := json.Marshal(balMap)
	if err != nil {
		w.log.Errorf("marshal balMap error: %v", err)
		return cString(emptyJsonObject)
	}

	return cString(string(balJson))
}
