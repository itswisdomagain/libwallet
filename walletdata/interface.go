package walletdata

import "github.com/asdine/storm/q"

// UserConfigDB defines methods for writing and reading user config information
// to/from a persistent data store.
type UserConfigDB interface {
	SaveUserConfigValue(key string, value interface{}) error
	ReadUserConfigValue(key string, valueOut interface{}) error
	DeleteUserConfigValue(key string) error
}

// WalletConfigDB defines methods for writing and reading wallet config
// information to/from a persistent data store.
type WalletConfigDB interface {
	SaveWalletConfigValue(key string, value interface{}) error
	ReadWalletConfigValue(key string, valueOut interface{}) error
	DeleteWalletConfigValue(key string) error
}

// TxIndexDB defines methods for saving and reading transactions to/from an
// indexed database.
type TxIndexDB[T any] interface {
	TxIndexLastBlock() (int32, error)
	SaveTxIndexLastBlock(height int32) error
	RollbackTxIndexLastBlock(height int32) error
	IndexTransaction(tx *T) (bool, error)
	FindTransaction(fieldName string, fieldValue interface{}) (*T, error)
	FindTransactions(offset, limit int, sort *SORT, matchers ...q.Matcher) ([]*T, error)
	CountTransactions(matchers ...q.Matcher) (int, error)
}
