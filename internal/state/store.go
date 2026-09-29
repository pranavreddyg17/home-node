// Package state owns the transactional management database. Workload bytes do
// not belong here; the database records intent, identity and observed results.
package state

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }

func Open(directory string) (*Store, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("state directory must be a private directory (0700)")
	}
	path, err := filepath.Abs(filepath.Join(directory, "management.db"))
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		_ = file.Close()
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err = os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("database must be a private regular file (0600)")
	}
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", u.String()+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if err = s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) migrate() error {
	return s.Transaction(context.Background(), func(tx *sql.Tx) error {
		var version int
		if err := tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
			return err
		}
		if version > 1 {
			return fmt.Errorf("database schema %d is newer than supported schema 1", version)
		}
		if version == 1 {
			return nil
		}
		_, err := tx.Exec(schema)
		return err
	})
}

// Random is used for opaque identifiers and bearer secrets. Hashes, rather
// than bearer values, are persisted for sessions, invitations and recovery.
func Random() string { return rand.Text() }
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func Event(tx *sql.Tx, actor, kind, object string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO events(actor,kind,object_id,payload,created_at) VALUES(?,?,?,?,?)", actor, kind, object, string(data), time.Now().Unix())
	return err
}

const schema = `
CREATE TABLE identity (singleton INTEGER PRIMARY KEY CHECK(singleton=1), owner_id TEXT NOT NULL, claimed INTEGER NOT NULL DEFAULT 0 CHECK(claimed IN(0,1)), epoch INTEGER NOT NULL DEFAULT 1);
CREATE TABLE devices (id TEXT PRIMARY KEY, name TEXT NOT NULL, capabilities TEXT NOT NULL, created_at INTEGER NOT NULL, revoked_at INTEGER);
CREATE TABLE credentials (id BLOB PRIMARY KEY, device_id TEXT NOT NULL REFERENCES devices(id), data TEXT NOT NULL);
CREATE TABLE sessions (token_hash TEXT PRIMARY KEY, device_id TEXT NOT NULL REFERENCES devices(id), epoch INTEGER NOT NULL, created_at INTEGER NOT NULL, verified_at INTEGER NOT NULL, last_seen INTEGER NOT NULL, expires_at INTEGER NOT NULL);
CREATE INDEX sessions_device ON sessions(device_id);
CREATE TABLE invitations (token_hash TEXT PRIMARY KEY, issuer TEXT REFERENCES devices(id), kind TEXT NOT NULL CHECK(kind IN('setup','pair','recovery')), name TEXT NOT NULL DEFAULT '', capabilities TEXT NOT NULL, epoch INTEGER NOT NULL, expires_at INTEGER NOT NULL);
CREATE TABLE challenges (token_hash TEXT PRIMARY KEY, kind TEXT NOT NULL, payload TEXT NOT NULL, epoch INTEGER NOT NULL, expires_at INTEGER NOT NULL);
CREATE TABLE recovery_codes (token_hash TEXT PRIMARY KEY);
CREATE TABLE events (id INTEGER PRIMARY KEY AUTOINCREMENT, actor TEXT NOT NULL, kind TEXT NOT NULL, object_id TEXT NOT NULL, payload TEXT NOT NULL, created_at INTEGER NOT NULL);
CREATE INDEX events_created ON events(created_at);
CREATE TABLE operations (id TEXT PRIMARY KEY, device_id TEXT NOT NULL REFERENCES devices(id), kind TEXT NOT NULL, state TEXT NOT NULL, request_hash TEXT NOT NULL, idempotency_key TEXT NOT NULL, result TEXT NOT NULL DEFAULT '{}', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, UNIQUE(device_id,idempotency_key));
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
PRAGMA user_version=1;
`
