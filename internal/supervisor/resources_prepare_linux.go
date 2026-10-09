//go:build linux

package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	if m == nil || stopped == nil || d.SystemPath != filepath.Join(m.Images, d.Image.SHA256+".raw") || d.DataPath != filepath.Join(m.Volumes, d.ID+".raw") || d.ChannelPath != filepath.Join(m.Channels, d.ID, "adapter.sock") || d.DiskReserveBytes != m.Policy.DiskReserveBytes {
		return reservedResourceIntent{}, ErrPolicy
	}
	if err := m.checkVolumePreparationDomain(ctx, d); err != nil {
		return reservedResourceIntent{}, err
	}
	var prepared reservedResourceIntent
	complete := false
	imageStopped := func(ctx context.Context) error {
		if complete {
			return m.qualifyReservedResources(ctx, d, prepared, stopped)
		}
		return stopped(ctx)
	}
	err := withReservedSystemImage(ctx, m.Images, d, imageStopped, func(ctx context.Context, image *os.File, imageGuard func(context.Context) error) error {
		oldSocket, err := m.loadChannelSocketRetirementIntent(ctx, d)
		if err == nil {
			retired, retireErr := m.retireChannelSocket(ctx, m.Channels, d, oldSocket, imageGuard)
			if retireErr != nil || retired != oldSocket {
				return errors.Join(ErrPolicy, retireErr)
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		volume, err := m.prepareReservedVolume(ctx, m.Volumes, d, d.DiskReserveBytes, imageGuard)
		if err != nil {
			return fmt.Errorf("prepare reserved data volume: %w", err)
		}
		channel, err := m.prepareReservedChannel(ctx, m.Channels, d, imageGuard)
		if err != nil {
			return fmt.Errorf("prepare reserved channel: %w", err)
		}
		prepared = reservedResourceIntent{Volume: volume, Channel: channel}
		complete = true
		return imageGuard(ctx)
	})
	if err != nil {
		return reservedResourceIntent{}, err
	}
	return prepared, nil
}

func (m *Manager) qualifyReservedResources(ctx context.Context, d Domain, intent reservedResourceIntent, stopped func(context.Context) error) error {
	guestOwned := true
	// Each volume runtime check first requalifies the channel with its own
	// retained scope. Channel checks call only the original exclusion guard,
	// avoiding recursive cross-guards and covering the volume's final guard.
	pairedStopped := func(ctx context.Context) error {
		return m.withReservedChannelEntry(ctx, filepath.Dir(d.ChannelPath), d, stopped, func(ctx context.Context, directory *os.File, current ChannelOwnershipIntent, guard func(context.Context) error) error {
			if current != intent.Channel {
				return ErrPolicy
			}
			return guard(ctx)
		}, false, &guestOwned)
	}
	return m.withReservedVolumeOwnership(ctx, m.Volumes, d, pairedStopped, func(ctx context.Context, file *os.File, current VolumeOwnershipIntent, guard func(context.Context) error) error {
		if current != intent.Volume {
			return ErrPolicy
		}
		return guard(ctx)
	}, &guestOwned, false)
}
