//go:build linux

package install

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRootGuestStorageConfigurationSourceRefusesReplacement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
	host, journalDir := roots(t)
	directory := filepath.Join(host, "etc", "homenode")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	e := openEngine(t, host, journalDir)
	defer e.Close()
	for _, name := range []string{"runtime-policy.json", "services.env"} {
		mode := os.FileMode(0644)
		if name == "runtime-policy.json" {
			mode = 0600
		}
		path := filepath.Join(directory, name)
		original := "owned configuration\n"
		if err := os.WriteFile(path, []byte(original), mode); err != nil {
			t.Fatal(err)
		}
		if err := e.withGuestStorageConfigurationSource(context.Background(), name, original, func(_ context.Context, _ *os.File, check func() error) error { return check() }); err != nil {
			t.Fatal(err)
		}
		err := e.withGuestStorageConfigurationSource(context.Background(), name, original, func(_ context.Context, _ *os.File, check func() error) error {
			if err := os.Rename(path, path+".original"); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(original), mode); err != nil {
				return err
			}
			if err := check(); !errors.Is(err, ErrConflict) {
				t.Fatal("identical replacement admitted", err)
			}
			return nil
		})
		if !errors.Is(err, ErrConflict) {
			t.Fatal("replacement scope succeeded", err)
		}
		for _, p := range []string{path, path + ".original"} {
			data, err := os.ReadFile(p)
			if err != nil || string(data) != original {
				t.Fatal("replacement evidence altered", err)
			}
		}
	}
}

