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
	path := filepath.Join(t.TempDir(), "guest.raw")
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
	if err = os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err = file.Chmod(0640); err != nil {
		t.Fatal(err)
	}
	if err = admitVolumeForUID(file, size, 200000); !errors.Is(err, ErrPolicy) {
		t.Fatal("permissive guest volume admitted", err)
	}
	if err = file.Chmod(0600); err != nil {
		t.Fatal(err)
	}
	if err = admitVolumeForUID(file, size, 200000); err != nil {
		t.Fatal(err)
	}
	for _, uid := range []uint32{1, 65535, 1 << 31} {
		if err = admitVolumeForUID(file, size, uid); !errors.Is(err, ErrPolicy) {
			t.Fatal("unreserved UID accepted", uid, err)
		}
	}
}
