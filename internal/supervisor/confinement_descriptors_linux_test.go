//go:build linux

package supervisor

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
)

// The disposable fixture uses the shared observer; it never enables production
// admission or derives a retirement/UID-release receipt from metadata.
func nativeQEMUDescriptorInventory(t *testing.T, ctx context.Context, d Domain, pid int) {
	t.Helper()
	if os.Getenv("HOMENODE_CONFINEMENT_INVENTORY") != "1" {
		return
	}
	records, err := observeQEMUDescriptorMetadata(ctx, d, pid)
	if err != nil {
		t.Fatal("QEMU descriptor metadata observation refused", err)
	}
	procRoot, err := os.OpenRoot("/proc")
	if err != nil {
		t.Fatal(err)
	}
	taskCount, verifiedTasks := 0, 0
	taskErr := withGuestUIDTaskStatus(ctx, procRoot, strconv.Itoa(pid), &taskCount, func(status []byte) error {
		if err := validateGuestDACCredentials(status, d.GuestUID, d.GuestGID); err != nil {
			return err
		}
		if err := validateQEMUSeccompStatus(status); err != nil {
			return err
		}
		verifiedTasks++
		return nil
	})
	closeErr := procRoot.Close()
	if taskErr != nil || closeErr != nil || verifiedTasks == 0 {
		t.Fatal("per-task seccomp observation refused", taskErr, closeErr)
	}
	writableRegular, socketCount := 0, 0
	for _, r := range records {
		flags, err := strconv.ParseUint(r.Flags, 8, 32)
		if err != nil {
			t.Fatal(err)
		}
		if r.Kind == "regular" && flags&3 != 0 {
			writableRegular++
		}
		if r.Kind == "socket" {
			socketCount++
		}
	}
	data, err := json.Marshal(records)
	if err != nil || len(data) > 128<<10 {
		t.Fatal("descriptor inventory serialization", err)
	}
	t.Log("native QEMU per-task seccomp filter mode observed", "tasks", verifiedTasks, "filterContentsQualified", false)
	t.Log("native QEMU sandbox launch arguments observed", "filterContentsQualified", false)
	t.Log("native QEMU paired output pipe metadata observed", "readerQualified", false)
	t.Log("native QEMU socket metadata observation completed", "count", socketCount, "admissionQualified", false)
	t.Log("native QEMU descriptor metadata", string(data))
	t.Log("native QEMU descriptor inventory completed", "count", len(records), "writableRegular", writableRegular, "admissionQualified", false)
}
