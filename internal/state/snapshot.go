package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
)

// RecoverySnapshot makes a consistent standalone management snapshot and removes
// identity trust. App disks must be stopped and backed up by maintenance at the
// same boundary; this function alone is not a complete recoverable backup set.
func (s *Store) RecoverySnapshot(ctx context.Context, directory string) (_ string, resultErr error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("snapshot staging directory must be private")
	}
	path, err := filepath.Abs(filepath.Join(directory, "snapshot.db"))
	if err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	defer func() {
		if resultErr != nil {
			_ = os.Remove(path)
		}
	}()
	deadline, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	conn, err := s.DB.Conn(deadline)
	if err != nil {
		return "", err
	}
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	err = conn.Raw(func(raw any) (err error) {
		copier, ok := raw.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("SQLite backup API unavailable")
		}
		backup, err := copier.NewBackup(uri)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, backup.Finish()) }()
		for {
			if err = deadline.Err(); err != nil {
				return err
			}
			more, e := backup.Step(256)
			if e != nil {
				return e
			}
			if !more {
				return nil
			}
		}
	})
	err = errors.Join(err, conn.Close())
	if err != nil {
		return "", err
	}
	db, err := sql.Open("sqlite", uri+"?_pragma=foreign_keys(1)&_pragma=journal_mode(DELETE)&_pragma=synchronous(FULL)&_pragma=secure_delete(ON)")
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
	clean := &Store{DB: db}
	err = clean.Transaction(deadline, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(deadline, `DELETE FROM sessions; DELETE FROM credentials;
DELETE FROM invitations; DELETE FROM challenges; DELETE FROM recovery_codes;
UPDATE devices SET capabilities='[]',revoked_at=coalesce(revoked_at,unixepoch());
UPDATE identity SET claimed=0,epoch=epoch+1,owner_id=?;
DELETE FROM settings WHERE key='origin';
DELETE FROM settings WHERE key='host.maintenance';
DELETE FROM settings WHERE key GLOB 'host.maintenance-job.*';
DELETE FROM settings WHERE key GLOB 'host.activity.*';
DELETE FROM settings WHERE key GLOB 'job.cleanup.*';
UPDATE jobs SET start_requested=0;
UPDATE transfers SET state='cancelling' WHERE state IN('uploading','verifying');
UPDATE jobs SET state='interrupted',error_code='RECOVERY_REQUIRED' WHERE state IN('queued','preparing','running','finalizing','cancelling');
UPDATE generations SET state='interrupted' WHERE state IN('queued','staging','running','cancelling');
UPDATE operations SET state='interrupted',result='{}',updated_at=unixepoch() WHERE state IN('pending','executing','requires-action');
UPDATE apps SET state='stopped',operation_id=NULL,revision=revision+1;`, Random()+Random())
		return err
	})
	if err != nil {
		return "", err
	}
	// Compact away freed pages in the recovery copy, including removed trust
	// records. This is not a promise of erasure on the original storage device.
	if _, err = db.ExecContext(deadline, "VACUUM"); err != nil {
		return "", err
	}
	var check string
	if err = db.QueryRowContext(deadline, "PRAGMA integrity_check").Scan(&check); err != nil {
		return "", err
	}
	if check != "ok" {
		return "", fmt.Errorf("snapshot integrity check failed")
	}
	// Close before syncing so the returned file requires no live WAL connection.
	if err = db.Close(); err != nil {
		return "", err
	}
	file, err = os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	err = errors.Join(file.Sync(), file.Close())
	if err != nil {
		return "", err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return "", err
	}
	err = errors.Join(dir.Sync(), dir.Close())
	if err != nil {
		return "", err
	}
	return path, nil
}
