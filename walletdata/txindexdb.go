package walletdata

import (
	"fmt"
	"reflect"

	"github.com/asdine/storm"
	"github.com/asdine/storm/q"
)

var ErrTxIndexNotSupported = fmt.Errorf("database isn't configured for tx indexing")

// TxIndexDBConfig contains properties that are necessary for transaction
// indexing.
type TxIndexDBConfig[Tx any] struct {
	txVersion          uint32
	txIDField          string
	txBlockHeightField string
	makeEmptyTx        func() *Tx
	txUpdateHook       func(oldTx, newTx *Tx) (*Tx, error)
}

// NewTxIndexDBConfig creates a TxIndexDBConfig.
func NewTxIndexDBConfig[Tx any](txVersion uint32, txIDField, txBlockHeightField string, makeEmptyTx func() *Tx, txUpdateHook func(oldTx, newTx *Tx) (*Tx, error)) *TxIndexDBConfig[Tx] {
	return &TxIndexDBConfig[Tx]{
		txVersion:          txVersion,
		txIDField:          txIDField,
		txBlockHeightField: txBlockHeightField,
		makeEmptyTx:        makeEmptyTx,
		txUpdateHook:       txUpdateHook,
	}
}

type SORT struct {
	fieldName string
	reversed  bool
}

func SortAscending(fieldName string) *SORT {
	return &SORT{fieldName: fieldName}
}

func SortDescending(fieldName string) *SORT {
	return &SORT{fieldName: fieldName, reversed: true}
}

// TxIndexLastBlock returns the highest block height for which transactions are
// indexed.
func (db *DB[Tx]) TxIndexLastBlock() (int32, error) {
	if db.txIndexCfg == nil {
		return -1, ErrTxIndexNotSupported
	}

	var lastBlock int32
	err := db.db.Get(metadataBktName, txIndexLastBlockKey, &lastBlock)
	return lastBlock, ignoreStormNotFoundError(err)
}

// SaveTxIndexLastBlock saves the specified height as the last block height for
// which transactions are indexed. Subsequent tx indexing should start from this
// height+1.
func (db *DB[Tx]) SaveTxIndexLastBlock(height int32) error {
	if db.txIndexCfg == nil {
		return ErrTxIndexNotSupported
	}

	if lastBlock, err := db.TxIndexLastBlock(); err != nil {
		return err
	} else if height < lastBlock {
		return fmt.Errorf("current last block is %d, use rollback to change to %d", lastBlock, height)
	}

	return db.db.Set(metadataBktName, txIndexLastBlockKey, height)
}

// RollbackTxIndexLastBlock is like SaveTxIndexLastBlock but it also deletes all
// previously indexed transactions whose block heights are above the specified
// height to allow subsequent re-indexing from the specified height+1.
func (db *DB[Tx]) RollbackTxIndexLastBlock(height int32) error {
	if db.txIndexCfg == nil {
		return ErrTxIndexNotSupported
	}

	batchTx, err := db.db.Begin(true)
	if err != nil {
		return fmt.Errorf("database error: %v", err)
	}

	defer batchTx.Rollback()

	sampleTx := db.txIndexCfg.makeEmptyTx()
	if height <= 0 {
		err = batchTx.Drop(sampleTx)
	} else {
		err = batchTx.Select(q.Gt(db.txIndexCfg.txBlockHeightField, height)).Delete(sampleTx)
	}
	if err != nil {
		return fmt.Errorf("error deleting invalidated wallet transactions: %s", err.Error())
	}

	err = batchTx.Set(metadataBktName, txIndexLastBlockKey, height)
	if err != nil {
		return err
	}

	if err = batchTx.Commit(); err != nil {
		return fmt.Errorf("database error: %v", err)
	}

	return nil
}

// IndexTransaction saves a transaction to the indexed transactions db. Returns
// true if the tx was previously saved.
func (db *DB[Tx]) IndexTransaction(tx *Tx) (bool, error) {
	if db.txIndexCfg == nil {
		return false, ErrTxIndexNotSupported
	}

	// First check if this tx was previously saved.
	txID := reflect.ValueOf(tx).Elem().FieldByName(db.txIndexCfg.txIDField).Interface()
	oldTx, err := db.FindTransaction(db.txIndexCfg.txIDField, txID)
	if err != nil {
		return false, err
	}

	batchTx, err := db.db.Begin(true)
	if err != nil {
		return false, fmt.Errorf("database error: %v", err)
	}

	defer batchTx.Rollback()

	isUpdate := oldTx != nil
	if isUpdate && db.txIndexCfg.txUpdateHook != nil {
		tx, err = db.txIndexCfg.txUpdateHook(oldTx, tx)
		if err != nil {
			return false, fmt.Errorf("tx update error: %v", err)
		}

		err = batchTx.DeleteStruct(oldTx)
		if err != nil {
			return false, fmt.Errorf("tx update error: %v", err)
		}
	}

	if err = batchTx.Save(tx); err != nil {
		return false, err
	}

	if err = batchTx.Commit(); err != nil {
		return false, fmt.Errorf("database error: %v", err)
	}

	return isUpdate, nil
}

// FindTransaction looks up a transaction that has the specified value in the
// specified field. It's not an error if no transaction is found to match this
// criteria, instead a nil tx and a nil error are returned.
func (db *DB[Tx]) FindTransaction(fieldName string, fieldValue interface{}) (*Tx, error) {
	if db.txIndexCfg == nil {
		return nil, ErrTxIndexNotSupported
	}

	tx := db.txIndexCfg.makeEmptyTx()
	err := db.db.One(fieldName, fieldValue, tx)
	if err != nil {
		return nil, ignoreStormNotFoundError(err)
	}
	return tx, err
}

// FindTransactions looks up and returns transactions that match the specified
// criteria and in the order specified by the sort argument. It is not an error
// if no transaction is found to match the provided criteria, instead an empty
// tx list and a nil error are returned. If no matcher is passed, all indexed
// transactions will be returned.
func (db *DB[Tx]) FindTransactions(offset, limit int, sort *SORT, matchers ...q.Matcher) ([]*Tx, error) {
	if db.txIndexCfg == nil {
		return nil, ErrTxIndexNotSupported
	}

	query := db.db.Select(matchers...)
	if offset > 0 {
		query = query.Skip(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	if sort != nil && sort.reversed {
		query = query.OrderBy(sort.fieldName).Reverse()
	} else if sort != nil {
		query = query.OrderBy(sort.fieldName)
	}

	var txs []*Tx
	if err := query.Find(&txs); err != nil && err != storm.ErrNotFound {
		return nil, err
	}

	return txs, nil
}

// CountTransactions returns the number of transactions that match the specified
// criteria.
func (db *DB[Tx]) CountTransactions(matchers ...q.Matcher) (int, error) {
	if db.txIndexCfg == nil {
		return -1, ErrTxIndexNotSupported
	}

	sampleTx := db.txIndexCfg.makeEmptyTx()
	return db.db.Select(matchers...).Count(sampleTx)
}

func ignoreStormNotFoundError(err error) error {
	if err == storm.ErrNotFound {
		return nil
	}
	return err
}
