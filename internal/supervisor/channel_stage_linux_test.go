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

func testReservedChannelStage(t *testing.T) {
	for _, fault := range []string{"none", "occupied-stage", "occupied-final", "before-intent", "after-intent"} {
		t.Run(fault, func(t *testing.T) {
			m, _ := newManager(t)
			pool := GuestUIDPool{First: 1000000000, Last: 1000000001}
			m.GuestUIDPool, m.GuestGID = &pool, 64054
			m.Backend = LinuxBackend{TransferGID: 64055}
			ctx := context.Background()
			d := Domain{ID: state.Random()}
			d.Image.SHA256 = strings.Repeat("a", 64)
			if err := m.bindDomainGuestIdentity(ctx, &d, true); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'files','preparing','running',?,256,1,?,0,0)`, d.ID, d.Image.SHA256, 16<<20); err != nil {
				t.Fatal(err)
			}
			parent := t.TempDir()
			stage := filepath.Join(parent, "."+d.ID+".channel-prepare")
			final := filepath.Join(parent, d.ID)
			occupied := stage
			if fault == "occupied-final" {
				occupied = final
			}
			if strings.HasPrefix(fault, "occupied-") {
				if err := os.Mkdir(occupied, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(occupied, "unknown"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			interruption := errors.New("channel staging exclusion lost")
			intent, err := m.stageReservedChannel(ctx, parent, d, func(ctx context.Context) error {
				calls++
				if fault == "before-intent" && calls == 2 || fault == "after-intent" && calls == 3 {
					return interruption
				}
				return ctx.Err()
			})
			var records int
			if countErr := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_channel_ownership`).Scan(&records); countErr != nil {
				t.Fatal(countErr)
			}
			if fault != "none" {
				if err == nil || intent != (ChannelOwnershipIntent{}) {
					t.Fatal("failed stage reported authority", fault, intent, err)
				}
				if strings.HasPrefix(fault, "occupied-") {
					if !errors.Is(err, ErrPolicy) || records != 0 {
						t.Fatal("occupied path adopted", fault, records, err)
					}
					if data, err := os.ReadFile(filepath.Join(occupied, "unknown")); err != nil || string(data) != "preserve" {
						t.Fatal("occupied contents changed", err)
					}
					return
				}
				if !errors.Is(err, interruption) || fault == "before-intent" && records != 0 || fault == "after-intent" && records != 1 {
					t.Fatal("staging interruption lost journal boundary", fault, records, err)
				}
			} else if err != nil || records != 1 {
				t.Fatal("fresh stage refused", intent, records, err)
			}
			var pinned unix.Stat_t
			if unix.Lstat(stage, &pinned) != nil || pinned.Uid != 0 || pinned.Gid != 0 || pinned.Mode&07777 != 0710 || pinned.Mode&unix.S_IFMT != unix.S_IFDIR {
				t.Fatal("private stage not preserved")
			}
			if _, err := os.Lstat(final); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("staging published final directory", err)
			}
			if records == 1 {
				saved, err := m.loadChannelOwnershipIntent(ctx, d)
				if err != nil || saved.Device != uint64(pinned.Dev) || saved.Inode != pinned.Ino {
					t.Fatal("stage receipt does not name retained inode", saved, err)
				}
			}
			if retried, err := m.stageReservedChannel(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() }); err == nil || retried != (ChannelOwnershipIntent{}) {
				t.Fatal("fresh staging adopted existing effects", retried, err)
			}
			if records == 0 {
				if got, err := m.publishReservedChannel(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() }); err == nil || got != (ChannelOwnershipIntent{}) {
					t.Fatal("publication adopted unrecorded stage", got, err)
				}
				return
			}
			if err := os.Mkdir(final, 0700); err != nil {
				t.Fatal(err)
			}
			if got, err := m.publishReservedChannel(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() }); !errors.Is(err, ErrPolicy) || got != (ChannelOwnershipIntent{}) {
				t.Fatal("publication replaced occupied destination", got, err)
			}
			if err := os.Remove(final); err != nil {
				t.Fatal(err)
			}
			displaced := stage + ".displaced"
			if err := os.Rename(stage, displaced); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(stage, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(stage, 0710); err != nil {
				t.Fatal(err)
			}
			if got, err := m.publishReservedChannel(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() }); !errors.Is(err, ErrPolicy) || got != (ChannelOwnershipIntent{}) {
				t.Fatal("publication adopted replacement stage inode", got, err)
			}
			if err := errors.Join(os.Remove(stage), os.Rename(displaced, stage)); err != nil {
				t.Fatal("restore replaced stage fixture", err)
			}
			publicationChecks := 0
			got, err := m.publishReservedChannel(ctx, parent, d, func(ctx context.Context) error {
				publicationChecks++
				if publicationChecks == 6 {
					return interruption
				}
				return ctx.Err()
			})
			if !errors.Is(err, interruption) || got != (ChannelOwnershipIntent{}) {
				t.Fatal("interrupted publication reported success", publicationChecks, got, err)
			}
			var published unix.Stat_t
			if unix.Lstat(final, &published) != nil || published.Dev != pinned.Dev || published.Ino != pinned.Ino || published.Uid != 0 || published.Gid != 0 || published.Mode != pinned.Mode {
				t.Fatal("interrupted publication lost journaled directory inode")
			}
			if _, err := os.Lstat(stage); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("completed publication retained ambiguous stage", err)
			}
			for retry := 0; retry < 2; retry++ {
				got, err := m.publishReservedChannel(ctx, parent, d, func(ctx context.Context) error { return ctx.Err() })
				if err != nil || got.Device != uint64(pinned.Dev) || got.Inode != pinned.Ino {
					t.Fatal("completed publication retry", retry, got, err)
				}
			}
		})
	}
}
