//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

type reservedResourceIntent struct {
	Volume  VolumeOwnershipIntent
	Channel ChannelOwnershipIntent
}

// prepareReservedResources composes both durable preparation lifecycles. Its
// caller must retain runtime exclusion. Success requalifies both guest-owned
// inodes under that barrier; it does not create sockets or authorize guest activation.
func (m *Manager) prepareReservedResources(ctx context.Context, d Domain, stopped func(context.Context) error) (reservedResourceIntent, error) {
	if m == nil || stopped == nil || d.DataPath != filepath.Join(m.Volumes, d.ID+".raw") || d.ChannelPath != filepath.Join(m.Channels, d.ID, "adapter.sock") || d.DiskReserveBytes != m.Policy.DiskReserveBytes {
		return reservedResourceIntent{}, ErrPolicy
	}
	volume, err := m.prepareReservedVolume(ctx, m.Volumes, d, d.DiskReserveBytes, stopped)
	if err != nil {
		return reservedResourceIntent{}, err
	}
	channel, err := m.prepareReservedChannel(ctx, m.Channels, d, stopped)
	if err != nil {
		return reservedResourceIntent{}, err
	}
	guestOwned := true
	err = m.withReservedVolumeOwnership(ctx, m.Volumes, d, stopped, func(ctx context.Context, file *os.File, current VolumeOwnershipIntent, volumeGuard func(context.Context) error) error {
		if current != volume {
			return ErrPolicy
		}
		return m.withReservedChannelEntry(ctx, filepath.Dir(d.ChannelPath), d, func(ctx context.Context) error { return volumeGuard(ctx) }, func(ctx context.Context, directory *os.File, current ChannelOwnershipIntent, channelGuard func(context.Context) error) error {
			if current != channel {
				return ErrPolicy
			}
			return errors.Join(volumeGuard(ctx), channelGuard(ctx))
		}, false, &guestOwned)
	}, &guestOwned, false)
	if err != nil {
		return reservedResourceIntent{}, err
	}
	return reservedResourceIntent{Volume: volume, Channel: channel}, nil
}
