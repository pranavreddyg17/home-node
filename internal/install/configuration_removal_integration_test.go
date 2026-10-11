package install

import (
	"context"
	"io"
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"
)

// Runs only after guarded NSS/allocator restoration and gateway teardown in the
// opt-in disposable Linux fixture. It qualifies generated legacy runtime units,
// not a running private HTTPS gateway or owner workload workflows.
func qualifyNativeConfigurationRemoval(t *testing.T, e *Engine, ctx context.Context) {
	t.Helper()
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || os.Getenv("HOMENODE_ACCOUNT_INSPECTION_INTEGRATION") != "1" || e.host.Name() != "/" {
		t.Fatal("disposable Linux root fixture required")
	}
	marker, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("disposable retained owner data fixture\n")
	fixtures := map[string]os.FileInfo{}
	for _, name := range []string{"var/lib/homenode/control/.homenode-removal-fixture-data", "var/lib/homenode-backup/.homenode-removal-fixture-data"} {
		file, err := e.host.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
		if err != nil {
			t.Fatal("retention fixture vacancy", err)
		}
		if _, err := file.Write(payload); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Sync(); err != nil {
			file.Close()
			t.Fatal(err)
		}
		info, err := file.Stat()
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
		fixtures[name] = info
	}
	plan, err := e.PrepareConfigurationRemoval(ctx)
	if err != nil {
		t.Fatal("native configuration removal preparation", err)
	}
	if len(plan.Files) == 0 {
		t.Fatal("native removal plan lacks files")
	}
	if err := e.RemoveConfiguration(ctx); err != nil {
		t.Fatal("native configuration removal", err)
	}
	if err := e.RemoveConfiguration(ctx); err != nil {
		t.Fatal("native configuration removal readiness replay", err)
	}
	after, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(marker, after) {
		t.Fatal("native removal released or replaced activation", err)
	}
	for _, file := range plan.Files {
		if _, err := e.host.Lstat(file.Path); !os.IsNotExist(err) {
			t.Fatal("configuration remains after removal", file.Path, err)
		}
	}
	for name, before := range fixtures {
		file, err := e.host.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal("retained data lost", err)
		}
		info, statErr := file.Stat()
		data, readErr := io.ReadAll(io.LimitReader(file, 4097))
		closeErr := file.Close()
		if statErr != nil || readErr != nil || closeErr != nil || !os.SameFile(before, info) || !owned(info, 0) || info.Mode().Perm() != 0600 || string(data) != string(payload) {
			t.Fatal("retained data changed", name, statErr, readErr, closeErr)
		}
		// Delete only this fixture's independently retained inode after qualification.
		named, err := e.host.Lstat(name)
		if err != nil || !os.SameFile(before, named) {
			t.Fatal("fixture data cleanup identity", err)
		}
		if err := e.host.Remove(name); err != nil {
			t.Fatal(err)
		}
		if err := syncDirectory(e.host, path.Dir(name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("native public configuration removal, dormant unit transitions, retained activation, app/backup data and ready replay passed; HTTPS gateway runtime unqualified")
}
