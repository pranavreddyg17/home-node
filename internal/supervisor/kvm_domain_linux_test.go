//go:build linux

package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

// This launches firmware on synthetic disks, not an application guest image.
func TestNativeReservedDACLibvirtLaunch(t *testing.T) {
	runNativeReservedDACLaunch(t, false)
}

func TestNativeReservedDACGuestConnect(t *testing.T) {
	runNativeReservedDACLaunch(t, true)
}

func TestNativeReservedDACManagerLaunch(t *testing.T) {
	runNativeReservedDACLaunch(t, false, true)
}

func TestNativeReservedFilesManagerLaunch(t *testing.T) {
	if os.Getenv("HOMENODE_FILES_MANAGER_INTEGRATION") != "1" {
		t.Skip("explicit development Files image fixture required")
	}
	runNativeReservedDACLaunch(t, false, true, true)
}

func runNativeReservedDACLaunch(t *testing.T, guestConnect bool, managerLaunch ...bool) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_KVM_DOMAIN_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux libvirt/KVM experiment")
	}
	duration := 90 * time.Second
	if len(managerLaunch) > 1 && managerLaunch[1] {
		duration = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	device, err := os.Lstat("/dev/kvm")
	if err != nil {
		t.Fatal(err)
	}
	deviceStat, ok := device.Sys().(*syscall.Stat_t)
	if !ok || device.Mode()&os.ModeCharDevice == 0 || deviceStat.Uid != 0 || deviceStat.Gid == 0 || deviceStat.Gid > 1<<31-1 || device.Mode().Perm() != 0660 || device.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || unix.Major(uint64(deviceStat.Rdev)) != 10 || unix.Minor(uint64(deviceStat.Rdev)) != 232 {
		t.Fatal("unqualified KVM device group")
	}
	if _, err := unix.Lgetxattr("/dev/kvm", "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
		t.Fatal("ambiguous native launch device ACL", err)
	}
	defer func() {
		current, err := os.Lstat("/dev/kvm")
		if err != nil || !os.SameFile(device, current) || current.Mode() != device.Mode() {
			t.Errorf("native launch device identity changed: %v", err)
			return
		}
		metadata, valid := current.Sys().(*syscall.Stat_t)
		if !valid || metadata.Uid != deviceStat.Uid || metadata.Gid != deviceStat.Gid || metadata.Rdev != deviceStat.Rdev {
			t.Error("native launch device ownership changed")
		}
		if _, err := unix.Lgetxattr("/dev/kvm", "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
			t.Errorf("native launch device ACL changed: %v", err)
		}
	}()
	// Never adopt an existing workload partition or an occupied libvirt service.
	names, err := command(ctx, "", "/usr/bin/virsh", "--connect", "qemu:///system", "list", "--all", "--name")
	if err != nil || strings.TrimSpace(names) != "" {
		t.Fatal("libvirt experiment requires an empty disposable service", err)
	}
	partition, err := command(ctx, "", "/usr/bin/systemctl", "show", "homenode.slice", "--property=LoadState,FragmentPath,DropInPaths,ActiveState,SubState,Job,ControlGroup")
	if err != nil {
		t.Fatal("cannot inspect disposable workload partition", err)
	}
	properties := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(partition), "\n") {
		key, value, valid := strings.Cut(line, "=")
		if !valid {
			t.Fatal("malformed workload partition properties")
		}
		if _, duplicate := properties[key]; duplicate {
			t.Fatal("duplicate workload partition property", key)
		}
		properties[key] = value
	}
	// Systemd can implicitly load a named slice without a unit file. Admit
	// only that inert, source-free state; never stop or replace an active slice.
	for key, expected := range map[string]string{"FragmentPath": "", "DropInPaths": "", "ActiveState": "inactive", "SubState": "dead", "Job": "", "ControlGroup": ""} {
		value, present := properties[key]
		if !present || value != expected {
			t.Fatal("workload partition already occupied", key, value)
		}
	}
	if properties["LoadState"] != "not-found" && properties["LoadState"] != "loaded" {
		t.Fatal("unqualified workload partition load state", properties["LoadState"])
	}
	pool, err := ObserveLocalGuestUIDConflicts(ctx, GuestUIDPool{First: 2000000000, Last: 2000000255})
	if err != nil {
		t.Fatal(err)
	}
	pool, err = ObserveGuestUIDProcessConflicts(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uint32
	for uid := pool.First; uid <= pool.Last && len(ids) < 2; uid++ {
		if !pool.Blocked[uid] && uid != deviceStat.Gid {
			ids = append(ids, uid)
		}
	}
	if len(ids) != 2 {
		t.Fatal("insufficient unused experiment identities")
	}
	uid, transferGID := ids[0], ids[1]
	base, err := os.MkdirTemp("/tmp", "hn-kvm-")
	if err != nil {
		t.Fatal(err)
	}
	safeCleanup := true
	defer func() {
		if !safeCleanup {
			t.Log("preserving uncertain native fixture", base)
			return
		}
		if err := os.RemoveAll(base); err != nil {
			t.Errorf("fixture directory cleanup: %v", err)
		}
	}()
	if err := os.Chmod(base, 0755); err != nil {
		t.Fatal(err)
	}
	const slice = "/run/systemd/system/homenode.slice"
	source, err := os.OpenFile(slice, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Chmod(0644); err != nil {
		source.Close()
		t.Fatal(err)
	}
	const unit = "[Unit]\nDescription=Disposable HomeNode native launch experiment\n[Slice]\nMemoryMax=1G\nCPUQuota=100%\nTasksMax=512\n"
	if _, err := io.WriteString(source, unit); err != nil {
		source.Close()
		t.Fatal(err)
	}
	if err := source.Sync(); err != nil {
		source.Close()
		t.Fatal(err)
	}
	ownedUnit, err := source.Stat()
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if !safeCleanup {
			t.Log("preserving uncertain experiment unit", slice)
			return
		}
		if _, err := command(cleanup, "", "/usr/bin/systemctl", "stop", "homenode.slice"); err != nil {
			safeCleanup = false
			t.Errorf("slice stop: %v", err)
			return
		}
		current, err := os.Lstat(slice)
		if err != nil || !os.SameFile(ownedUnit, current) {
			safeCleanup = false
			t.Errorf("slice ownership changed: %v", err)
			return
		}
		contents, err := os.ReadFile(slice)
		if err != nil || string(contents) != unit {
			safeCleanup = false
			t.Errorf("slice bytes changed: %v", err)
			return
		}
		if err := os.Remove(slice); err != nil {
			safeCleanup = false
			t.Errorf("slice removal: %v", err)
			return
		}
		if _, err := command(cleanup, "", "/usr/bin/systemctl", "daemon-reload"); err != nil {
			t.Errorf("slice reload: %v", err)
		}
	}()
	if _, err := command(ctx, "", "/usr/bin/systemctl", "daemon-reload"); err != nil {
		t.Fatal(err)
	}
	if _, err := command(ctx, "", "/usr/bin/systemctl", "start", "homenode.slice"); err != nil {
		t.Fatal(err)
	}
	id := state.Random()
	domain := Domain{ID: id, GuestUID: uid, GuestGID: deviceStat.Gid, Image: catalog.Image{MemoryMiB: 256, VCPUs: 1, DataBytes: 16 << 20}, SystemPath: filepath.Join(base, "images", "system.raw"), DataPath: filepath.Join(base, "volumes", id+".raw"), ChannelPath: filepath.Join(base, id, "adapter.sock")}
	// Exercise the proposed protected storage layout under the sole KVM group.
	// These are owned fixture directories, not an installed policy publication.
	for _, directory := range []string{filepath.Dir(domain.SystemPath), filepath.Dir(domain.DataPath)} {
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(directory, 0, int(deviceStat.Gid)); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(directory, 0710); err != nil {
			t.Fatal(err)
		}
	}
	for _, disk := range []struct {
		path  string
		owner uint32
		mode  os.FileMode
	}{{domain.SystemPath, 0, 0440}, {domain.DataPath, uid, 0600}} {
		file, err := os.OpenFile(disk.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(16 << 20); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Chown(int(disk.owner), int(deviceStat.Gid)); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Chmod(disk.mode); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Sync(); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	otherDisk := filepath.Join(filepath.Dir(domain.DataPath), "other.raw")
	other, err := os.OpenFile(otherDisk, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Chown(int(ids[1]), int(deviceStat.Gid)); err != nil {
		other.Close()
		t.Fatal(err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	probeStorage := func() {
		t.Helper()
		// A host process with the exact proposed guest DAC identity tests
		// pathname access only; this is not an in-guest exploit test.
		probe := exec.CommandContext(ctx, "/usr/bin/python3", "-c", `import errno, os, sys
uid, gid = int(sys.argv[4]), int(sys.argv[5])
if os.getresuid() != (uid,uid,uid) or os.getresgid() != (gid,gid,gid) or os.getgroups() != []:
    raise RuntimeError("guest storage probe credentials differ")
for path, flags in ((sys.argv[1],os.O_RDONLY),(sys.argv[2],os.O_RDWR)):
    os.close(os.open(path,flags|os.O_NOFOLLOW))
for path, flags in ((sys.argv[1],os.O_WRONLY),(sys.argv[3],os.O_RDONLY),(sys.argv[3],os.O_WRONLY)):
    try:
        fd=os.open(path,flags|os.O_NOFOLLOW)
    except OSError as error:
        if error.errno != errno.EACCES: raise
    else:
        os.close(fd)
        raise RuntimeError("forbidden storage access admitted")
`, domain.SystemPath, domain.DataPath, otherDisk, strconv.FormatUint(uint64(uid), 10), strconv.FormatUint(uint64(deviceStat.Gid), 10))
		probe.Env = []string{"PATH=/usr/bin:/bin", "LANG=C"}
		probe.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: deviceStat.Gid, Groups: []uint32{}}, Pdeathsig: syscall.SIGKILL}
		probe.WaitDelay = 3 * time.Second
		if output, err := probe.CombinedOutput(); err != nil {
			t.Fatalf("native guest DAC storage probe: %v %s", err, output)
		}
	}
	storage := map[string]unix.Stat_t{}
	for _, path := range []string{filepath.Dir(domain.SystemPath), filepath.Dir(domain.DataPath), domain.SystemPath, domain.DataPath, otherDisk} {
		var before unix.Stat_t
		if err := unix.Lstat(path, &before); err != nil {
			t.Fatal(err)
		}
		storage[path] = before
	}
	checkStorage := func() {
		t.Helper()
		for path, before := range storage {
			var after unix.Stat_t
			if err := unix.Lstat(path, &after); err != nil {
				t.Fatal("native storage disappeared", path, err)
			}
			if before.Dev != after.Dev || before.Ino != after.Ino || before.Mode != after.Mode || before.Uid != after.Uid || before.Gid != after.Gid || before.Nlink != after.Nlink || before.Size != after.Size {
				t.Fatal("native launch changed protected storage metadata", path)
			}
		}
	}
	if err := prepareGuestChannelDirectory(ctx, filepath.Dir(domain.ChannelPath), int(uid), int(transferGID)); err != nil {
		t.Fatal(err)
	}
	backend := LinuxBackend{DataRoot: base, TransferGID: int(transferGID)}
	// Register teardown before launch so uncertain create outcomes are stopped.
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if err := backend.Stop(cleanup, id); err != nil {
			safeCleanup = false
			t.Errorf("domain cleanup: %v", err)
			return
		}
		running, err := backend.Running(cleanup, id)
		if err != nil || running {
			safeCleanup = false
			t.Errorf("domain cleanup exit not observed: %v", err)
		}
	}()
	var listener *net.UnixListener
	if guestConnect {
		listener, err = net.ListenUnix("unix", &net.UnixAddr{Name: domain.ChannelPath, Net: "unix"})
		if err != nil {
			t.Fatal(err)
		}
		listener.SetUnlinkOnClose(false)
		defer listener.Close()
		// Synthetic admission only: production listener ownership is undecided.
		if err := os.Chown(domain.ChannelPath, int(uid), int(transferGID)); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(domain.ChannelPath, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		start := func() error {
			if !guestConnect {
				return backend.Start(ctx, domain)
			}
			xml, err := domain.XML()
			if err != nil {
				return err
			}
			// Experiment changes only channel direction in production-generated XML.
			xml = strings.Replace(xml, "<source mode='bind'", "<source mode='connect'", 1)
			_, err = command(ctx, xml, "/usr/bin/virsh", "--connect", "qemu:///system", "create", "/dev/stdin")
			return err
		}
		if err := start(); err != nil {
			safeCleanup = false
			nativeDomainDiagnostics(t, domain)
			t.Fatal("native reserved-DAC launch refused", err)
		}
		// Qualify the draft memory observer before the existing production gate.
		// Success here does not bypass channel or full launch verification.
		readBounded := func(path string, limit int64) (data []byte, result error) {
			file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
			if err != nil {
				return nil, err
			}
			defer func() { result = errors.Join(result, file.Close()) }()
			data, err = io.ReadAll(io.LimitReader(file, limit+1))
			if err != nil {
				return nil, err
			}
			if int64(len(data)) > limit {
				return nil, ErrPolicy
			}
			return data, nil
		}
		pidBytes, err := readBounded(filepath.Join("/run/libvirt/qemu", domain.Name()+".pid"), 64)
		if err != nil {
			t.Fatal(err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
		if err != nil || pid <= 0 {
			t.Fatal("invalid native domain PID")
		}
		membership, err := readBounded(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"), 4096)
		if err != nil {
			t.Fatal(err)
		}
		relative, err := guestUnifiedMembership(membership, id)
		if err != nil {
			t.Fatal("ambiguous native cgroup membership", err)
		}
		maximum := int64(domain.Image.MemoryMiB+512) * (1 << 20)
		if err := observeGuestMemoryDomain(ctx, relative, id, maximum); err != nil {
			nativeDomainDiagnostics(t, domain)
			t.Fatal("native per-domain memory observation refused", err)
		}
		if err := observeGuestMemoryDomain(ctx, relative, id, maximum-1); err == nil {
			t.Fatal("excessive domain memory bound admitted")
		}
		if err := observeGuestMemoryDomain(ctx, relative, state.Random(), maximum); err == nil {
			t.Fatal("another domain memory scope admitted")
		}
		cancelled, stopObservation := context.WithCancel(ctx)
		stopObservation()
		if err := observeGuestMemoryDomain(cancelled, relative, id, maximum); !errors.Is(err, context.Canceled) {
			t.Fatal("native cancelled memory observation admitted", err)
		}
		if err := observeGuestMemoryProcess(ctx, pid, id, maximum); err != nil {
			nativeDomainDiagnostics(t, domain)
			t.Fatal("native pinned process memory observation refused", err)
		}
		memoryEvidence, err := nativeFixtureCommand(ctx, "/usr/bin/env", "HOMENODE_SUPERVISOR_SYSTEMD_INTEGRATION=1",
			"HOMENODE_SUPERVISOR_MEMORY_PID="+strconv.Itoa(pid), "HOMENODE_SUPERVISOR_MEMORY_ID="+id,
			"HOMENODE_SUPERVISOR_MEMORY_MAX="+strconv.FormatInt(maximum, 10),
			"/usr/bin/python3", "../../packaging/systemd/supervisor_fixture.py")
		if err != nil {
			t.Log("bounded source-protected memory fixture diagnostics", memoryEvidence)
			t.Fatal("source-protected native process memory fixture", err)
		}
		t.Log(memoryEvidence)
		t.Log("native per-domain memory observer positive and refusal checks passed")
		if guestConnect {
			if err := verifyGuestDACProcess(ctx, pid, uid, domain.GuestGID); err != nil {
				t.Fatal("guest-connect process credentials", err)
			}
			label, err := readBounded(filepath.Join("/proc", strconv.Itoa(pid), "attr/current"), 4096)
			if err != nil || !strings.HasPrefix(string(label), "libvirt-") || !strings.HasSuffix(strings.TrimSpace(string(label)), "(enforce)") {
				t.Fatal("guest-connect AppArmor not enforcing", err)
			}
			if err := listener.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			accepted, err := listener.AcceptUnix()
			if err != nil {
				t.Fatal("guest-initiated connection missing", err)
			}
			identity, identityErr := PeerProcessIdentity(accepted)
			closeErr := accepted.Close()
			if identityErr != nil || closeErr != nil || identity.UID != uid || identity.GID != domain.GuestGID || int(identity.PID) != pid {
				t.Fatal("guest-connect peer identity mismatch", identity, identityErr, closeErr)
			}
			if err := backend.Stop(ctx, id); err != nil {
				t.Fatal(err)
			}
			if running, err := backend.Running(ctx, id); err != nil || running {
				t.Fatal("guest-connect stop unverified", running, err)
			}
			t.Log("native guest-initiated channel authenticated without payload", attempt)
			continue
		}
		// Production adoption already probes exact QEMU credentials without a
		// payload. Do not add a diagnostic connection to the firmware-only guest:
		// it has no adapter to drain the inherited listener's bounded backlog.
		if err := backend.Verify(ctx, domain); err != nil {
			nativeDomainDiagnostics(t, domain)
			t.Fatal("native launch isolation verification refused", err)
		}
		peer, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", domain.ChannelPath)
		if err != nil {
			t.Fatal("native adapter listener unavailable", err)
		}
		peerUID, identityErr := PeerUID(peer.(*net.UnixConn))
		checkStorage()
		probeStorage()
		// Reconciliation must not create another adapter connection after adoption.
		repeatedVerify := backend.Verify(ctx, domain)
		closeErr := peer.Close()
		if repeatedVerify != nil {
			t.Fatal("native active-channel verification retry refused", repeatedVerify)
		}
		if identityErr != nil || peerUID != uid || closeErr != nil {
			t.Fatal("native listener creator identity differs from guest", peerUID, identityErr, closeErr)
		}
		if err := backend.Stop(ctx, id); err != nil {
			t.Fatal(err)
		}
		running, err := backend.Running(ctx, id)
		if err != nil || running {
			t.Fatal("native guest not stopped", running, err)
		}
		checkStorage()
		if err := prepareGuestChannelDirectory(ctx, filepath.Dir(domain.ChannelPath), int(uid), int(transferGID)); err != nil {
			t.Fatal("stopped channel retry refused", err)
		}
	}
	if len(managerLaunch) != 0 && managerLaunch[0] {
		filesImage := len(managerLaunch) > 1 && managerLaunch[1]
		runNativeReservedManagerLifecycle(t, ctx, base, domain, backend, &safeCleanup, filesImage)
	}
}

// Native fixture diagnostics contain only fixture authority and process metadata.
// Keep their bounded output on failure without changing production command errors.
func nativeFixtureCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = 3 * time.Second
	var output boundedOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	err := errors.Join(cmd.Run(), ctx.Err())
	if output.tooLarge {
		err = errors.Join(err, errors.New("native fixture diagnostic limit exceeded"))
	}
	return output.String(), err
}

func runNativeReservedManagerLifecycle(t *testing.T, ctx context.Context, base string, previous Domain, backend LinuxBackend, safeCleanup *bool, filesImage bool) {
	t.Helper()
	contents, err := os.ReadFile(previous.SystemPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(contents)
	image := previous.Image
	image.ID, image.SHA256, image.Bytes, image.Protocol = "files", hex.EncodeToString(sum[:]), int64(len(contents)), 1
	images := filepath.Dir(previous.SystemPath)
	if err := os.Rename(previous.SystemPath, filepath.Join(images, image.SHA256+".raw")); err != nil {
		t.Fatal(err)
	}
	clientScript := ""
	if filesImage {
		image = stageNativeFilesImage(t, ctx, images, previous.GuestGID)
		clientScript = stageNativeFilesClient(t, base)
	}
	channels := filepath.Join(base, "manager-channels")
	if err := os.Mkdir(channels, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(channels, 0, backend.TransferGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(channels, 0710); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(filepath.Join(base, "manager-journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	identity := ReservedGuestPolicy{Version: 1, FirstUID: previous.GuestUID, LastUID: previous.GuestUID, GuestGID: previous.GuestGID}
	pool, err := QualifyReservedGuestPolicy(ctx, identity, []uint32{1, 2}, []uint32{uint32(backend.TransferGID), uint32(backend.TransferGID)})
	if err != nil {
		t.Fatal("native published policy qualification", err)
	}
	m := &Manager{Store: store, Backend: backend, Images: images, Volumes: filepath.Dir(previous.DataPath), Channels: channels,
		GuestUIDPool: &pool, GuestGID: previous.GuestGID,
		Policy:   Policy{GuestIdentity: &identity, Generation: 1, MemoryMiB: 1024, VCPUs: 1, MaxInstances: 1, DiskReserveBytes: 4 * catalog.GiB, ControllerUID: 1, TransferUID: 2},
		Manifest: catalog.Manifest{Schema: 1, Version: 1, Expires: time.Now().Add(time.Hour), Images: []catalog.Image{image}}}
	if err := m.Initialize(ctx); err != nil {
		t.Fatal("native manager initialization", err)
	}
	r := startRequest()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := m.stopDomain(cleanup, r.InstanceID); err != nil {
			*safeCleanup = false
			t.Errorf("native manager teardown: %v", err)
		}
	}()
	for attempt := int64(0); attempt < 2; attempt++ {
		r.OperationID, r.Revision = state.Random(), attempt*2+1
		instance, err := m.Apply(ctx, r)
		if err != nil || instance.State != "running" || instance.GuestUID != previous.GuestUID {
			observed, inspectErr := m.Inspect(ctx, r.InstanceID)
			t.Log("native manager journal after refusal", observed, inspectErr)
			t.Fatal("native manager launch", instance, err)
		}
		if err := m.Audit(ctx); err != nil {
			t.Fatal("native manager audit", err)
		}
		if filesImage {
			channel, err := m.Channel(ctx, r.InstanceID)
			if err != nil {
				t.Fatal("native Files manager channel admission", err)
			}
			roundTripNativeFilesChannel(t, ctx, clientScript, channel, backend.TransferGID, r.InstanceID, attempt)
		}
		stop := r
		stop.Action, stop.OperationID, stop.Revision = "stop", state.Random(), r.Revision+1
		instance, err = m.Apply(ctx, stop)
		if err != nil || instance.State != "stopped" {
			t.Fatal("native manager stop", instance, err)
		}
	}
	if filesImage {
		t.Log("development Files manager launch, object round trip, audit, stop and identity-retaining restart completed; release remains unqualified")
	} else {
		t.Log("synthetic native manager launch, audit, stop and identity-retaining restart completed")
	}
}

// Diagnostics are limited to this synthetic domain's process credentials and log.
func nativeDomainDiagnostics(t *testing.T, domain Domain) {
	t.Helper()
	read := func(path string, limit int64) []byte {
		file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return nil
		}
		defer file.Close()
		data, _ := io.ReadAll(io.LimitReader(file, limit))
		return data
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(string(read(filepath.Join("/run/libvirt/qemu", domain.Name()+".pid"), 64)))); err == nil && pid > 0 {
		process := filepath.Join("/proc", strconv.Itoa(pid))
		for _, line := range strings.Split(string(read(filepath.Join(process, "status"), 64<<10)), "\n") {
			if strings.HasPrefix(line, "Uid:") || strings.HasPrefix(line, "Gid:") || strings.HasPrefix(line, "Groups:") || strings.HasPrefix(line, "Cap") || strings.HasPrefix(line, "NoNewPrivs:") {
				t.Log(line)
			}
		}
		t.Log("AppArmor", strings.TrimSpace(string(read(filepath.Join(process, "attr/current"), 4096))))
		cgroup := strings.TrimSpace(string(read(filepath.Join(process, "cgroup"), 4096)))
		t.Log("cgroup", cgroup)
		// Record only the synthetic domain's hierarchy, stopping at our slice.
		// Libvirt may place emulator threads beneath the memory-limited scope.
		for _, line := range strings.Split(cgroup, "\n") {
			relative, unified := strings.CutPrefix(line, "0::")
			if !unified || !strings.HasPrefix(relative, "/homenode.slice/") || filepath.Clean(relative) != relative {
				continue
			}
			for depth := 0; depth < 16 && strings.HasPrefix(relative, "/homenode.slice"); depth++ {
				t.Log("memory bound", relative, strings.TrimSpace(string(read(filepath.Join("/sys/fs/cgroup", relative, "memory.max"), 128))))
				if relative == "/homenode.slice" {
					break
				}
				relative = filepath.Dir(relative)
			}
		}
	}
	if socket, err := os.Lstat(domain.ChannelPath); err == nil {
		if metadata, ok := socket.Sys().(*syscall.Stat_t); ok {
			t.Log("channel metadata", socket.Mode(), metadata.Uid, metadata.Gid, metadata.Nlink)
		}
	}
	if log := read(filepath.Join("/var/log/libvirt/qemu", domain.Name()+".log"), 16<<10); len(log) > 0 {
		t.Log("synthetic domain log", string(log))
	}
}
