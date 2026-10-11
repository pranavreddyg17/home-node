package install

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// Runs only in the opt-in disposable native installed account fixture. It
// qualifies removal authority and retained TLS bytes, not HTTPS startup or trust.
func qualifyNativeInstalledGatewayRetirement(t *testing.T, engine **Engine, ctx context.Context, identity GatewayAccount) {
	e := *engine
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

			metadata, ok := info.Sys().(*syscall.Stat_t)
			expectedMode := os.FileMode(0644)
			if i == 0 {
				expectedMode = 0600
			}
			if !ok || metadata.Uid != 0 || metadata.Gid != 0 || metadata.Nlink != 1 || !info.Mode().IsRegular() || info.Mode().Perm() != expectedMode {
				t.Error("TLS cleanup metadata changed; preserving data")
				return
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				t.Error(err)
				return
			}
			data, err := io.ReadAll(io.LimitReader(file, 1024))
			if err != nil || string(data) != payloads[i] {
				t.Error("TLS cleanup contents changed; preserving data", err)
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

	if _, err := e.PrepareGatewayAccountRemoval(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.RemoveGatewayAccount(ctx); !errors.Is(err, ErrConflict) {
		t.Fatal("numeric identity release before TLS retirement admitted", err)
	}
	if actual, err := InspectGatewayAccount(ctx); err != nil || actual != identity {
		t.Fatal("refused TLS-dependent removal changed identity", err)
	}
	base, err := e.loadAccountJournal()
	if err != nil {
		t.Fatal(err)
	}
	removal, err := e.loadGatewayRemovalIntent(base)
	if err != nil || removal.Completed != 0 || removal.Ready {
		t.Fatal("refused TLS-dependent removal advanced intent", err)
	}
	// Release the installation lock for the actual installed CLI process.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	*engine = nil

	cliContext, stopCLI := context.WithTimeout(ctx, 10*time.Minute)
	command := exec.CommandContext(cliContext, "/usr/bin/homenode", "gateway-tls-retire", "--journal-dir", "/var/lib/homenode-install")
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	command.WaitDelay = time.Second
	var boundedOutput accountOutput
	command.Stdout = &boundedOutput
	command.Stderr = io.Discard
	err = command.Run()
	contextErr := cliContext.Err()
	stopCLI()
	if err != nil || contextErr != nil {
		t.Fatal("packaged native TLS retirement CLI", err, contextErr)
	}
	output := boundedOutput.Bytes()

	var status struct {
		Phase     string `json:"phase"`
		Retired   bool   `json:"tlsOwnershipRetired"`
		Block     bool   `json:"activationBlockRetained"`
		Dormant   bool   `json:"servicesDormant"`
		Deleted   bool   `json:"tlsContentsDeleted"`
		Removed   bool   `json:"installationRemoved"`
		Activated bool   `json:"servicesActivated"`
	}
	if err := json.Unmarshal(output, &status); err != nil || status.Phase != "gateway-tls-retire" || !status.Retired || !status.Block || !status.Dormant || status.Deleted || status.Removed || status.Activated {
		t.Fatal("packaged TLS retirement result", err)
	}
	e, err = Open("/", "/var/lib/homenode-install")
	if err != nil {
		t.Fatal("reopen packaged TLS retirement", err)
	}
	*engine = e

	if err := e.RetireGatewayTLS(ctx); err != nil {
		t.Fatal("native TLS retirement replay", err)
	}
	if _, err := e.PrepareGatewayAccountRemoval(ctx); err != nil {
		t.Fatal(err)
	}

	interrupted := errors.New("native installed removal lost acknowledgement")
	e.checkpoint = func(stage, name string) error {
		if stage == "gateway-account-removed" {
			return interrupted
		}
		return nil
	}
	if err := e.RemoveGatewayAccount(ctx); !errors.Is(err, interrupted) {
		t.Fatal("native installed removal interruption", err)
	}
	e.checkpoint = nil
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

		if err != nil || nameErr != nil || !os.SameFile(info, named) || info.Mode().Perm() != mode || !info.Mode().IsRegular() {
			t.Fatal("retained TLS inode or permissions", err, nameErr)
		}
		metadata, ok := info.Sys().(*syscall.Stat_t)
		if !ok || metadata.Uid != 0 || metadata.Gid != 0 || metadata.Nlink != 1 {
			t.Fatal("retained TLS ownership or links changed")
		}

	}

	directory, err := e.host.Lstat("etc/homenode/tls")
	if err != nil || !directory.IsDir() || directory.Mode().Perm() != 0700 {
		t.Fatal("retained TLS directory permissions", err)
	}
	directoryMetadata, ok := directory.Sys().(*syscall.Stat_t)
	if !ok || directoryMetadata.Uid != 0 || directoryMetadata.Gid != 0 {
		t.Fatal("retained TLS directory ownership")
	}
	after, err := e.journalRoot.Lstat("recovery-blocked")
	if err != nil || !os.SameFile(marker, after) {
		t.Fatal("activation block changed", err)
	}
	qualifyNativeConfigurationRemoval(t, e, ctx)
	passed = true
	t.Log("native packaged TLS retirement CLI, ownership publication, gateway identity removal and ready replay passed; retained bytes and activation marker preserved; HTTPS startup remains unqualified")
}
