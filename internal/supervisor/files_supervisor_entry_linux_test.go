//go:build linux

package supervisor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/state"
)

// Qualify the actual executable's signed catalog and reserved-policy startup.
// The ephemeral fixture signing key is not a release publisher. This does not
// launch a VM through that process or qualify installed systemd/user access.
func nativeFilesSupervisorEntry(t *testing.T, ctx context.Context, base, images string, image catalog.Image, policy Policy, transferGID int) {
	t.Helper()
	binary := "/usr/lib/homenode-fixtures/homenode-supervisor"
	info, err := os.Lstat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0755 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		t.Fatal("native supervisor binary unavailable", err)
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok || metadata.Uid != 0 || metadata.Nlink != 1 {
		t.Fatal("unprotected supervisor fixture binary")
	}
	directory := filepath.Join(base, "supervisor-entry")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for name, gid := range map[string]int{"volumes": int(policy.GuestIdentity.GuestGID), "channels": transferGID} {
		path := filepath.Join(directory, name)
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(path, 0, gid); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0710); err != nil {
			t.Fatal(err)
		}
	}
	storage := make(map[string]os.FileInfo)
	for _, path := range []string{images, filepath.Join(images, image.SHA256+".raw"), filepath.Join(directory, "volumes"), filepath.Join(directory, "channels")} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		storage[path] = info
	}
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(catalog.Manifest{Schema: 1, Version: 1, Expires: time.Now().Add(time.Hour), Images: []catalog.Image{image}})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(catalog.Envelope{KeyID: catalog.KeyID(pub), Payload: payload, Signature: ed25519.Sign(private, payload)})
	if err != nil {
		t.Fatal(err)
	}
	policyBytes, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"policy.json": policyBytes, "catalog.pub": []byte(hex.EncodeToString(pub)), "catalog.json": envelope, "catalog-floor": []byte("1\n")} {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	socket := filepath.Join(directory, "supervisor.sock")
	childCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(childCtx, binary,
		"--policy", filepath.Join(directory, "policy.json"), "--publisher-key", filepath.Join(directory, "catalog.pub"),
		"--catalog", filepath.Join(directory, "catalog.json"), "--catalog-floor", filepath.Join(directory, "catalog-floor"),
		"--state-dir", filepath.Join(directory, "journal"), "--images", images, "--volumes", filepath.Join(directory, "volumes"),
		"--channels", filepath.Join(directory, "channels"), "--socket", socket,
		"--access-gid", strconv.Itoa(transferGID), "--transfer-gid", strconv.Itoa(transferGID))
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	command.WaitDelay = 3 * time.Second
	var diagnostics boundedOutput
	command.Stdout, command.Stderr = &diagnostics, &diagnostics
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	waited := false
	defer func() {
		if !waited {
			cancel()
			<-done
		}
		if output := diagnostics.String(); output != "" {
			t.Log("native supervisor entry diagnostics", output)
		}
	}()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	deadline := time.Now().Add(20 * time.Second)
	ready := false
	for time.Now().Before(deadline) && childCtx.Err() == nil {
		select {
		case err := <-done:
			waited = true
			t.Fatal("supervisor exited before readiness", err)
		default:
		}
		response, err := client.Get("http://local/v1/runtime")
		if err == nil {
			response.Body.Close()
			if response.StatusCode != http.StatusForbidden {
				t.Fatal("root caller not refused", response.StatusCode)
			}
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatal("supervisor startup did not reach authenticated listener")
	}
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		waited = true
		if err != nil {
			t.Fatal("supervisor did not shut down cleanly", err)
		}
	case <-childCtx.Done():
		t.Fatal("supervisor shutdown timed out")
	}
	if diagnostics.tooLarge {
		t.Fatal("supervisor diagnostics exceeded fixture bound")
	}
	for path, before := range storage {
		after, err := os.Lstat(path)
		if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() {
			t.Fatal("startup changed provisioned storage metadata", path, err)
		}
		first, last := before.Sys().(*syscall.Stat_t), after.Sys().(*syscall.Stat_t)
		if first.Uid != last.Uid || first.Gid != last.Gid || first.Nlink != last.Nlink {
			t.Fatal("startup changed provisioned storage ownership", path)
		}
	}
	for _, name := range []string{"volumes", "channels"} {
		entries, err := os.ReadDir(filepath.Join(directory, name))
		if err != nil || len(entries) != 0 {
			t.Fatal("idle startup created guest storage", name, err)
		}
	}
	journal := filepath.Join(directory, "journal")
	if _, err := os.Lstat(journal); err != nil {
		t.Fatal("startup journal absent", err)
	}
	store, err := state.Open(journal)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var version, generation string
	if err := store.DB.QueryRow("SELECT value FROM settings WHERE key='catalog-version'").Scan(&version); err != nil || version != "1" {
		t.Fatal("signed catalog acceptance not retained", version, err)
	}
	if err := store.DB.QueryRow("SELECT value FROM settings WHERE key='policy-generation'").Scan(&generation); err != nil || generation != "1" {
		t.Fatal("reserved policy acceptance not retained", generation, err)
	}
	var policyHash string
	if err := store.DB.QueryRow("SELECT value FROM settings WHERE key='policy-hash'").Scan(&policyHash); err != nil || policyHash != state.Hash(string(policyBytes)) {
		t.Fatal("accepted policy bytes not retained", err)
	}
	t.Log("actual supervisor executable accepted fixture-signed Files catalog and qualified reserved policy, refused root caller, and shut down cleanly; no VM or installed-service acceptance claimed")
}
