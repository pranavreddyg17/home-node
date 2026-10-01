package backup

import (
	"context"
	"os"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// QualifiedMaintenanceDisks retains the source bridge's stopped-runtime lease
// across filesystem checking and copying. Production staging must use this
// adapter rather than a raw descriptor bridge. Qualification never grants a
// lease or excludes writers independently of the wrapped source.
type QualifiedMaintenanceDisks struct{ Source MaintenanceDisks }

func (d QualifiedMaintenanceDisks) WithMaintenanceDisk(ctx context.Context, token, id string, copyDisk func(context.Context, *os.File, supervisor.Instance) error) error {
	if d.Source == nil || copyDisk == nil {
		return ErrManifest
	}
	return d.Source.WithMaintenanceDisk(ctx, token, id, func(ctx context.Context, file *os.File, instance supervisor.Instance) error {
		if instance.ID != id || instance.State != "stopped" || instance.Desired != "stopped" {
			return ErrManifest
		}
		if err := QualifyExt4Disk(ctx, file); err != nil {
			return err
		}
		return copyDisk(ctx, file, instance)
	})
}
