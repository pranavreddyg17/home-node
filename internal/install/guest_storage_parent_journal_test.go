package install

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestGuestStorageParentJournalPreservesUnrelatedRecords(t *testing.T) {
	items := []record{
		{Path: "var/lib/homenode/images", Directory: true, UID: 0, GID: 993, Mode: 0710, State: "created"},
		{Path: "var/lib/homenode/volumes", Directory: true, UID: 0, GID: 993, Mode: 0710, State: "created"},
	}
	normalized := append([]record(nil), items...)
	for i := range normalized {
		normalized[i].State = "pending"
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	installed := journal{Version: 1, ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Phase: "installed", Digest: digest(data), Items: items}
	changed, err := planGuestStorageImageParentJournal(context.Background(), installed, 993, 994)
	if err != nil || changed.Items[0].GID != 994 || !reflect.DeepEqual(changed.Items[1], installed.Items[1]) || changed.Digest == installed.Digest {
		t.Fatal("incorrect transition", changed, err)
	}
	if installed.Items[0].GID != 993 {
		t.Fatal("planner mutated installed authority")
	}
	if _, err := planGuestStorageImageParentJournal(context.Background(), installed, 992, 994); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong source group admitted", err)
	}
	corrupt := installed
	corrupt.Digest = "untrusted"
	if _, err := planGuestStorageImageParentJournal(context.Background(), corrupt, 993, 994); !errors.Is(err, ErrConflict) {
		t.Fatal("unbound journal admitted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := planGuestStorageImageParentJournal(ctx, installed, 993, 994); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
