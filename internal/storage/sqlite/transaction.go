package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type transactionContextKey struct{}

const processedTelegramUpdateRetention = 10_000

type databaseRunner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func transactionFromContext(ctx context.Context) *sql.Tx {
	if ctx == nil {
		return nil
	}
	tx, _ := ctx.Value(transactionContextKey{}).(*sql.Tx)
	return tx
}

func (s *Store) runner(ctx context.Context) databaseRunner {
	if tx := transactionFromContext(ctx); tx != nil {
		return tx
	}
	return s.db
}

func (s *Store) withTransaction(ctx context.Context, operation string, fn func(context.Context, *sql.Tx) error) error {
	if tx := transactionFromContext(ctx); tx != nil {
		return fn(ctx, tx)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start %s transaction: %w", operation, err)
	}
	defer tx.Rollback()

	txCtx := context.WithValue(ctx, transactionContextKey{}, tx)
	if err := fn(txCtx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s transaction: %w", operation, err)
	}
	return nil
}

// ProcessTelegramUpdate runs one update atomically with SQLite mutations made
// through this Store. Duplicate update IDs are acknowledged without rerunning.
func (s *Store) ProcessTelegramUpdate(ctx context.Context, updateID int64, handler func(context.Context) error) (bool, error) {
	if updateID <= 0 {
		return false, fmt.Errorf("telegram update ID must be positive")
	}
	if handler == nil {
		return false, fmt.Errorf("telegram update handler is nil")
	}

	processed := false
	err := s.withTransaction(ctx, "Telegram update", func(txCtx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(txCtx, `
			INSERT OR IGNORE INTO processed_telegram_updates (update_id, processed_at)
			VALUES (?, ?)
		`, updateID, time.Now().UTC().Unix())
		if err != nil {
			return fmt.Errorf("claim Telegram update: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check Telegram update claim: %w", err)
		}
		if rows == 0 {
			return nil
		}
		if err := handler(txCtx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(txCtx, `
			DELETE FROM processed_telegram_updates
			WHERE update_id < COALESCE((
				SELECT update_id
				FROM processed_telegram_updates
				ORDER BY update_id DESC
				LIMIT 1 OFFSET ?
			), -1)
		`, processedTelegramUpdateRetention-1); err != nil {
			return fmt.Errorf("prune processed Telegram updates: %w", err)
		}
		processed = true
		return nil
	})
	return processed, err
}
