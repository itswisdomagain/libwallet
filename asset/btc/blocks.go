package btc

import "github.com/btcsuite/btcd/chaincfg/chainhash"

// GetBlockHash returns the block hash for the given block height.
func (w *Wallet[Tx]) GetBlockHash(height int64) (*chainhash.Hash, error) {
	return w.ChainClient().GetBlockHash(height)
}
