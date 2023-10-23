package walletdata

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/asdine/storm"
	bolt "go.etcd.io/bbolt"
)

const (
	metadataBktName     = "db_metadata"
	txVersionKey        = "tx_version"
	txIndexLastBlockKey = "tx_index_last_block"
)

// DB is a storm-backed database for storing simple information in key-value
// pairs for a wallet. It can also be optionally used to index a wallet's
// transactions for subsequent quick querying and filtering.
type DB[T any] struct {
	db *storm.DB

	makeEmptyTx       func() *T
	txVersion         uint32
	uniqueTxFieldName string
}

// Ensure DB implements UserConfigDB, WalletConfigDB and TxIndexDB.
var _ UserConfigDB = (*DB[struct{}])(nil)
var _ WalletConfigDB = (*DB[struct{}])(nil)
var _ TxIndexDB[struct{}] = (*DB[struct{}])(nil)

// Initialize creates or open a database at the specified path. newTxConstructor
// is optional but must be provided if the database will be used for transaction
// indexing. If provided, newTxConstructor should be a function that can be used
// to initialize an empty transaction object. If the transaction object created
// by the newTxConstructor returns a TxVersion that is different from the
// version last used by this database, the transactions index will be dropped
// and the wallet's transactions will need to be re-indexed.
func Initialize[T any](dbPath string, newTxConstructor func() T, latestTxVersion uint32) (*DB[T], error) {
	if newTxConstructor == nil {
		db, _, err := openOrCreateDB(dbPath, 0)
		if err != nil {
			return nil, err
		}
		return &DB[T]{db: db}, nil
	}

	db, dbTxVersion, err := openOrCreateDB(dbPath, latestTxVersion)
	if err != nil {
		return nil, err
	}

	if dbTxVersion != latestTxVersion {
		// Delete previously indexed transactions.
		sampleTx := newTxConstructor()
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

	// The uniqueTxField will be used for checking for existing transactions.
	// TODO: Also read all indexed fields and check that all field names used in
	// tx lookups are indexed. Additionally, if the list of indexed fields found
	// now differs from what was last used, reindex the database.
	sampleTx := newTxConstructor()
	uniqueTxField, ok := uniqueFieldName(sampleTx)
	if !ok {
		return nil, fmt.Errorf("tx object does not have a unique field")
	}

	// Initialize the tx index bucket.
	err = db.Init(sampleTx)
	if err != nil {
		return nil, fmt.Errorf("error initializing tx index bucket: %s", err.Error())
	}

	return &DB[T]{
		db:                db,
		txVersion:         latestTxVersion,
		uniqueTxFieldName: uniqueTxField,
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

func uniqueFieldName(elem any) (string, bool) {
	elemType := reflect.TypeOf(elem)
	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}
	nFields := elemType.NumField()
	for i := 0; i < nFields; i++ {
		field := elemType.Field(i)
		if strings.Index(field.Tag.Get("storm"), "unique") > 0 {
			return field.Name, true
		}
	}
	return "", false
}
