package db

import (
	"context"
	"fmt"
)

// execTx executes a function within a database transaction.
func (store *SQLStore) execTx(ctx context.Context, fn func(*Queries) error) error {
	tx, err := store.connPoll.Begin(ctx)

	if err != nil {
		return fmt.Errorf("failed to begin transactionn %w", err)
	}

	q := New(tx)

	if err = fn(q); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx err %v,rollback err %v", rbErr, err)
		}
		return err
	}
	return tx.Commit(ctx)
}
