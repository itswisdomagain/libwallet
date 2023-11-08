package dcr

import (
	"decred.org/dcrwallet/v3/wallet"
	"github.com/decred/dcrd/chaincfg/v3"
)

const (
	defaultGapLimit        = uint32(100)
	defaultAllowHighFees   = false
	defaultRelayFeePerKb   = 1e4
	defaultAccountGapLimit = 10
	defaultManualTickets   = false
	defaultMixSplitLimit   = 10
)

func newWalletConfig(db wallet.DB, chainParams *chaincfg.Params) *wallet.Config {
	return &wallet.Config{
		DB:              db,
		GapLimit:        defaultGapLimit,
		AccountGapLimit: defaultAccountGapLimit,
		ManualTickets:   defaultManualTickets,
		AllowHighFees:   defaultAllowHighFees,
		RelayFee:        defaultRelayFeePerKb,
		Params:          chainParams,
		MixSplitLimit:   defaultMixSplitLimit,
	}
}
