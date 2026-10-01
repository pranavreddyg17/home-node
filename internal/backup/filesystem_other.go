//go:build !linux

package backup

import (
	"context"
	"os"
)

func QualifyExt4Disk(ctx context.Context, source *os.File) error { return ErrManifest }
