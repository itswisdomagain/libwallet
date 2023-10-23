package walletdata

import (
	"github.com/asdine/storm"
)

const (
	userConfigBktName   = "user_config"
	walletConfigBktName = "wallet_config"
)

// ErrNotFound is the error returned when a requested data is not found in the
// db. It is the same as storm.ErrNotFound but defined here so that callers
// don't need to reference the storm package.
var ErrNotFound = storm.ErrNotFound

// SaveUserConfigValue saves user configuration data as a key-value pair.
func (db *DB[T]) SaveUserConfigValue(key string, value interface{}) error {
	return db.db.Set(userConfigBktName, key, value)
}

// ReadUserConfigValue reads user configuration data from the database.
func (db *DB[T]) ReadUserConfigValue(key string, valueOut interface{}) error {
	return db.db.Get(userConfigBktName, key, valueOut)
}

// DeleteUserConfigValue deletes the user config data with the specified key.
func (db *DB[T]) DeleteUserConfigValue(key string) error {
	return db.db.Delete(userConfigBktName, key)
}

// SaveWalletConfigValue saves wallet configuration data as a key-value pair.
func (db *DB[T]) SaveWalletConfigValue(key string, value interface{}) error {
	return db.db.Set(walletConfigBktName, key, value)
}

// ReadWalletConfigValue reads wallet configuration data from the database.
func (db *DB[T]) ReadWalletConfigValue(key string, valueOut interface{}) error {
	return db.db.Get(walletConfigBktName, key, valueOut)
}

// DeleteWalletConfigValue deletes the wallet config data with the specified key.
func (db *DB[T]) DeleteWalletConfigValue(key string) error {
	return db.db.Delete(walletConfigBktName, key)
}
