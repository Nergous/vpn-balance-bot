package testutil

import (
	"context"
	"testing"
)

func TestNewSQLite(t *testing.T) {
	store := NewSQLite(t)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
}
