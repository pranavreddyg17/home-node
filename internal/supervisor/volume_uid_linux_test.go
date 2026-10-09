//go:build linux

package supervisor

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeGuestUIDVolumeAdmission(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VOLUME_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	path := filepath.Join(volumeFixtureDir(t), "guest.raw")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	const size = 16 << 20
	if err = file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		t.Fatal(err)
	}
	intent := VolumeOwnershipIntent{InstanceID: state.Random(), ImageSHA256: strings.Repeat("a", 64), UID: 200000, GID: 200000, Device: uint64(stat.Dev), Inode: stat.Ino, Size: size}
	m, _ := newManager(t)
	pool := GuestUIDPool{First: 200000, Last: 200002}
	m.GuestUIDPool, m.GuestGID = &pool, intent.GID
	d := Domain{ID: intent.InstanceID}
	d.Image.SHA256, d.Image.DataBytes = intent.ImageSHA256, size
	if err := m.bindDomainGuestIdentity(context.Background(), &d, true); err != nil {
		t.Fatal(err)
	}
	if d.GuestUID != intent.UID {
		t.Fatal("fixture identity drift")
	}
	if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','preparing','running',?,256,1,?,0,0)`, d.ID, d.Image.SHA256, size); err != nil {
		t.Fatal(err)
	}
	if _, err := m.verifyPinnedVolumeOwnership(context.Background(), d, file); !errors.Is(err, ErrPolicy) {
		t.Fatal("missing descriptor provenance verified", err)
	}
	recorded, err := m.recordPinnedVolumeOwnership(context.Background(), d, file)
	if err != nil || recorded != intent {
		t.Fatal("descriptor intent recording", recorded, err)
	}
	verified, err := m.verifyPinnedVolumeOwnership(context.Background(), d, file)
	if err != nil || verified != recorded {
		t.Fatal("descriptor provenance verification", verified, err)
	}
	if err := m.verifyVolumeOwnershipIntent(context.Background(), recorded); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*VolumeOwnershipIntent){func(i *VolumeOwnershipIntent) { i.Inode++ }, func(i *VolumeOwnershipIntent) { i.Device++ }, func(i *VolumeOwnershipIntent) { i.Size++ }, func(i *VolumeOwnershipIntent) { i.InstanceID = "invalid" }, func(i *VolumeOwnershipIntent) { i.ImageSHA256 = "invalid" }} {
		changed := intent
		change(&changed)
		if err := transferVolumeToGuest(context.Background(), file, changed); !errors.Is(err, ErrPolicy) {
			t.Fatal("unbound intent admitted", changed, err)
		}
		var unchanged unix.Stat_t
		if err := unix.Fstat(int(file.Fd()), &unchanged); err != nil || unchanged.Uid != 0 || unchanged.Gid != 0 || unchanged.Ino != stat.Ino || unchanged.Mode != stat.Mode {
			t.Fatal("refusal changed metadata", err)
		}
	}
	other, err := os.OpenFile(filepath.Join(filepath.Dir(path), "other.raw"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if _, err := m.verifyPinnedVolumeOwnership(context.Background(), d, other); !errors.Is(err, ErrPolicy) {
		t.Fatal("replacement provenance verified", err)
	}
	if _, err := m.recordPinnedVolumeOwnership(context.Background(), d, other); err == nil {
		t.Fatal("replacement descriptor rewrote provenance")
	}
	if err := transferVolumeToGuest(context.Background(), other, intent); !errors.Is(err, ErrPolicy) {
		t.Fatal("replacement inode admitted", err)
	}
	var otherStat unix.Stat_t
	if err := unix.Fstat(int(other.Fd()), &otherStat); err != nil || otherStat.Uid != 0 || otherStat.Gid != 0 || otherStat.Mode&0777 != 0600 {
		t.Fatal("replacement refusal changed metadata", err)
	}
	transfer := func(ctx context.Context, file *os.File, size int64, uid, gid uint32) error {
		requested := intent
		requested.Size = size
		requested.UID = uid
		requested.GID = gid
		return transferVolumeToGuest(ctx, file, requested)
	}
	if err = transfer(context.Background(), file, size, 200000, 0); !errors.Is(err, ErrPolicy) {
		t.Fatal("root guest group accepted", err)
	}
	if err = transfer(context.Background(), file, size, 200000, 200000); err != nil {
		t.Fatal(err)
	}
	if err = transfer(context.Background(), file, size, 200000, 200000); err != nil {
		t.Fatal("ownership retry refused", err)
	}
	retried, err := m.recordPinnedVolumeOwnership(context.Background(), d, file)
	if err != nil || retried != recorded {
		t.Fatal("guest-owned descriptor retry", retried, err)
	}
	if err = transfer(context.Background(), file, size, 200001, 200001); !errors.Is(err, ErrPolicy) {
		t.Fatal("another guest took ownership", err)
	}
	if err = transfer(context.Background(), file, size, 200000, 200001); !errors.Is(err, ErrPolicy) {
		t.Fatal("changed guest group accepted", err)
	}
	if err = transfer(context.Background(), file, size, 200000, 200000); err != nil {
		t.Fatal("refused group change modified ownership", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = transfer(ctx, file, size, 200000, 200000); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled ownership ignored", err)
	}
	if err = admitVolumeForUID(file, size, 200000); err != nil {
		t.Fatal("reserved guest ownership refused", err)
	}
	if err = admitVolume(file, size); !errors.Is(err, ErrPolicy) {
		t.Fatal("guest volume admitted as root-owned", err)
	}
	if err = admitVolumeForUID(file, size, 200001); !errors.Is(err, ErrPolicy) {
		t.Fatal("other guest ownership admitted", err)
	}
	alias := path + ".alias"
	if err = os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if err = admitVolumeForUID(file, size, 200000); !errors.Is(err, ErrPolicy) {
		t.Fatal("aliased guest volume admitted", err)
	}
	for _, check := range []func(context.Context, Domain, *os.File) (VolumeOwnershipIntent, error){m.recordPinnedVolumeOwnership, m.verifyPinnedVolumeOwnership} {
		if got, err := check(context.Background(), d, file); !errors.Is(err, ErrPolicy) || got != (VolumeOwnershipIntent{}) {
			t.Fatal("aliased provenance admitted", got, err)
		}
	}
	if err = os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err = file.Chmod(0640); err != nil {
		t.Fatal(err)
	}
	if err = admitVolumeForUID(file, size, 200000); !errors.Is(err, ErrPolicy) {
		t.Fatal("permissive guest volume admitted", err)
	}
	for _, check := range []func(context.Context, Domain, *os.File) (VolumeOwnershipIntent, error){m.recordPinnedVolumeOwnership, m.verifyPinnedVolumeOwnership} {
		if got, err := check(context.Background(), d, file); !errors.Is(err, ErrPolicy) || got != (VolumeOwnershipIntent{}) {
			t.Fatal("permissive provenance admitted", got, err)
		}
	}
	if err = file.Chmod(0600); err != nil {
		t.Fatal(err)
	}
	if err = admitVolumeForUID(file, size, 200000); err != nil {
		t.Fatal(err)
	}
	for _, check := range []func(context.Context, Domain, *os.File) (VolumeOwnershipIntent, error){m.recordPinnedVolumeOwnership, m.verifyPinnedVolumeOwnership} {
		if got, err := check(ctx, d, file); !errors.Is(err, context.Canceled) || got != (VolumeOwnershipIntent{}) {
			t.Fatal("cancelled provenance admitted", got, err)
		}
		if got, err := check(context.Background(), d, nil); !errors.Is(err, ErrPolicy) || got != (VolumeOwnershipIntent{}) {
			t.Fatal("nil descriptor admitted", got, err)
		}
	}
	finalIntent, err := m.verifyPinnedVolumeOwnership(context.Background(), d, file)
	if err != nil || finalIntent != recorded {
		t.Fatal("refusals changed provenance", finalIntent, err)
	}
	directory := filepath.Dir(path)
	var originalParent unix.Stat_t
	if err := unix.Stat(directory, &originalParent); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chown(directory, 0, int(originalParent.Gid)); err != nil {
			t.Error(err)
		}
		if err := os.Chmod(directory, os.FileMode(originalParent.Mode&0777)); err != nil {
			t.Error(err)
		}
	}()
	reservedPath := filepath.Join(directory, d.ID+".raw")
	if err := os.Rename(path, reservedPath); err != nil {
		t.Fatal(err)
	}
	if got, err := openReservedVolume(context.Background(), directory, d); err == nil || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("unqualified parent admitted", err)
	}
	if err := os.Chown(directory, 0, int(d.GuestGID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0710); err != nil {
		t.Fatal(err)
	}
	for _, size := range []int64{0, 16<<20 - 1, 512<<30 + 1} {
		invalid := d
		invalid.Image.DataBytes = size
		if got, err := openReservedVolume(context.Background(), directory, invalid); !errors.Is(err, ErrPolicy) || got != nil {
			if got != nil {
				got.Close()
			}
			t.Fatal("invalid reserved volume size admitted", size, err)
		}
	}
	reserved, err := openReservedVolume(context.Background(), directory, d)
	if err != nil {
		t.Fatal("reserved parent/volume refused", err)
	}
	reservedIntent, verifyErr := m.verifyPinnedVolumeOwnership(context.Background(), d, reserved)
	closeErr := reserved.Close()
	if verifyErr != nil || closeErr != nil || reservedIntent != recorded {
		t.Fatal("opened reserved provenance", reservedIntent, verifyErr, closeErr)
	}
	aliasPath := reservedPath + ".alias"
	stoppedChecks := 0
	stopped := func(ctx context.Context) error {
		stoppedChecks++
		return ctx.Err()
	}
	if err := m.withReservedVolume(context.Background(), directory, d, stopped, func(ctx context.Context, pinned *os.File, intent VolumeOwnershipIntent, guard func(context.Context) error) error {
		if intent != recorded {
			t.Fatal("scope changed authenticated intent", intent)
		}
		if err := guard(ctx); err != nil {
			return err
		}
		return transferVolumeToGuest(ctx, pinned, intent)
	}); err != nil || stoppedChecks < 4 {
		t.Fatal("retained reserved scope", stoppedChecks, err)
	}
	consumerCalled := false
	stopFailure := errors.New("guest exclusion lost")
	if err := m.withReservedVolume(context.Background(), directory, d, func(context.Context) error { return stopFailure }, func(context.Context, *os.File, VolumeOwnershipIntent, func(context.Context) error) error {
		consumerCalled = true
		return nil
	}); !errors.Is(err, stopFailure) || consumerCalled {
		t.Fatal("scope ignored stopped guest refusal", consumerCalled, err)
	}
	if err := m.withReservedVolume(context.Background(), directory, d, stopped, func(ctx context.Context, pinned *os.File, intent VolumeOwnershipIntent, guard func(context.Context) error) error {
		return pinned.Chmod(0640)
	}); !errors.Is(err, ErrPolicy) {
		t.Fatal("scope accepted consumer metadata drift", err)
	}
	if err := file.Chmod(0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(reservedPath, aliasPath); err != nil {
		t.Fatal(err)
	}
	if got, err := openReservedVolume(context.Background(), directory, d); !errors.Is(err, ErrPolicy) || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("aliased reserved volume admitted", err)
	}
	if err := os.Remove(aliasPath); err != nil {
		t.Fatal(err)
	}
	if err := file.Chmod(0640); err != nil {
		t.Fatal(err)
	}
	if got, err := openReservedVolume(context.Background(), directory, d); !errors.Is(err, ErrPolicy) || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("permissive reserved volume admitted", err)
	}
	if err := file.Chmod(0600); err != nil {
		t.Fatal(err)
	}
	if err := file.Chown(int(d.GuestUID), int(d.GuestGID+1)); err != nil {
		t.Fatal(err)
	}
	if got, err := openReservedVolume(context.Background(), directory, d); !errors.Is(err, ErrPolicy) || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("foreign disk group admitted", err)
	}
	if err := file.Chown(int(d.GuestUID), int(d.GuestGID)); err != nil {
		t.Fatal(err)
	}
	originalPath := reservedPath + ".original"
	if err := os.Rename(reservedPath, originalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(originalPath, reservedPath); err != nil {
		t.Fatal(err)
	}
	if got, err := openReservedVolume(context.Background(), directory, d); err == nil || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("symlinked reserved volume admitted", err)
	}
	if err := os.Remove(reservedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(originalPath, reservedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(reservedPath, originalPath); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(reservedPath, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := openReservedVolume(context.Background(), directory, d); !errors.Is(err, ErrPolicy) || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("FIFO reserved volume admitted", err)
	}
	if err := os.Remove(reservedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(originalPath, reservedPath); err != nil {
		t.Fatal(err)
	}
	preserved, err := m.verifyPinnedVolumeOwnership(context.Background(), d, file)
	if err != nil || preserved != recorded {
		t.Fatal("reserved opener refusals changed provenance", preserved, err)
	}
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if got, err := openReservedVolume(context.Background(), directory, d); !errors.Is(err, ErrPolicy) || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("permissive reserved parent admitted", err)
	}
	if err := os.Chmod(directory, 0710); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(directory, 0, int(d.GuestGID+1)); err != nil {
		t.Fatal(err)
	}
	if got, err := openReservedVolume(context.Background(), directory, d); !errors.Is(err, ErrPolicy) || got != nil {
		if got != nil {
			got.Close()
		}
		t.Fatal("foreign parent group admitted", err)
	}
	if err := os.Chown(directory, 0, int(d.GuestGID)); err != nil {
		t.Fatal(err)
	}
	for _, uid := range []uint32{1, 65535, 1 << 31} {
		if err = admitVolumeForUID(file, size, uid); !errors.Is(err, ErrPolicy) {
			t.Fatal("unreserved UID accepted", uid, err)
		}
	}
}
