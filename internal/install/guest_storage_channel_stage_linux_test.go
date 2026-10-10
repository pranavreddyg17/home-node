//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"golang.org/x/sys/unix"
)

func TestRootGuestStorageChannelStagePreservesRecordedCandidate(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
	ctx := context.Background()
	identity, err := guestUIDProvisioningPlan(ctx, strings.Repeat("a", 32), supervisor.GuestUIDPool{First: 200000, Last: 200002}, []uint32{1001, 1002})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := guestStorageProvisioningPlan(ctx, identity, 994, []int{1001, 1002, 1003})
	if err != nil {
		t.Fatal(err)
	}
	host, journalDir := roots(t)
	runtime := filepath.Join(host, "run", "homenode")
	if err := os.MkdirAll(runtime, 0755); err != nil {
		t.Fatal(err)
	}
	engine := openEngine(t, host, journalDir)
	defer engine.Close()
	guard := func(ctx context.Context) error { return ctx.Err() }
	stage, err := engine.stageGuestStorageChannelParent(ctx, plan, 1002, guard)
	if err != nil {
		t.Fatal(err)
	}
	var actual unix.Stat_t
	path := filepath.Join(runtime, ".homenode-guests.stage")
	if unix.Lstat(path, &actual) != nil || actual.Mode != unix.S_IFDIR|0710 || actual.Uid != 0 || actual.Gid != 1002 || stage.Device != uint64(actual.Dev) || stage.Inode != actual.Ino {
		t.Fatal("candidate provenance mismatch", stage, actual)
	}
	if _, err := os.Lstat(filepath.Join(runtime, "guests")); !os.IsNotExist(err) {
		t.Fatal("staging published parent", err)
	}
	receipt := filepath.Join(journalDir, "guest-storage-channel-stage.json")
	before, err := os.Lstat(receipt)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(receipt)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := engine.loadGuestStorageChannelStage(ctx, plan, 1002)
	if err != nil || !reflect.DeepEqual(loaded, stage) {
		t.Fatal("recorded stage could not be loaded", loaded, err)
	}
	if foreign, err := engine.loadGuestStorageChannelStage(ctx, plan, 1003); !errors.Is(err, ErrConflict) || !reflect.DeepEqual(foreign, guestStorageChannelStage{}) {
		t.Fatal("foreign transfer authority admitted", foreign, err)
	}
	unknown := append(append([]byte(nil), data[:len(data)-1]...), []byte(",\"unexpected\":true}")...)
	if err := os.WriteFile(receipt, unknown, 0600); err != nil {
		t.Fatal(err)
	}
	if malformed, err := engine.loadGuestStorageChannelStage(ctx, plan, 1002); !errors.Is(err, ErrConflict) || !reflect.DeepEqual(malformed, guestStorageChannelStage{}) {
		t.Fatal("unknown receipt field admitted", malformed, err)
	}
	if err := os.WriteFile(receipt, data, 0600); err != nil {
		t.Fatal(err)
	}
	if retry, err := engine.stageGuestStorageChannelParent(ctx, plan, 1002, guard); err == nil || !reflect.DeepEqual(retry, guestStorageChannelStage{}) {
		t.Fatal("existing stage adopted", retry, err)
	}
	after, err := os.Lstat(receipt)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("recorded receipt replaced", err)
	}
	current, err := os.ReadFile(receipt)
	if err != nil || string(current) != string(data) {
		t.Fatal("receipt evidence changed", err)
	}
	interrupted := errors.New("channel publication acknowledgement interrupted")
	engine.checkpoint = func(name, path string) error {
		if name == "guest-storage-channel-parent-published" {
			return interrupted
		}
		return nil
	}
	if err := engine.publishGuestStorageChannelParent(ctx, plan, 1002, stage, guard); !errors.Is(err, interrupted) {
		t.Fatal("publication interruption not observed", err)
	}
	engine.checkpoint = nil
	if err := engine.publishGuestStorageChannelParent(ctx, plan, 1002, stage, guard); err != nil {
		t.Fatal("recorded publication retry refused", err)
	}
	if completed, err := os.Lstat(receipt); err != nil || !os.SameFile(before, completed) {
		t.Fatal("publication retry rewrote receipt", err)
	}
	final := filepath.Join(runtime, "guests")
	if err := os.Rename(final, final+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(final, 0710); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(final, 0, 1002); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := engine.withRecordedGuestStorageChannel(ctx, plan, 1002, stage, guard, func(_ *os.Root, _, _ *os.File, _ func(context.Context) error) error { called = true; return nil }); err == nil || called {
		t.Fatal("matching foreign final directory admitted", called, err)
	}
}
