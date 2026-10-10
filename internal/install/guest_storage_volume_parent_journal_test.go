package install

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestGuestStorageVolumeParentJournalPreservesImageAndConfigurationAuthority(t *testing.T) {
	items := []record{{Path: "var/lib/homenode/images", Directory: true, GID: 994, Mode: 0710, State: "created"}, {Path: "var/lib/homenode/volumes", Directory: true, GID: 993, Mode: 0710, State: "created"}, {Path: "etc/homenode/runtime-policy.json", Mode: 0600, SHA256: "unchanged", State: "created"}}
	normalized := append([]record(nil), items...)
	for i := range normalized {
		normalized[i].State = "pending"
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	original := journal{Version: 1, ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Phase: "installed", Items: items, Digest: digest(encoded)}
	desired, err := planGuestStorageVolumeParentJournal(context.Background(), original, 993, 994)
	if err != nil {
		t.Fatal(err)
	}
	if desired.Items[1].GID != 994 || !reflect.DeepEqual(desired.Items[0], original.Items[0]) || !reflect.DeepEqual(desired.Items[2], original.Items[2]) || original.Items[1].GID != 993 || desired.Digest == original.Digest {
		t.Fatal("volume transition changed unrelated authority")
	}
	if _, err := planGuestStorageVolumeParentJournal(context.Background(), original, 992, 994); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong source group admitted", err)
	}
	duplicate := original
	duplicate.Items = append(append([]record(nil), original.Items...), original.Items[1])
	if _, err := planGuestStorageVolumeParentJournal(context.Background(), duplicate, 993, 994); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate volume parent admitted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := planGuestStorageVolumeParentJournal(ctx, original, 993, 994); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
