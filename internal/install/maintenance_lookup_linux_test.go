//go:build linux

package install

import (
	"context"
	"testing"
	"time"
)

func TestMaintenanceNameVacancyUsesFixedNativeLookup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, database := range []string{"passwd", "group"} {
		if _, _, err := lookupAccount(ctx, database, "homenode-backup"); err != nil {
			t.Fatal("fixed backup name rejected", database, err)
		}
	}
	if _, _, err := lookupAccount(ctx, "passwd", "arbitrary-user"); err == nil {
		t.Fatal("arbitrary name lookup admitted")
	}
}
