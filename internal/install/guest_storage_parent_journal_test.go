package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	t.Run("immutable transition", func(t *testing.T) {
		host, journalDirectory := roots(t)
		e := openEngine(t, host, journalDirectory)
		defer e.Close()
		ctx := context.Background()
		sha := strings.Repeat("b", 64)
		if err := e.commitGuestStorageParentJournalIntent(ctx, installed, 993, 994, sha); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(journalDirectory, "guest-storage-image-parent-journal.json")
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var intent guestStorageParentJournalIntent
		if json.Unmarshal(saved, &intent) != nil || !reflect.DeepEqual(intent.Original, installed) || !reflect.DeepEqual(intent.Desired, changed) || intent.ParentIntentSHA256 != sha {
			t.Fatal("transition not bound to exact states")
		}
		if err := e.commitGuestStorageParentJournalIntent(ctx, installed, 993, 994, sha); err != nil {
			t.Fatal("exact retry refused", err)
		}
		if err := e.commitGuestStorageParentJournalIntent(ctx, installed, 993, 995, sha); !errors.Is(err, ErrConflict) {
			t.Fatal("changed destination admitted", err)
		}
		if err := e.commitGuestStorageParentJournalIntent(ctx, installed, 993, 994, strings.Repeat("c", 64)); !errors.Is(err, ErrConflict) {
			t.Fatal("changed parent binding admitted", err)
		}
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(before, after) {
			t.Fatal("transition inode changed", err)
		}
		current, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(saved, current) {
			t.Fatal("transition bytes changed", err)
		}
		for _, admitted := range []journal{installed, changed} {
			if err := e.withGuestStorageParentJournalIntent(ctx, admitted, 993, 994, sha, func(intent guestStorageParentJournalIntent, guard func() error) error {
				return guard()
			}); err != nil {
				t.Fatal("recorded journal state refused", err)
			}
		}
		if err := e.withGuestStorageParentJournalIntent(ctx, installed, 993, 995, sha, func(guestStorageParentJournalIntent, func() error) error {
			t.Fatal("changed transition consumer called")
			return nil
		}); !errors.Is(err, ErrConflict) {
			t.Fatal("changed destination transition admitted", err)
		}
		if err := e.withGuestStorageParentJournalIntent(ctx, installed, 993, 994, sha, func(intent guestStorageParentJournalIntent, guard func() error) error {
			if err := os.Rename(path, path+".original"); err != nil {
				return err
			}
			if err := os.WriteFile(path, saved, 0600); err != nil {
				return err
			}
			if err := guard(); !errors.Is(err, ErrConflict) {
				t.Fatal("identical transition replacement admitted", err)
			}
			return nil
		}); !errors.Is(err, ErrConflict) {
			t.Fatal("final transition guard admitted replacement", err)
		}
	})
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
