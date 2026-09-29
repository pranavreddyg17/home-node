//go:build !linux

package backup

import (
	"context"
	"os"
)

func (*Repository) Restore(context.Context, string, *os.File, RestorePolicy) (Manifest, error) {
	return Manifest{}, ErrRepository
}
