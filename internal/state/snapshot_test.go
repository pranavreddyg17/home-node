package state

import (
	"bytes"
	"context"
	"database/sql"
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
