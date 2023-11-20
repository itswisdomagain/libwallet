package walletdata

import (
	"fmt"
	"os"

	"github.com/asdine/storm"
	bolt "go.etcd.io/bbolt"
)

const (
	metadataBktName     = "db_metadata"
	txVersionKey        = "tx_version"
	txIndexLastBlockKey = "tx_index_last_block"
)

type privateTxIndexDBConfig[Tx any] TxIndexDBConfig[Tx]

// DB is a storm-backed database for storing simple information in key-value
// pairs for a wallet. It can also be optionally used to index a wallet's
// transactions for subsequent quick querying and filtering.
type DB[Tx any] struct {
	db *storm.DB

	// txIndexCfg may be nil, if the database is not intended to be used for tx
	// indexing.
	txIndexCfg *TxIndexDBConfig[Tx]
}

// Ensure DB implements UserConfigDB, WalletConfigDB and TxIndexDB.
var _ UserConfigDB = (*DB[struct{}])(nil)
var _ WalletConfigDB = (*DB[struct{}])(nil)
var _ TxIndexDB[struct{}] = (*DB[struct{}])(nil)

// Initialize creates or open a database at the specified path. txIndexCfg is
// optional but must be provided if the database will be used for transaction
// indexing. If txIndexCfg.txVersion is different from the version last used by
// this database, the transactions index will be dropped and the wallet's
// transactions will need to be re-indexed.
func Initialize[Tx any](dbPath string, txIndexCfg *TxIndexDBConfig[Tx]) (*DB[Tx], error) {
	if txIndexCfg == nil {
		db, _, err := openOrCreateDB(dbPath, 0)
		if err != nil {
			return nil, err
		}
		return &DB[Tx]{db: db}, nil
	}

	sampleTx := txIndexCfg.makeEmptyTx()

	// TODO: Use reflection to verify that the provided txIndexCfg.uniqueTxField
	// has a `storm:"unique"` tag and to validate the txIndexCfg.txHeightField.
	// Also read all indexed fields into a slice and log a warning if tx lookup
	// is performed using a field that is not in the indexed fields slice.
	// Additionally, if the list of indexed fields found now differs from what
	// was last used, reindex the database.

	latestTxVersion := txIndexCfg.txVersion
	db, dbTxVersion, err := openOrCreateDB(dbPath, latestTxVersion)
	if err != nil {
		return nil, err
	}

	if dbTxVersion != latestTxVersion {
		// Delete previously indexed transactions.
		if err = db.Drop(sampleTx); err != nil {
			return nil, fmt.Errorf("error deleting outdated wallet transactions: %s", err.Error())
		}
		// Reset the tx index last block value, so the db consumer knows to
		// re-index transactions from scratch.
		if err = db.Set(metadataBktName, txIndexLastBlockKey, 0); err != nil {
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

	return &DB[Tx]{
		db:         db,
		txIndexCfg: txIndexCfg,
	}, nil
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
