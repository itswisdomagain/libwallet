package asset

import (
	"time"

	"github.com/decred/slog"
)

// CreateWalletParams are the parameters for opening a wallet.
type OpenWalletParams struct {
	Net      Network
	DataDir  string
	DbDriver string
	Logger   slog.Logger
}

// CreateWalletParams are the parameters for creating a wallet.
type CreateWalletParams struct {
	OpenWalletParams
	Pass     []byte
	Birthday time.Time
}

// RecoveryCfg is the information used to recover a wallet.
type RecoveryCfg struct {
	Seed                 []byte
	NumExternalAddresses uint32
	NumInternalAddresses uint32
}
