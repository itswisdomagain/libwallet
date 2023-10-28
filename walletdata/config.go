package walletdata

const (
	userConfigBktName   = "user_config"
	walletConfigBktName = "wallet_config"
)

// SaveUserConfigValue saves user configuration data as a key-value pair.
func (db *DB) SaveUserConfigValue(key string, value interface{}) error {
	return db.db.Set(userConfigBktName, key, value)
}

// ReadUserConfigValue reads user configuration data from the database.
func (db *DB) ReadUserConfigValue(key string, valueOut interface{}) error {
	return db.db.Get(userConfigBktName, key, valueOut)
}

// DeleteUserConfigValue deletes the user config data with the specified key.
func (db *DB) DeleteUserConfigValue(key string) error {
	return db.db.Delete(userConfigBktName, key)
}

// SaveWalletConfigValue saves wallet configuration data as a key-value pair.
func (db *DB) SaveWalletConfigValue(key string, value interface{}) error {
	return db.db.Set(walletConfigBktName, key, value)
}

// ReadWalletConfigValue reads wallet configuration data from the database.
func (db *DB) ReadWalletConfigValue(key string, valueOut interface{}) error {
	return db.db.Get(walletConfigBktName, key, valueOut)
}

// DeleteWalletConfigValue deletes the wallet config data with the specified
// key.
func (db *DB) DeleteWalletConfigValue(key string) error {
	return db.db.Delete(walletConfigBktName, key)
}
