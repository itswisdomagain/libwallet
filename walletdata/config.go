package walletdata

import (
	"github.com/asdine/storm"
)

const walletConfigBktName = "wallet_config"

// ErrNotFound is the error returned when a requested data is not found in the
// db. It is the same as storm.ErrNotFound but defined here so that callers
// don't need to reference the storm package.
var ErrNotFound = storm.ErrNotFound

// SaveConfigValue saves wallet configuration data as a key-value pair.
func (db *DB[T]) SaveConfigValue(key string, value interface{}) error {
	return db.db.Set(walletConfigBktName, key, value)
}

// ReadConfigValue reads wallet configuration data from the database.
func (db *DB[T]) ReadConfigValue(key string, valueOut interface{}) error {
	return db.db.Get(walletConfigBktName, key, valueOut)
}

// DeleteConfigValue deletes the wallet config data with the specified key.
func (db *DB[T]) DeleteConfigValue(key string) error {
	return db.db.Delete(walletConfigBktName, key)
}
