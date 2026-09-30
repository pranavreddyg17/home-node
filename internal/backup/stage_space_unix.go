//go:build linux || darwin

package backup

import (
	"golang.org/x/sys/unix"
	"os"
)

func requireStagingSpace(directory *os.File, remaining int64) error {
	var space unix.Statfs_t
	if directory == nil || unix.Fstatfs(int(directory.Fd()), &space) != nil {
		return ErrStagingCapacity
	}
	return stagingCapacity(space.Bavail, int64(space.Bsize), remaining)
}
