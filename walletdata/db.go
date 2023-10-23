package walletdata

import (
	"fmt"
	"os"

	"github.com/asdine/storm"
	bolt "go.etcd.io/bbolt"
)

const (
	metadataBktName       = "db_metadata"
	txVersionKey          = "tx_version"
	txLastIndexedBlockKey = "tx_last_indexed_block"
)

// DBTransaction defines methods that must be implemented by the transaction
// struct of an asset.
type DBTransaction interface {
	TxVersion() uint32
}

// DB is a database for storing simple information in key-value pairs for a
// wallet. It can also be optionally used to index a wallet's transactions for
// subsequent quick querying and filtering.
type DB[T DBTransaction] struct {
	db          *storm.DB
	makeEmptyTx func() T
}

// Ensure DB implements UserConfigDB and WalletConfigDB.
var _ UserConfigDB = (*DB[DBTransaction])(nil)
var _ WalletConfigDB = (*DB[DBTransaction])(nil)

// Initialize creates or open a database at the specified path. newTxConstructor
// is optional but must be provided if the database will be used for transaction
// indexing. If provided, newTxConstructor should be a function that can be used
// to initialize an empty transaction object. If the transaction object created
// by the newTxConstructor returns a TxVersion that is different from the
// version last used by this database, the transactions index will be dropped
// and the wallet's transactions will need to be re-indexed.
func Initialize[T DBTransaction](dbPath string, newTxConstructor func() T) (*DB[T], error) {
	if newTxConstructor == nil {
		db, _, err := openOrCreateDB(dbPath, 0)
		if err != nil {
			return nil, err
		}
		return &DB[T]{db: db}, nil
	}

	sampleTx := newTxConstructor()
	latestTxVersion := sampleTx.TxVersion()
	db, dbTxVersion, err := openOrCreateDB(dbPath, latestTxVersion)
	if err != nil {
		return nil, err
	}

	if dbTxVersion != latestTxVersion {
		// Delete previously indexed transactions.
		if err = db.Drop(sampleTx); err != nil {
			return nil, fmt.Errorf("error deleting outdated wallet transactions: %s", err.Error())
		}
		// Reset the last indexed block value, so the db consumer knows to
		// re-index transactions from scratch.
		if err = db.Set(metadataBktName, txLastIndexedBlockKey, 0); err != nil {
			return nil, fmt.Errorf("error updating txVersion: %s", err.Error())
		}
		// This db is now on the latest tx version.
		if err = db.Set(metadataBktName, txVersionKey, latestTxVersion); err != nil {
			return nil, fmt.Errorf("error updating txVersion: %s", err.Error())
		}
	}

	// Initialize the tx index bucket.
	err = db.Init(sampleTx)
	if err != nil {
		return nil, fmt.Errorf("error initializing tx index bucket: %s", err.Error())
	}

	return &DB[T]{db: db}, nil
}

// openOrCreateDB checks if a db file exists at the specified path, opens it and
// returns the txVersion saved in the database. If the file does not exist, it
// is created and the latestTxVersion is saved as the newly created database's
// txVersion.
func openOrCreateDB(dbPath string, latestTxVersion uint32) (*storm.DB, uint32, error) {
	// First check if a file exists at dbPath; if it does not already exist,
	// we'll need to create it and set the txVersion to the latestTxVersion.
	var isNewDbFile bool
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			isNewDbFile = true
		} else {
			return nil, 0, fmt.Errorf("error checking db file path: %w", err)
		}
	}

	db, err := storm.Open(dbPath)
	if err != nil {
		switch err {
		case bolt.ErrTimeout: // storm failed to acquire a lock on the db file
			return nil, 0, fmt.Errorf("database is in use by another process")
		default:
			return nil, 0, fmt.Errorf("open db error: %w", err)
		}
	}

	var dbTxVersion uint32
	if isNewDbFile {
		dbTxVersion = latestTxVersion
		err = db.Set(metadataBktName, txVersionKey, latestTxVersion)
		if err != nil {
			os.RemoveAll(dbPath)
			return nil, 0, fmt.Errorf("error saving txVersion for new database: %v", err)
		}
	} else {
		err := db.Get(metadataBktName, txVersionKey, &dbTxVersion)
		if err != nil && err != storm.ErrNotFound { // not found is ok, means very old db
			return nil, 0, fmt.Errorf("error checking database txVersion: %w", err)
		}
	}

	return db, dbTxVersion, nil
}
