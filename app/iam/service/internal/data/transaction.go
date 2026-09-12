package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	entmodel "github.com/Servora-Kit/plateau/app/iam/service/internal/data/ent"
)

// inTx executes repository work atomically and rolls back on errors or panic.
func inTx(ctx context.Context, client *entmodel.Client, fn func(*entmodel.Tx) error) (err error) {
	if fn == nil {
		return fmt.Errorf("transaction function is nil")
	}
	tx, err := client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin IAM transaction: %w", err)
	}
	defer func() {
		if panicValue := recover(); panicValue != nil {
			_ = tx.Rollback()
			panic(panicValue)
		}
	}()
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit IAM transaction: %w", err)
	}
	return nil
}
