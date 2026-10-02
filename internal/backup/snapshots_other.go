//go:build !linux

package backup

import "context"

func (*Repository) Snapshots(context.Context) ([]SnapshotReference, error) {
	return nil, ErrRepository
}
