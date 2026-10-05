package state

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryRebindFreshIdentityAndExactRetry(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "source"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err = source.DB.Exec("INSERT INTO identity(singleton,owner_id,claimed,epoch) VALUES(1,?,1,7)", Random()); err != nil {
		t.Fatal(err)
	}
	oldInstance := Random()
	if _, err = source.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at) VALUES('files',?,'running',1)", oldInstance); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir()
	if err = os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.RecoverySnapshot(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", snapshot+"?_pragma=foreign_keys(1)&_pragma=trusted_schema(OFF)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	newOwner, newInstance := Random(), Random()
	run := func(owner string, epoch int64, mapping map[string]string) error {
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err = RebindRecoveryTx(context.Background(), tx, owner, epoch, mapping); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err = run(newOwner, 9, map[string]string{"files": oldInstance}); err == nil {
		t.Fatal("old disk identity reused")
	}
	if err = run(newOwner, 8, map[string]string{"files": newInstance}); err == nil {
		t.Fatal("old recovery epoch reused")
	}
	if err = run(newOwner, 9, map[string]string{"files": newInstance}); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err = db.QueryRow("SELECT revision FROM apps WHERE workload='files'").Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err = run(newOwner, 9, map[string]string{"files": newInstance}); err != nil {
		t.Fatal("exact retry refused", err)
	}
	var afterRevision int64
	var instance, status string
	if err = db.QueryRow("SELECT instance_id,state,revision FROM apps WHERE workload='files'").Scan(&instance, &status, &afterRevision); err != nil || instance != newInstance || status != "stopped" || afterRevision != revision {
		t.Fatal("retry changed app binding", err)
	}
	var owner string
	var epoch, claimed int64
	if err = db.QueryRow("SELECT owner_id,epoch,claimed FROM identity").Scan(&owner, &epoch, &claimed); err != nil || owner != newOwner || epoch != 9 || claimed != 0 {
		t.Fatal("fresh recovery identity differs", err)
	}
	if err = run(newOwner, 9, map[string]string{"files": Random()}); err == nil {
		t.Fatal("contradictory retry accepted")
	}
}
