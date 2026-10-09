//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func testReservedVolumeStage(t *testing.T) {
	for _, fault := range []string{"none", "occupied", "before-format", "after-intent", "composed-fresh", "late-destination"} {
		t.Run(fault, func(t *testing.T) {
			m, _ := newManager(t)
			pool := GuestUIDPool{First: 1000000000, Last: 1000000001}
			m.GuestUIDPool, m.GuestGID = &pool, 64054
			ctx := context.Background()
			d := Domain{ID: state.Random()}
			d.Image.SHA256, d.Image.DataBytes = strings.Repeat("a", 64), 16<<20
			if err := m.bindDomainGuestIdentity(ctx, &d, true); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','preparing','running',?,256,1,?,0,0)`, d.ID, d.Image.SHA256, d.Image.DataBytes); err != nil {
				t.Fatal(err)
			}
			parent := volumeFixtureDir(t)
			if err := os.Chown(parent, 0, int(d.GuestGID)); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(parent, 0710); err != nil {
				t.Fatal(err)
			}
			stage := filepath.Join(parent, "."+d.ID+".volume-prepare")
			if fault == "occupied" {
				if err := os.WriteFile(stage, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if fault == "composed-fresh" {
				m.Backend = LinuxBackend{TransferGID: 64055}
				m.Volumes, m.Channels = parent, volumeFixtureDir(t)
				d.DataPath = filepath.Join(parent, d.ID+".raw")
				d.ChannelPath = filepath.Join(m.Channels, d.ID, "adapter.sock")
				d.DiskReserveBytes = m.Policy.DiskReserveBytes
				final := filepath.Join(parent, d.ID+".raw")
				interrupted := errors.New("composed volume transfer interrupted")
				observed := false
				got, err := m.prepareReservedResources(ctx, d, func(ctx context.Context) error {
					var disk unix.Stat_t
					if unix.Lstat(final, &disk) == nil && disk.Uid == d.GuestUID && !observed {
						observed = true
						return interrupted
					}
					return ctx.Err()
				})
				if !observed || !errors.Is(err, interrupted) || got != (reservedResourceIntent{}) {
					t.Fatal("composed transfer boundary", observed, got, err)
				}
				saved, err := m.loadVolumeOwnershipIntent(ctx, d)
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.OpenFile(final, os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				marker := []byte("preserve guest data across retry")
				if _, err := file.WriteAt(marker, 8<<20); err != nil {
					t.Fatal(err)
				}
				if err := errors.Join(file.Sync(), file.Close()); err != nil {
					t.Fatal(err)
				}
				for retry := 0; retry < 2; retry++ {
					got, err = m.prepareReservedResources(ctx, d, func(ctx context.Context) error { return ctx.Err() })
					if err != nil || got.Volume != saved || got.Channel.InstanceID != d.ID {
						t.Fatal("composed volume retry", retry, got, err)
					}
				}
				// Observe a complete retry, then invalidate the channel during
				// its last runtime check. The coupled final fence must catch it.
				completeChecks := 0
				if _, err := m.prepareReservedResources(ctx, d, func(ctx context.Context) error { completeChecks++; return ctx.Err() }); err != nil {
					t.Fatal(err)
				}
				faultChecks := 0
				channelDirectory := filepath.Dir(d.ChannelPath)
				got, err = m.prepareReservedResources(ctx, d, func(ctx context.Context) error {
					faultChecks++
					if faultChecks == completeChecks {
						return os.Chown(channelDirectory, 0, 0)
					}
					return ctx.Err()
				})
				if faultChecks != completeChecks || !errors.Is(err, ErrPolicy) || got != (reservedResourceIntent{}) {
					t.Fatal("final coupled ownership fence", completeChecks, faultChecks, got, err)
				}
				if got, err = m.prepareReservedResources(ctx, d, func(ctx context.Context) error { return ctx.Err() }); err != nil || got.Volume != saved {
					t.Fatal("coupled ownership retry", got, err)
				}
				var disk unix.Stat_t
				if unix.Lstat(final, &disk) != nil || disk.Uid != saved.UID || disk.Gid != saved.GID || disk.Ino != saved.Inode || uint64(disk.Dev) != saved.Device {
					t.Fatal("completed volume lost ownership or inode")
				}
				file, err = os.Open(final)
				if err != nil {
					t.Fatal(err)
				}
				actual := make([]byte, len(marker))
				_, readErr := file.ReadAt(actual, 8<<20)
				if closeErr := file.Close(); readErr != nil || closeErr != nil || string(actual) != string(marker) {
					t.Fatal("retry changed guest data", readErr, closeErr)
				}
				return
			}
			calls := 0
			interruption := errors.New("volume staging interrupted")
			got, err := m.stageReservedVolume(ctx, parent, d, 4<<30, func(ctx context.Context) error {
				calls++
				if fault == "late-destination" && calls == 2 {
					return os.WriteFile(filepath.Join(parent, d.ID+".raw"), []byte("preserve late destination"), 0600)
				}
				if fault == "before-format" && calls == 2 || fault == "after-intent" && calls == 4 {
					return interruption
				}
				return ctx.Err()
			})
			var count int
			if countErr := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_volume_ownership`).Scan(&count); countErr != nil {
				t.Fatal(countErr)
			}
			if fault == "occupied" {
				if !errors.Is(err, ErrPolicy) || got != (VolumeOwnershipIntent{}) || count != 0 {
					t.Fatal("occupied volume adopted", got, count, err)
				}
				if data, err := os.ReadFile(stage); err != nil || string(data) != "preserve" {
					t.Fatal("occupied volume changed", err)
				}
				return
			}
			if fault == "late-destination" {
				if !errors.Is(err, ErrPolicy) || got != (VolumeOwnershipIntent{}) || count != 0 {
					t.Fatal("late destination admitted", got, count, err)
				}
				if data, err := os.ReadFile(filepath.Join(parent, d.ID+".raw")); err != nil || string(data) != "preserve late destination" {
					t.Fatal("late destination changed", err)
				}
				file, err := os.Open(stage)
				if err != nil {
					t.Fatal(err)
				}
				magic := make([]byte, 2)
				_, readErr := file.ReadAt(magic, 1024+56)
				if closeErr := file.Close(); readErr != nil || closeErr != nil || magic[0] != 0 || magic[1] != 0 {
					t.Fatal("namespace refusal formatted partial disk", readErr, closeErr, magic)
				}
				return
			}
			if fault == "none" {
				if err != nil || count != 1 {
					t.Fatal("fresh volume staging", got, count, err)
				}
			} else if !errors.Is(err, interruption) || got != (VolumeOwnershipIntent{}) || fault == "before-format" && count != 0 || fault == "after-intent" && count != 1 {
				t.Fatal("staging fault boundary", fault, got, count, err)
			}
			var disk unix.Stat_t
			if unix.Lstat(stage, &disk) != nil || disk.Uid != 0 || disk.Gid != 0 || disk.Size != d.Image.DataBytes || disk.Mode&07777 != 0600 {
				t.Fatal("staged disk not preserved")
			}
			if got, err := m.stageReservedVolume(ctx, parent, d, 4<<30, func(ctx context.Context) error { return ctx.Err() }); err == nil || got != (VolumeOwnershipIntent{}) {
				t.Fatal("fresh retry adopted staged disk", got, err)
			}
			if count == 0 {
				if got, err := m.prepareReservedVolume(ctx, parent, d, 4<<30, func(ctx context.Context) error { return ctx.Err() }); err == nil || got != (VolumeOwnershipIntent{}) {
					t.Fatal("composed retry adopted unrecorded data", got, err)
				}
				return
			}
			file, err := os.Open(stage)
			if err != nil {
				t.Fatal(err)
			}
			magic := make([]byte, 2)
			_, readErr := file.ReadAt(magic, 1024+56)
			if closeErr := file.Close(); readErr != nil || closeErr != nil || magic[0] != 0x53 || magic[1] != 0xef {
				t.Fatal("recorded disk lacks ext4 superblock", readErr, closeErr, magic)
			}
			published, err := m.prepareReservedVolume(ctx, parent, d, 4<<30, func(ctx context.Context) error { return ctx.Err() })
			if err != nil || published.Inode != disk.Ino || published.Device != uint64(disk.Dev) {
				t.Fatal("formatted inode publication", published, err)
			}
		})
	}
}
