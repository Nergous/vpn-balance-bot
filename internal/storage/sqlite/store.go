package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	timeout time.Duration
}

const dsn = "file:%s?mode=rwc&_foreign_keys=1&_journal=WAL&_busy_timeout=5000"

func New(ctx context.Context, dbPath string, dbTimeout time.Duration) (*Store, error) {
	conn, err := sql.Open("sqlite", fmt.Sprintf(dsn, dbPath))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOpenConnection, err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	pingCtx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	if err := conn.PingContext(pingCtx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("%w: %w", ErrOpenConnection, err)
	}

	return &Store{db: conn, timeout: dbTimeout}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