func TestRootGuestStorageConfigurationSourcesRetainBothFiles(t *testing.T) {
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
	policy := supervisor.Policy{Generation: 1, MemoryMiB: 1024, VCPUs: 1, MaxInstances: 1, DiskReserveBytes: 4 << 30, ControllerUID: 1001, TransferUID: 1002}
	source, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	source = append(source, '\n')
	env := []byte("TAILNET_IP=100.100.1.2\nHTTPS_PORT=8787\nHTTPS_ORIGIN=https://home.example.ts.net:8787\nPOLICY_GENERATION=1\nCONTROLLER_UID=1001\nRUNTIME_GID=1003\nTRANSFER_GID=1002\n")
	items := []record{{Path: "etc/homenode/runtime-policy.json", UID: 0, GID: 0, Mode: 0600, SHA256: digest(source), State: "pending"}, {Path: "var/lib/homenode/images", Directory: true, Mode: 0710, GID: 994, State: "pending"}}
	items = append(items, record{Path: "etc/homenode/services.env", Mode: 0644, SHA256: digest(env), State: "pending"})
	encoded, _ := json.Marshal(items)
	installed := journal{Version: 1, ID: strings.Repeat("a", 32), Phase: "installed", Digest: digest(encoded), Items: items}
	for i := range installed.Items {
		installed.Items[i].State = "created"
	}

	host, journalDir := roots(t)
	directory := filepath.Join(host, "etc", "homenode")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(directory, "runtime-policy.json")
	envPath := filepath.Join(directory, "services.env")
	if err := os.WriteFile(policyPath, source, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, env, 0644); err != nil {
		t.Fatal(err)
	}
	e := openEngine(t, host, journalDir)
	defer e.Close()
	guard := func(ctx context.Context) error { return ctx.Err() }
	if err := e.commitGuestStorageConfigurationIntent(ctx, installed, source, env, plan, guard); err != nil {
		t.Fatal(err)
	}
	consume := func(_ context.Context, _ guestStorageConfigurationIntent, _ *os.File, _ *os.File, check func() error) error {
		return check()
	}
	if err := e.withGuestStorageConfigurationSources(ctx, installed, plan, guard, consume); err != nil {
		t.Fatal(err)
	}
	stage, err := e.stageGuestStorageConfiguration(ctx, installed, plan, guard)
	if err != nil || stage.Version != 1 || len(stage.Files) != 2 {
		t.Fatal("configuration staging failed", err)
	}
	loaded, err := e.loadGuestStorageConfigurationStage(ctx, installed, plan)
	if err != nil || !reflect.DeepEqual(loaded, stage) {
		t.Fatal("recorded stage could not be authenticated", err)
	}
	for _, receipt := range stage.Files {
		data, err := os.ReadFile(filepath.Join(directory, receipt.Name))
		if err != nil || digest(data) != receipt.SHA256 || int64(len(data)) != receipt.Bytes {
			t.Fatal("staged replacement lost recorded content", err)
		}
	}
	if err := e.withGuestStorageConfigurationStaged(ctx, installed, plan, guard, func(_ guestStorageConfigurationStage, files []*os.File, check func() error) error {
		if len(files) != 2 {
			t.Fatal("missing retained replacement")
		}
		return check()
	}); err != nil {
		t.Fatal("recorded replacements refused", err)
	}
	stagePath := filepath.Join(directory, stage.Files[1].Name)
	stageData, err := os.ReadFile(stagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.withGuestStorageConfigurationStaged(ctx, installed, plan, guard, func(_ guestStorageConfigurationStage, _ []*os.File, check func() error) error {
		if err := os.Rename(stagePath, stagePath+".original"); err != nil {
			return err
		}
		if err := os.WriteFile(stagePath, stageData, 0600); err != nil {
			return err
		}
		return check()
	}); !errors.Is(err, ErrConflict) {
		t.Fatal("identical replacement stage adopted", err)
	}
	if retry, err := e.stageGuestStorageConfiguration(ctx, installed, plan, guard); !errors.Is(err, os.ErrExist) || retry.Version != 0 {
		t.Fatal("existing stage adopted without reconciliation", err)
	}
	err = e.withGuestStorageConfigurationSources(ctx, installed, plan, guard, func(_ context.Context, _ guestStorageConfigurationIntent, _ *os.File, _ *os.File, check func() error) error {
		if err := os.Rename(policyPath, policyPath+".original"); err != nil {
			return err
		}
		if err := os.WriteFile(policyPath, source, 0600); err != nil {
			return err
		}
		if err := check(); !errors.Is(err, ErrConflict) {
			t.Fatal("policy replacement hidden while environment retained", err)
		}
		return nil
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("joint scope accepted replaced policy", err)
	}
	data, err := os.ReadFile(envPath)
	if err != nil || string(data) != string(env) {
		t.Fatal("environment changed on refusal", err)
	}
	recordPath := filepath.Join(journalDir, "guest-storage-configuration-stage.json")
	bad := stage
	bad.Files = append([]guestStorageConfigurationStageFile(nil), stage.Files...)
	bad.Files[1].Inode = bad.Files[0].Inode
	encodedBad, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, encodedBad, 0600); err != nil {
		t.Fatal(err)
	}
	if loaded, err := e.loadGuestStorageConfigurationStage(ctx, installed, plan); !errors.Is(err, ErrConflict) || loaded.Version != 0 {
		t.Fatal("aliased replacement receipt admitted", err)
	}

}

func TestRootGuestStorageConfigurationSourceRefusesDefaultACLDrift(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("disposable Linux root fixture required")
	}
	host, journalDir := roots(t)
	directory := filepath.Join(host, "etc", "homenode")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "runtime-policy.json")
	original := "owned configuration\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	e := openEngine(t, host, journalDir)
	defer e.Close()
	// A valid base default ACL leaves existing directory mode bits unchanged.
	acl := make([]byte, 28)
	binary.LittleEndian.PutUint32(acl, 2)
	for i, entry := range []struct{ tag, perm uint16 }{{1, 7}, {4, 5}, {32, 5}} {
		offset := 4 + i*8
		binary.LittleEndian.PutUint16(acl[offset:], entry.tag)
		binary.LittleEndian.PutUint16(acl[offset+2:], entry.perm)
		binary.LittleEndian.PutUint32(acl[offset+4:], 0xffffffff)
	}
	err := e.withGuestStorageConfigurationSource(context.Background(), "runtime-policy.json", original, func(_ context.Context, _ *os.File, check func() error) error {
		if err := unix.Setxattr(directory, "system.posix_acl_default", acl, 0); err != nil {
			return err
		}
		return check()
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("default ACL drift admitted", err)
	}
	observed, err := os.Stat(directory)
	if err != nil || observed.Mode().Perm() != 0755 {
		t.Fatal("ACL fixture changed ordinary mode", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != original {
		t.Fatal("refusal changed source", err)
	}
}

func TestRootGuestStorageConfigurationPublicationRecoversJournalInterruption(t *testing.T) {
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
	policy := supervisor.Policy{Generation: 1, MemoryMiB: 1024, VCPUs: 1, MaxInstances: 1, DiskReserveBytes: 4 << 30, ControllerUID: 1001, TransferUID: 1002}
	source, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	source = append(source, '\n')
	env := []byte("TAILNET_IP=100.100.1.2\nHTTPS_PORT=8787\nHTTPS_ORIGIN=https://home.example.ts.net:8787\nPOLICY_GENERATION=1\nCONTROLLER_UID=1001\nRUNTIME_GID=1003\nTRANSFER_GID=1002\n")
	items := []record{{Path: "etc/homenode/runtime-policy.json", UID: 0, GID: 0, Mode: 0600, SHA256: digest(source), State: "pending"}, {Path: "var/lib/homenode/images", Directory: true, Mode: 0710, GID: 994, State: "pending"}}
	items = append(items, record{Path: "etc/homenode/services.env", Mode: 0644, SHA256: digest(env), State: "pending"})
	encoded, _ := json.Marshal(items)
	installed := journal{Version: 1, ID: strings.Repeat("a", 32), Phase: "installed", Digest: digest(encoded), Items: items}
	for i := range installed.Items {
		installed.Items[i].State = "created"
	}

	host, journalDir := roots(t)
	directory := filepath.Join(host, "etc", "homenode")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(directory, "runtime-policy.json")
	envPath := filepath.Join(directory, "services.env")
	if err := os.WriteFile(policyPath, source, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, env, 0644); err != nil {
		t.Fatal(err)
	}
	e := openEngine(t, host, journalDir)
	defer e.Close()
	guard := func(ctx context.Context) error { return ctx.Err() }
	if err := e.commitGuestStorageConfigurationIntent(ctx, installed, source, env, plan, guard); err != nil {
		t.Fatal(err)
	}
	consume := func(_ context.Context, _ guestStorageConfigurationIntent, _ *os.File, _ *os.File, check func() error) error {
		return check()
	}
	if err := e.withGuestStorageConfigurationSources(ctx, installed, plan, guard, consume); err != nil {
		t.Fatal(err)
	}

	imagesPath := filepath.Join(host, "var", "lib", "homenode", "images")
	if err := os.MkdirAll(imagesPath, 0710); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(imagesPath, 0, 994); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(imagesPath, 0710); err != nil {
		t.Fatal(err)
	}
	if err := e.save(installed); err != nil {
		t.Fatal(err)
	}
	if _, err := e.stageGuestStorageConfiguration(ctx, installed, plan, guard); err != nil {
		t.Fatal(err)
	}
	directoryFD, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer directoryFD.Close()
	if err := os.Chmod(imagesPath, 0777); err != nil {
		t.Fatal(err)
	}
	if err := e.publishGuestStorageConfigurationLocked(ctx, directoryFD, plan, guard); !errors.Is(err, ErrConflict) {
		t.Fatal("unrelated installation drift admitted", err)
	}
	for path, expected := range map[string][]byte{policyPath: source, envPath: env} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != string(expected) {
			t.Fatal("unrelated drift refusal published configuration", err)
		}
	}
	if err := os.Chmod(imagesPath, 0710); err != nil {
		t.Fatal(err)
	}
	foreign, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.publishGuestStorageConfigurationLocked(ctx, foreign, plan, guard); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign configuration directory admitted", err)
	}
	if err := foreign.Close(); err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("journal publication interrupted")
	fault := func(context.Context) error {
		data, err := os.ReadFile(envPath)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "POLICY_GENERATION=2\n") {
			return interrupted
		}
		return nil
	}
	if err := e.publishGuestStorageConfigurationLocked(ctx, directoryFD, plan, fault); !errors.Is(err, interrupted) {
		t.Fatal("missing journal interruption", err)
	}
	observed, err := e.load()
	if err != nil || !reflect.DeepEqual(observed, installed) {
		t.Fatal("journal changed before configuration exchange acknowledgement", err)
	}
	publishedPolicy, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	corruptPolicy := append([]byte(nil), publishedPolicy...)
	corruptPolicy[0] ^= 1
	if err := os.WriteFile(policyPath, corruptPolicy, 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.publishGuestStorageConfigurationLocked(ctx, directoryFD, plan, guard); !errors.Is(err, ErrConflict) {
		t.Fatal("corrupt exchanged policy authorized journal publication", err)
	}
	observed, err = e.load()
	if err != nil || !reflect.DeepEqual(observed, installed) {
		t.Fatal("corrupt exchanged bytes changed journal", err)
	}
	if err := os.WriteFile(policyPath, publishedPolicy, 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.publishGuestStorageConfigurationLocked(ctx, directoryFD, plan, guard); err != nil {
		t.Fatal("interrupted publication retry failed", err)
	}
	desired, _, _, err := planGuestStorageConfiguration(ctx, installed, source, env, plan)
	if err != nil {
		t.Fatal(err)
	}
	observed, err = e.load()
	if err != nil || !reflect.DeepEqual(observed, desired) {
		t.Fatal("destination journal not committed", err)
	}
	before, err := os.Stat(filepath.Join(journalDir, "install.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.publishGuestStorageConfigurationLocked(ctx, directoryFD, plan, guard); err != nil {
		t.Fatal("completed retry refused", err)
	}
	after, err := os.Stat(filepath.Join(journalDir, "install.json"))
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("completed retry replaced journal", err)
	}
	if err := unix.Renameat2(int(directoryFD.Fd()), "runtime-policy.json", int(directoryFD.Fd()), ".homenode-runtime-policy.stage", unix.RENAME_EXCHANGE); err != nil {
		t.Fatal(err)
	}
	reverted, err := os.Stat(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.publishGuestStorageConfigurationLocked(ctx, directoryFD, plan, guard); !errors.Is(err, ErrConflict) {
		t.Fatal("committed journal authorized repair of reverted configuration", err)
	}
	preserved, err := os.Stat(policyPath)
	if err != nil || !os.SameFile(reverted, preserved) {
		t.Fatal("refusal rewrote reverted namespace", err)
	}
	observed, err = e.load()
	if err != nil || !reflect.DeepEqual(observed, desired) {
		t.Fatal("refusal reverted committed journal", err)
	}
}
