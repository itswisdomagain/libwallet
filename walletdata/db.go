package walletdata

import (
	"fmt"

	"github.com/asdine/storm"
	bolt "go.etcd.io/bbolt"
)

// DB is a storm-backed database for storing simple information in key-value
// pairs for a wallet.
type DB struct {
	db *storm.DB
}

// Ensure DB implements UserConfigDB and WalletConfigDB.
var _ UserConfigDB = (*DB)(nil)
var _ WalletConfigDB = (*DB)(nil)

// Initialize creates or open a database at the specified path.
func Initialize(dbPath string) (*DB, error) {
	db, err := storm.Open(dbPath)
	if err != nil {
		switch err {
		case bolt.ErrTimeout: // storm failed to acquire a lock on the db file
			return nil, fmt.Errorf("database is in use by another process")
		default:
			return nil, fmt.Errorf("open db error: %w", err)
		}
	}

	return &DB{db: db}, nil
}
