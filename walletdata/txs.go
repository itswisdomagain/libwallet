package walletdata

import (
	"fmt"
	"reflect"

	"github.com/asdine/storm"
	"github.com/asdine/storm/q"
)

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

func (db *DB[T]) TxIndexLastBlock() (int32, error) {
	var lastBlock int32
	err := db.db.Get(metadataBktName, txIndexLastBlockKey, &lastBlock)
	return lastBlock, ignoreStormNotFoundError(err)
}

func (db *DB[T]) SaveTxIndexLastBlock(height int32) error {
	if lastBlock, err := db.TxIndexLastBlock(); err != nil {
		return err
	} else if height < lastBlock {
		return db.RollbackTxIndexLastBlock(height)
	}
	return db.db.Set(metadataBktName, txIndexLastBlockKey, height)
}

func (db *DB[T]) RollbackTxIndexLastBlock(height int32) error {
	// TODO: Use batch tx.
	sampleTx := db.makeEmptyTx()
	if height <= 0 {
		if err := db.db.Drop(sampleTx); err != nil {
			return fmt.Errorf("error deleting invalidated wallet transactions: %s", err.Error())
		}
	} else {
		// TODO: Perform selective deletion based on block height.
	}

	return db.db.Set(metadataBktName, txIndexLastBlockKey, height)
}

// IndexTransaction saves a transaction to the indexed transactions db. If
// updatefn is provided and the tx was previously saved, the updatefn will be
// called to update the new tx with any information from the previously saved
// tx. Returns true if the tx was previously saved.
func (db *DB[T]) IndexTransaction(tx *T, updatefn func(oldTx, newTx *T) (*T, error)) (bool, error) {
	// First check if this tx was previously saved.
	txUniqueKey := reflect.ValueOf(tx).FieldByName(db.uniqueTxFieldName).Interface()
	oldTx, err := db.FindTransaction(db.uniqueTxFieldName, txUniqueKey)
	if err != nil {
		return false, err
	}

	isUpdate := oldTx != nil
	if isUpdate && updatefn != nil {
		tx, err = updatefn(oldTx, tx)
		if err != nil {
			return false, fmt.Errorf("tx update error: %v", err)
		}
	}

	if isUpdate {
		// TODO: Use batch tx.
		err = db.db.DeleteStruct(oldTx)
		if err != nil {
			return false, fmt.Errorf("tx update error: %v", err)
		}
	}

	return isUpdate, db.db.Save(tx)
}

func (db *DB[T]) FindTransaction(fieldName string, fieldValue interface{}) (*T, error) {
	tx := db.makeEmptyTx()
	err := db.db.One(fieldName, fieldValue, tx)
	if err != nil {
		return nil, ignoreStormNotFoundError(err)
	}
	return tx, err
}

func (db *DB[T]) FindTransactions(offset, limit int, sort *SORT, matchers ...q.Matcher) ([]*T, error) {
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

	var txs []*T
	return txs, query.Find(&txs)
}

func (db *DB[T]) CountTransactions(matchers ...q.Matcher) (int, error) {
	sampleTx := db.makeEmptyTx()
	return db.db.Select(matchers...).Count(sampleTx)
}

func ignoreStormNotFoundError(err error) error {
	if err == storm.ErrNotFound {
		return nil
	}
	return err
}
