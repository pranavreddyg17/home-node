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
		})
	}
}
