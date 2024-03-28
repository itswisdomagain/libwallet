package main

import "C"
import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	dcrwallet "decred.org/dcrwallet/v3/wallet"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/itswisdomagain/libwallet/asset/dcr"
)

const defaultAccount = "default"

//export createSignedTransaction
func createSignedTransaction(cName, cCreateSignedTxJSONReq *C.char) *C.char {
	w, exists := loadedWallet(cName)
	if !exists {
		return errCResponse("wallet with name %q does not exist", goString(cName))
	}
	signSendJSONReq := goString(cCreateSignedTxJSONReq)
	var req CreateSignedTxReq
	if err := json.Unmarshal([]byte(signSendJSONReq), &req); err != nil {
		return errCResponse("malformed sign send request: %v", err)
	}

	outputs := make([]*dcr.Output, len(req.Outputs))
	for i, out := range req.Outputs {
		o := &dcr.Output{
			Address: out.Address,
			Amount:  uint64(out.Amount),
		}
		outputs[i] = o
	}

	inputs := make([]*dcr.Input, len(req.Inputs))
	for i, in := range req.Inputs {
		o := &dcr.Input{
			TxID: in.TxID,
			Vout: uint32(in.Vout),
		}
		inputs[i] = o
	}

	ignoreInputs := make([]*dcr.Input, len(req.IgnoreInputs))
	for i, in := range req.IgnoreInputs {
		o := &dcr.Input{
			TxID: in.TxID,
			Vout: uint32(in.Vout),
		}
		ignoreInputs[i] = o
	}

	if err := w.MainWallet().Unlock(ctx, []byte(req.Password), nil); err != nil {
		return errCResponse("cannot unlock wallet: %v", err)
	}
	defer w.MainWallet().Lock()

	txBytes, txhash, fee, err := w.CreateSignedTransaction(ctx, outputs, inputs, ignoreInputs, uint64(req.FeeRate))
	if err != nil {
		return errCResponse("unable to sign send transaction: %v", err)
	}
	res := &CreateSignedTxRes{
		SignedHex: hex.EncodeToString(txBytes),
		Txid:      txhash.String(),
		Fee:       int(fee),
	}

	b, err := json.Marshal(res)
	if err != nil {
		return errCResponse("unable to marshal sign send transaction result: %v", err)
	}
	return successCResponse(string(b))
}

//export sendRawTransaction
func sendRawTransaction(cName, cTxHex *C.char) *C.char {
	w, exists := loadedWallet(cName)
	if !exists {
		return errCResponse("wallet with name %q does not exist", goString(cName))
	}
	txHash, err := w.SendRawTransaction(ctx, goString(cTxHex))
	if err != nil {
		return errCResponse("unable to sign send transaction: %v", err)
	}
	return successCResponse(txHash.String())
}

//export listUnspents
func listUnspents(cName *C.char) *C.char {
	w, exists := loadedWallet(cName)
	if !exists {
		return errCResponse("wallet with name %q does not exist", goString(cName))
	}
	res, err := w.MainWallet().ListUnspent(ctx, 1, math.MaxInt32, nil, defaultAccount)
	if err != nil {
		return errCResponse("unable to get unspents: %v", err)
	}
	// Add is change to results.
	unspentRes := make([]ListUnspentRes, len(res))
	for i, unspent := range res {
		addr, err := stdaddr.DecodeAddress(unspent.Address, w.MainWallet().ChainParams())
		if err != nil {
			return errCResponse("unable to decode address: %v", err)
		}

		ka, err := w.MainWallet().KnownAddress(ctx, addr)
		if err != nil {
			return errCResponse("unspent address is not known: %v", err)
		}

		isChange := false
		if ka, ok := ka.(dcrwallet.BIP0044Address); ok {
			_, branch, _ := ka.Path()
			isChange = branch == 1
		}
		unspentRes[i] = ListUnspentRes{
			ListUnspentResult: unspent,
			IsChange:          isChange,
		}
	}
	b, err := json.Marshal(unspentRes)
	if err != nil {
		return errCResponse("unable to marshal list unspents result: %v", err)
	}
	return successCResponse(string(b))
}

//export estimateFee
func estimateFee(cName, cNBlocks *C.char) *C.char {
	w, exists := loadedWallet(cName)
	if !exists {
		return errCResponse("wallet with name %q does not exist", goString(cName))
	}
	nBlocks, err := strconv.ParseUint(goString(cNBlocks), 10, 64)
	if err != nil {
		return errCResponse("number of blocks is not a uint64: %v", err)
	}
	txFee, err := w.FetchFeeFromOracle(ctx, nBlocks)
	if err != nil {
		return errCResponse("unable to get fee from oracle: %v", err)
	}
	return successCResponse(fmt.Sprintf("%d", (uint64(txFee * 1e8))))
}

//export listTransactions
func listTransactions(cName, cFrom, cCount *C.char) *C.char {
	w, exists := loadedWallet(cName)
	if !exists {
		return errCResponse("wallet with name %q does not exist", goString(cName))
	}
	from, err := strconv.ParseInt(goString(cFrom), 10, 32)
	if err != nil {
		return errCResponse("from is not an int: %v", err)
	}
	count, err := strconv.ParseInt(goString(cCount), 10, 32)
	if err != nil {
		return errCResponse("count is not an int: %v", err)
	}
	res, err := w.MainWallet().ListTransactions(ctx, int(from), int(count))
	if err != nil {
		return errCResponse("unable to get transactions: %v", err)
	}
	ltRes := make([]*ListTransactionRes, len(res))
	for i, ltw := range res {
		lt := &ListTransactionRes{
			Address:       ltw.Address,
			Amount:        ltw.Amount,
			Category:      ltw.Category,
			Confirmations: ltw.Confirmations,
			Fee:           ltw.Fee,
			Time:          ltw.Time,
			TxID:          ltw.TxID,
		}
		ltRes[i] = lt
	}
	b, err := json.Marshal(ltRes)
	if err != nil {
		return errCResponse("unable to marshal list transactions result: %v", err)
	}
	return successCResponse(string(b))
}

//export bestBlock
func bestBlock(cName *C.char) *C.char {
	w, exists := loadedWallet(cName)
	if !exists {
		return errCResponse("wallet with name %q does not exist", goString(cName))
	}
	blockHash, blockHeight := w.MainWallet().MainChainTip(ctx)
	res := &BestBlockRes{
		Hash:   blockHash.String(),
		Height: int(blockHeight),
	}
	b, err := json.Marshal(res)
	if err != nil {
		return errCResponse("unable to marshal best block result: %v", err)
	}
	return successCResponse(string(b))
}
