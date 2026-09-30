//go:build !linux && !darwin

package backup

import "os"

func requireStagingSpace(*os.File, int64) error { return ErrStagingCapacity }
