package state

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoverySnapshotIncludesCommittedWALAndExcludesTrust(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "source"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	device, credential, token := Random(), "PRIVATE-CREDENTIAL-MARKER-"+Random(), "PRIVATE-SESSION-MARKER-"+Random()
	conversation, deletionOp, approvalMarker := Random(), Random(), "PRIVATE-APPROVAL-MARKER-"+Random()
	deletionPayload := `{"conversationId":"` + conversation + `","attempts":3,"nextAttemptAt":9999999999}`
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO identity(singleton,owner_id,claimed,epoch) VALUES(1,'original-owner',1,7)`, nil},
		{`INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'owner','["admin"]',1)`, []any{device}},
		{`INSERT INTO credentials(id,device_id,data) VALUES(?,?,?)`, []any{[]byte("id"), device, credential}},
		{`INSERT INTO sessions VALUES(?,?,7,1,1,1,9999999999)`, []any{token, device}},
		{`INSERT INTO settings VALUES('origin','https://old-host.example')`, nil},
		{`INSERT INTO settings VALUES('host.maintenance','active-source-barrier')`, nil},
		{`INSERT INTO settings VALUES('host.backup-publication','source-publication-intent')`, nil},
		{`INSERT INTO settings VALUES('host.backup-dispatch','uncertain:source-dispatch-intent')`, nil},
		{`INSERT INTO settings VALUES('host.backup-outcome.current','source-outcome')`, nil},
		{`INSERT INTO settings VALUES('host.backup-outcome.last-success','source-success')`, nil},
		{`INSERT INTO settings VALUES('host.maintenance-job.root-token','source-root-token')`, nil},
		{`INSERT INTO settings VALUES('host.activity.fixture','trash-expiry')`, nil},
		{`INSERT INTO settings VALUES('retained-config','committed-WAL-value')`, nil},
		{`INSERT INTO settings VALUES('job.cleanup.old-attempt','{"state":"pending","lastAttempt":0}')`, nil},
		{`INSERT INTO conversations(id,title,created_at) VALUES(?,'retained conversation',1)`, []any{conversation}},
		{`INSERT INTO operations(id,device_id,kind,state,request_hash,idempotency_key,result,created_at,updated_at) VALUES(?,?,'conversation.delete','pending',?,'delete-key',?,1,1)`, []any{deletionOp, device, Hash("delete request"), deletionPayload}},
		{`INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,'approval-grant',?,7,9999999999)`, []any{Hash("grant"), approvalMarker}},
		{`INSERT INTO challenges(token_hash,kind,payload,epoch,expires_at) VALUES(?,'approval',?,7,9999999999)`, []any{Hash("ceremony"), approvalMarker}},
		{`INSERT INTO apps(workload,instance_id,state,updated_at,revision) VALUES('files',?,'running',1,9)`, []any{Random()}},
		{`INSERT INTO transfers(id,device_id,name,size,sha256,state,created_at,expires_at) VALUES(?,?,'unfinished',1,?,'uploading',1,9999999999)`, []any{Random(), device, Hash("content")}},
	} {
		if _, err = s.DB.Exec(statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	staging := filepath.Join(t.TempDir(), "snapshot")
	if err = os.Mkdir(staging, 0700); err != nil {
		t.Fatal(err)
	}
	path, err := s.RecoverySnapshot(context.Background(), staging)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("snapshot permissions", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(credential)) || bytes.Contains(data, []byte(token)) || bytes.Contains(data, []byte(approvalMarker)) || bytes.Contains(data, []byte("source-root-token")) {
		t.Fatal("removed secrets remain in snapshot pages")
	}
	snapshotFile, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	apps, err := ValidateRecoverySnapshot(context.Background(), snapshotFile)
	_ = snapshotFile.Close()
	if err != nil || len(apps) != 1 || apps[0].Workload != "files" {
		t.Fatal("snapshot inventory validation", apps, err)
	}
	// Open only the main file, with no source WAL or live source connection.
	restored, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var value string
	if err = restored.QueryRow("SELECT value FROM settings WHERE key='retained-config'").Scan(&value); err != nil || value != "committed-WAL-value" {
		t.Fatal(value, err)
	}
	var claimed, epoch int
	if err = restored.QueryRow("SELECT claimed,epoch FROM identity").Scan(&claimed, &epoch); err != nil || claimed != 0 || epoch != 8 {
		t.Fatal(claimed, epoch, err)
	}
	var count int
	for _, table := range []string{"sessions", "credentials", "invitations", "challenges", "recovery_codes"} {
		if err = restored.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	if err = restored.QueryRow("SELECT count(*) FROM devices WHERE revoked_at IS NULL").Scan(&count); err != nil || count != 0 {
		t.Fatal("device trust restored", err)
	}
	if err = restored.QueryRow("SELECT count(*) FROM settings WHERE key='origin'").Scan(&count); err != nil || count != 0 {
		t.Fatal("old host origin retained", err)
	}
	if err = restored.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'job.cleanup.*'").Scan(&count); err != nil || count != 0 {
		t.Fatal("old cleanup authority retained", count, err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'job.cleanup.*'").Scan(&count); err != nil || count != 1 {
		t.Fatal("source cleanup intent modified", count, err)
	}
	if err = restored.QueryRow("SELECT count(*) FROM settings WHERE key='host.maintenance'").Scan(&count); err != nil || count != 0 {
		t.Fatal("recovery retained source maintenance barrier", count, err)
	}
	if err = restored.QueryRow("SELECT count(*) FROM settings WHERE key='host.backup-dispatch'").Scan(&count); err != nil || count != 0 {
		t.Fatal("recovery retained source worker dispatch authority", count, err)
	}
	if err = s.DB.QueryRow("SELECT value FROM settings WHERE key='host.backup-dispatch'").Scan(&value); err != nil || value != "uncertain:source-dispatch-intent" {
		t.Fatal("snapshot changed live dispatch intent", value, err)
	}
	if err = s.DB.QueryRow("SELECT value FROM settings WHERE key='host.maintenance'").Scan(&value); err != nil || value != "active-source-barrier" {
		t.Fatal("snapshot released source barrier", value, err)
	}
	if err = restored.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'host.activity.*'").Scan(&count); err != nil || count != 0 {
		t.Fatal("recovery retained source activity", count, err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'host.activity.*'").Scan(&count); err != nil || count != 1 {
		t.Fatal("snapshot released live activity", count, err)
	}
	if err = restored.QueryRow("SELECT count(*) FROM settings WHERE key GLOB 'host.maintenance-job.*'").Scan(&count); err != nil || count != 0 {
		t.Fatal("maintenance journal restored", count, err)
	}
	if err = s.DB.QueryRow("SELECT value FROM settings WHERE key='host.maintenance-job.root-token'").Scan(&value); err != nil || value != "source-root-token" {
		t.Fatal("source maintenance journal changed", value, err)
	}
	var operationState, operationPayload string
	if err = restored.QueryRow("SELECT state,result FROM operations WHERE id=?", deletionOp).Scan(&operationState, &operationPayload); err != nil || operationState != "interrupted" || operationPayload != "{}" {
		t.Fatal("recovery retained destructive authority", operationState, operationPayload, err)
	}
	if err = s.DB.QueryRow("SELECT state,result FROM operations WHERE id=?", deletionOp).Scan(&operationState, &operationPayload); err != nil || operationState != "pending" || operationPayload != deletionPayload {
		t.Fatal("source deletion authority changed", operationState, operationPayload, err)
	}
	if err = restored.QueryRow("SELECT count(*) FROM conversations WHERE id=?", conversation).Scan(&count); err != nil || count != 1 {
		t.Fatal("recovery removed conversation prematurely", count, err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM challenges").Scan(&count); err != nil || count != 2 {
		t.Fatal("source approval authority changed", count, err)
	}
	var phase string
	var revision int
	if err = restored.QueryRow("SELECT state,revision FROM apps WHERE workload='files'").Scan(&phase, &revision); err != nil || phase != "stopped" || revision != 10 {
		t.Fatal("restored runtime intent", phase, revision, err)
	}
	if err = restored.QueryRow("SELECT state FROM transfers").Scan(&phase); err != nil || phase != "cancelling" {
		t.Fatal("restored upload resumed", phase, err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM sessions").Scan(&count); err != nil || count != 1 {
		t.Fatal("source identity modified", err)
	}
	if _, err = s.RecoverySnapshot(context.Background(), staging); err == nil {
		t.Fatal("existing snapshot overwritten")
	}
}

func TestRecoverySnapshotRefusesPublicDirectoryAndCancelledContext(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "source"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	staging := filepath.Join(t.TempDir(), "snapshot")
	if err = os.Mkdir(staging, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecoverySnapshot(context.Background(), staging); err == nil {
		t.Fatal("public staging directory accepted")
	}
	if err = os.Chmod(staging, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.RecoverySnapshot(ctx, staging); err == nil {
		t.Fatal("cancelled snapshot succeeded")
	}
	if _, err = os.Stat(filepath.Join(staging, "snapshot.db")); !os.IsNotExist(err) {
		t.Fatal("failed snapshot left behind", err)
	}
}

func TestRecoverySnapshotRejectsMalformedFreshIdentity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "source"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.DB.Exec("INSERT INTO identity(singleton,owner_id,claimed,epoch) VALUES(1,?,1,1)", Random()); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path, err := store.RecoverySnapshot(context.Background(), directory)
	if err != nil {
		t.Fatal(err)
	}
	writable, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writable.Close()
	validate := func() error {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = ValidateRecoverySnapshot(context.Background(), file)
		return err
	}
	if err := validate(); err != nil {
		t.Fatal("generated identity refused", err)
	}
	for _, statement := range []string{"UPDATE identity SET owner_id='short'", "UPDATE identity SET owner_id='invalid identity with spaces'", "UPDATE identity SET owner_id='AAAAAAAAAAAAAAAAAAAAAAAA',epoch=1.5"} {
		if _, err := writable.Exec(statement); err != nil {
			t.Fatal(err)
		}
		if err := validate(); err == nil {
			t.Fatal("malformed restored identity admitted")
		}
	}
	if _, err := writable.Exec("UPDATE identity SET owner_id=?,epoch=2", Random()); err != nil {
		t.Fatal(err)
	}
	if err := validate(); err != nil {
		t.Fatal("valid replacement identity refused", err)
	}
}

func TestRecoverySnapshotRefusesEpochOverflowWithoutChangingLiveIdentity(t *testing.T) {
	for _, epoch := range []any{int64(0), float64(1.5), int64(math.MaxInt64)} {
		store, err := Open(filepath.Join(t.TempDir(), "source"))
		if err != nil {
			t.Fatal(err)
		}
		owner := Random()
		if _, err := store.DB.Exec("INSERT INTO identity(singleton,owner_id,claimed,epoch) VALUES(1,?,1,?)", owner, epoch); err != nil {
			t.Fatal(err)
		}
		directory := t.TempDir()
		if err := os.Chmod(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if path, err := store.RecoverySnapshot(context.Background(), directory); err == nil || path != "" {
			t.Fatal("invalid recovery epoch emitted", path, err)
		}
		var retained string
		var claimed int
		if err := store.DB.QueryRow("SELECT owner_id,claimed FROM identity").Scan(&retained, &claimed); err != nil || retained != owner || claimed != 1 {
			t.Fatal("failed snapshot changed source identity", err)
		}
		if _, err := os.Stat(filepath.Join(directory, "snapshot.db")); !os.IsNotExist(err) {
			t.Fatal("failed snapshot retained partial database", err)
		}
		store.Close()
	}
}

func TestRecoverySnapshotRefusesInvalidApplicationRevision(t *testing.T) {
	for _, revision := range []any{int64(-1), float64(1.5), int64(math.MaxInt64)} {
		store, err := Open(filepath.Join(t.TempDir(), "source"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.DB.Exec("INSERT INTO identity(singleton,owner_id,claimed,epoch) VALUES(1,?,1,1)", Random()); err != nil {
			t.Fatal(err)
		}
		instance := Random()
		if _, err := store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at,revision) VALUES('files',?,'stopped',1,?)", instance, revision); err != nil {
			t.Fatal(err)
		}
		directory := t.TempDir()
		if err := os.Chmod(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if path, err := store.RecoverySnapshot(context.Background(), directory); err == nil || path != "" {
			t.Fatal("invalid application revision emitted", path, err)
		}
		var retained string
		if err := store.DB.QueryRow("SELECT instance_id FROM apps WHERE workload='files'").Scan(&retained); err != nil || retained != instance {
			t.Fatal("failed snapshot modified live app", err)
		}
		if _, err := os.Stat(filepath.Join(directory, "snapshot.db")); !os.IsNotExist(err) {
			t.Fatal("partial snapshot retained", err)
		}
		store.Close()
	}
}

func TestRecoverySnapshotRejectsNonIntegerRestoredApplicationRevision(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "source"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.DB.Exec("INSERT INTO settings(key,value) VALUES('host.future-authority','live-marker')"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec("INSERT INTO identity(singleton,owner_id,claimed,epoch) VALUES(1,?,1,1)", Random()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec("INSERT INTO apps(workload,instance_id,state,updated_at,revision) VALUES('files',?,'stopped',1,0)", Random()); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path, err := store.RecoverySnapshot(context.Background(), directory)
	if err != nil {
		t.Fatal(err)
	}
	writable, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writable.Close()
	validate := func() error {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = ValidateRecoverySnapshot(context.Background(), file)
		return err
	}
	if err := validate(); err != nil {
		t.Fatal("valid exported application refused", err)
	}
	tx, err := writable.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	apps, validationErr := validateRecoveryDatabase(context.Background(), tx)
	if validationErr != nil || len(apps) != 1 || apps[0].Workload != "files" {
		tx.Rollback()
		t.Fatal("transaction recovery validation", apps, validationErr)
	}
	if _, err = tx.Exec("UPDATE identity SET claimed=1"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if apps, err = validateRecoveryDatabase(context.Background(), tx); !errors.Is(err, ErrRecovery) || apps != nil {
		tx.Rollback()
		t.Fatal("transaction restored authority admitted", apps, err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := writable.Exec("INSERT INTO settings(key,value) VALUES('host.future-authority','retained-marker')"); err != nil {
		t.Fatal(err)
	}
	if err := validate(); err == nil {
		t.Fatal("unknown restored host authority admitted")
	}
	if _, err := writable.Exec("DELETE FROM settings WHERE key='host.future-authority'"); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []any{float64(1.5), int64(-1)} {
		if _, err := writable.Exec("UPDATE apps SET revision=?", revision); err != nil {
			t.Fatal(err)
		}
		if err := validate(); err == nil {
			t.Fatal("invalid restored application revision admitted", revision)
		}
	}
	if _, err := writable.Exec("UPDATE apps SET revision=1"); err != nil {
		t.Fatal(err)
	}
	if err := validate(); err != nil {
		t.Fatal("valid restored application revision refused", err)
	}
}
