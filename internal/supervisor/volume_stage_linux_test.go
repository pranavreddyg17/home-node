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
	for _, fault := range []string{"none", "occupied", "before-format", "after-intent"} {
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
			calls := 0
			interruption := errors.New("volume staging interrupted")
			got, err := m.stageReservedVolume(ctx, parent, d, 4<<30, func(ctx context.Context) error {
				calls++
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
			published, err := m.publishReservedVolume(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() })
			if err != nil || published.Inode != disk.Ino || published.Device != uint64(disk.Dev) {
				t.Fatal("formatted inode publication", published, err)
			}
		})
	}
}
