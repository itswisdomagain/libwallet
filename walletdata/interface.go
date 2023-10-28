package walletdata

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
