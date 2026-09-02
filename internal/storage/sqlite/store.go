package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store owns one SQLite connection pool configured for serialized access.
type Store struct {
	db      *sql.DB
	timeout time.Duration
}

const dsn = "file:%s?mode=rwc&_foreign_keys=1&_journal=WAL&_busy_timeout=5000"

// New opens SQLite, enables required PRAGMAs, and verifies connectivity.
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
	if dbPath != ":memory:" && !strings.HasPrefix(dbPath, "file::memory:") {
		if err := os.Chmod(dbPath, 0o600); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("%w: secure database permissions: %w", ErrOpenConnection, err)
		}
	}

	return &Store{db: conn, timeout: dbTimeout}, nil
}

// Close releases the SQLite connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}
