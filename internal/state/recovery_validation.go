package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

var ErrRecovery = errors.New("recovery snapshot is invalid or contains restored authority")
var recoveryInstanceID = regexp.MustCompile(`^[a-zA-Z0-9_-]{20,64}$`)

type RecoveryApp struct{ Workload, InstanceID string }

func schemaDigest(ctx context.Context, db *sql.DB) ([32]byte, error) {
	rows, err := db.QueryContext(ctx, "SELECT type,name,tbl_name,sql FROM sqlite_schema WHERE name NOT GLOB 'sqlite_*' ORDER BY type,name")
	if err != nil {
		return [32]byte{}, err
	}
	defer rows.Close()
	definitions := [][4]string{}
	for rows.Next() {
		var row [4]string
		if err = rows.Scan(&row[0], &row[1], &row[2], &row[3]); err != nil {
			return [32]byte{}, err
		}
		definitions = append(definitions, row)
		if len(definitions) > 256 {
			return [32]byte{}, ErrRecovery
		}
	}
	if err = rows.Err(); err != nil {
		return [32]byte{}, err
	}
	data, err := json.Marshal(definitions)
	return sha256.Sum256(data), err
}

// ValidateRecoverySnapshot reads an already-open standalone snapshot without
// migrating or running any restored SQL. Its schema must match the versioned
// schema compiled into this release. No recovered identity grants access.
func ValidateRecoverySnapshot(ctx context.Context, file *os.File) ([]RecoveryApp, error) {
	if file == nil {
		return nil, ErrRecovery
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 256<<20 {
		return nil, ErrRecovery
	}
	prefix := "/proc/self/fd/"
	if runtime.GOOS == "darwin" {
		prefix = "/dev/fd/"
	} else if runtime.GOOS != "linux" {
		return nil, ErrRecovery
	}
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	uri := "file:" + prefix + strconv.FormatUint(uint64(file.Fd()), 10) + "?mode=ro&immutable=1&_pragma=query_only(ON)&_pragma=trusted_schema(OFF)"
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, ErrRecovery
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	reference, err := sql.Open("sqlite", "file:homenode-recovery-schema?mode=memory&cache=private&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	defer reference.Close()
	reference.SetMaxOpenConns(1)
	if err = (&Store{DB: reference}).migrate(); err != nil {
		return nil, err
	}
	expected, err := schemaDigest(deadline, reference)
	if err != nil {
		return nil, err
	}
	actual, err := schemaDigest(deadline, db)
	if err != nil || actual != expected {
		return nil, ErrRecovery
	}
	var version int
	if err = db.QueryRowContext(deadline, "PRAGMA user_version").Scan(&version); err != nil || version != 4 {
		return nil, ErrRecovery
	}
	var check string
	if err = db.QueryRowContext(deadline, "PRAGMA integrity_check").Scan(&check); err != nil || check != "ok" {
		return nil, ErrRecovery
	}
	foreignKeys, err := db.QueryContext(deadline, "PRAGMA foreign_key_check")
	if err != nil {
		return nil, ErrRecovery
	}
	invalid := foreignKeys.Next()
	err = foreignKeys.Err()
	foreignKeys.Close()
	if err != nil || invalid {
		return nil, ErrRecovery
	}
	for _, query := range []string{
		"SELECT count(*) FROM identity WHERE claimed=0 AND epoch>0 AND length(owner_id)>0",
	} {
		var count int
		if err = db.QueryRowContext(deadline, query).Scan(&count); err != nil || count != 1 {
			return nil, ErrRecovery
		}
	}
	for _, query := range []string{
		"SELECT (SELECT count(*) FROM sessions)+(SELECT count(*) FROM credentials)+(SELECT count(*) FROM invitations)+(SELECT count(*) FROM challenges)+(SELECT count(*) FROM recovery_codes)",
		"SELECT count(*) FROM devices WHERE revoked_at IS NULL OR capabilities!='[]'",
		"SELECT count(*) FROM settings WHERE key='origin'",
		"SELECT count(*) FROM settings WHERE key GLOB 'job.cleanup.*'",
		"SELECT count(*) FROM jobs WHERE start_requested!=0",
		"SELECT count(*) FROM apps WHERE state!='stopped' OR operation_id IS NOT NULL OR revision<0",
		"SELECT count(*) FROM jobs WHERE state IN('queued','preparing','running','finalizing','cancelling')",
		"SELECT count(*) FROM generations WHERE state IN('staging','queued','running','cancelling')",
		"SELECT count(*) FROM operations WHERE state IN('pending','executing','requires-action')",
		"SELECT count(*) FROM transfers WHERE state IN('uploading','verifying')",
		"SELECT count(*) FROM files WHERE NOT EXISTS(SELECT 1 FROM apps WHERE workload='files')",
		"SELECT count(*) FROM generations WHERE NOT EXISTS(SELECT 1 FROM apps WHERE workload='ai')",
	} {
		var count int
		if err = db.QueryRowContext(deadline, query).Scan(&count); err != nil || count != 0 {
			return nil, ErrRecovery
		}
	}
	rows, err := db.QueryContext(deadline, "SELECT workload,instance_id FROM apps ORDER BY workload")
	if err != nil {
		return nil, ErrRecovery
	}
	defer rows.Close()
	apps := []RecoveryApp{}
	for rows.Next() {
		var app RecoveryApp
		if err = rows.Scan(&app.Workload, &app.InstanceID); err != nil {
			return nil, ErrRecovery
		}
		if !recoveryInstanceID.MatchString(app.InstanceID) {
			return nil, ErrRecovery
		}
		apps = append(apps, app)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: inventory unavailable", ErrRecovery)
	}
	return apps, nil
}
