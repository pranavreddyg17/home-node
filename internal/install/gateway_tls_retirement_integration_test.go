package install

import (
	"context"
	"io"
	"os"
	"runtime"
	"syscall"
	"testing"
)

// Runs only in the opt-in disposable native installed account fixture. It
// qualifies removal authority and retained TLS bytes, not HTTPS startup or trust.
func qualifyNativeInstalledGatewayRetirement(t *testing.T, e *Engine, ctx context.Context, identity GatewayAccount) {
	t.Helper()
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || os.Getenv("HOMENODE_ACCOUNT_INSPECTION_INTEGRATION") != "1" {
		t.Fatal("disposable Linux root required")
	}
	names := []string{"etc/homenode/tls/server.key", "etc/homenode/tls/server.crt"}
	payloads := []string{"native retained key fixture\n", "native retained certificate fixture\n"}
	retained := []*os.File{}
	passed := false
	// Cleanup runs before configuration rollback and touches only these exclusive,
	// independently retained fixture inodes after all assertions have passed.
	t.Cleanup(func() {
		defer func() {
			for _, file := range retained {
				file.Close()
			}
		}()
		if !passed {
			return
		}
		root, err := os.OpenRoot("/")
		if err != nil {
			t.Error(err)
			return
		}
		defer root.Close()
		for i, file := range retained {
			info, err := file.Stat()
			named, nameErr := root.Lstat(names[i])
			if err != nil || nameErr != nil || !os.SameFile(info, named) {
				t.Error("TLS fixture replacement; preserving data")
				return
			}
			if err := root.Remove(names[i]); err != nil {
				t.Error(err)
				return
			}
		}
		if err := syncDirectory(root, "etc/homenode/tls"); err != nil {
			t.Error(err)
		}
	})
	for i, name := range names {
		mode := os.FileMode(0644)
		gid := 0
		if i == 0 {
			mode = 0640
			gid = identity.GID
		}
		file, err := e.host.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW, mode)
		if err != nil {
			t.Fatal("TLS fixture refuses occupied path", err)
		}
		retained = append(retained, file)
		if err := file.Chown(0, gid); err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString(payloads[i]); err != nil {
			t.Fatal(err)
		}
		if err := file.Sync(); err != nil {
			t.Fatal(err)
		}
	}
	marker, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.RetireGatewayTLS(ctx); err != nil {
		t.Fatal("native TLS retirement", err)
	}
	if err := e.RetireGatewayTLS(ctx); err != nil {
		t.Fatal("native TLS retirement replay", err)
	}
	if _, err := e.PrepareGatewayAccountRemoval(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.RemoveGatewayAccount(ctx); err != nil {
		t.Fatal("native installed gateway removal", err)
	}
	if err := e.RemoveGatewayAccount(ctx); err != nil {
		t.Fatal("native installed gateway removal replay", err)
	}
	for i, file := range retained {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(io.LimitReader(file, 1024))
		if err != nil || string(data) != payloads[i] {
			t.Fatal("retained TLS contents", err)
		}
		info, err := file.Stat()
		named, nameErr := e.host.Lstat(names[i])
		mode := os.FileMode(0644)
		if i == 0 {
			mode = 0600
		}
		if err != nil || nameErr != nil || !os.SameFile(info, named) || info.Mode().Perm() != mode {
			t.Fatal("retained TLS inode or permissions", err, nameErr)
		}
	}
	after, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(marker, after) {
		t.Fatal("activation block changed", err)
	}
	qualifyNativeConfigurationRemoval(t, e, ctx)
	passed = true
	t.Log("native installed gateway TLS retirement, ownership publication, gateway identity removal and ready replay passed; retained bytes and activation marker preserved; HTTPS startup remains unqualified")
}
