//go:build !linux

package backup

import "context"

func (*Repository) InspectSnapshot(context.Context, string, RestorePolicy) (Manifest, error) {
	return Manifest{}, ErrRepository
}
