package asset

import (
	"time"

	"github.com/decred/slog"
	"github.com/itswisdomagain/libwallet/walletdata"
)

// CreateWalletParams are the parameters for opening a wallet.
type OpenWalletParams[Tx any] struct {
	Net            Network
	DataDir        string
	DbDriver       string
	Logger         slog.Logger
	UserConfigDB   walletdata.UserConfigDB
	WalletConfigDB walletdata.WalletConfigDB

	// TransformTx is only required if transaction indexing and/or transaction
	// notifications is/are desired. Can be nil otherwise.
	TransformTx func(blockHeight int32, tx any, network Network) (*Tx, error)
	// TxIndexDB is only required if transaction indexing is desired. Can be nil
	// otherwise.
	TxIndexDB walletdata.TxIndexDB[Tx]
}

// CreateWalletParams are the parameters for creating a wallet.
type CreateWalletParams[Tx any] struct {
	OpenWalletParams[Tx]
	Pass     []byte
	Birthday time.Time
}

// RecoveryCfg is the information used to recover a wallet.
type RecoveryCfg struct {
	Seed                 []byte
	NumExternalAddresses uint32
	NumInternalAddresses uint32
}
