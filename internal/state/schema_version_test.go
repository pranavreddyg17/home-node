package state

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
)

func TestObserveSchemaVersionDoesNotMigrateOrResetDatabase(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	version, err := store.ObserveSchemaVersion(ctx)
	if err != nil || version != 4 {
		t.Fatal(version, err)
	}
	for _, schema := range []int{3, 5, 0, 1025} {
		query := "PRAGMA user_version=" + strconv.Itoa(schema)
		if _, err = store.DB.Exec(query); err != nil {
			t.Fatal(err)
		}
		version, err = store.ObserveSchemaVersion(ctx)
		if schema == 0 || schema == 1025 {
			if err == nil || version != 0 {
				t.Fatal("unbounded schema accepted", version, err)
			}
		} else if err != nil || version != schema {
			t.Fatal("schema observation substituted release constant", version, err)
		}
		var retained int
		if err = store.DB.QueryRow("PRAGMA user_version").Scan(&retained); err != nil || retained != schema {
			t.Fatal("observation migrated schema", retained, err)
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = store.ObserveSchemaVersion(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = (*Store)(nil).ObserveSchemaVersion(context.Background()); err == nil {
		t.Fatal("missing state accepted")
	}
}
