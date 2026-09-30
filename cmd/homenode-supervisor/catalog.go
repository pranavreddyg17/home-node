package main

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
)

// The observed version and its advancement are one transaction. Concurrent
// SQLite writers cannot replace a newer committed floor from a stale snapshot;
// a competing transaction fails closed rather than retrying with stale trust.
func acceptCatalog(ctx context.Context, db *sql.DB, data []byte, key ed25519.PublicKey, configured int64, now time.Time) (catalog.Manifest, error) {
	var empty catalog.Manifest
	if configured < 1 {
		return empty, catalog.ErrUntrusted
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	minimum := configured
	var stored string
	err = tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='catalog-version'").Scan(&stored)
	if err == nil {
		minimum, err = raiseCatalogFloor(configured, stored)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	manifest, err := catalog.Verify(data, map[string]ed25519.PublicKey{catalog.KeyID(key): key}, minimum, now)
	if err != nil {
		return empty, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('catalog-version',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", strconv.FormatInt(manifest.Version, 10)); err != nil {
		return empty, err
	}
	if err = tx.Commit(); err != nil {
		return empty, err
	}
	return manifest, nil
}
