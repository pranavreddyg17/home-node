//go:build linux || darwin

package updates

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

var errUpdateClock = errors.New("update clock checkpoint is missing, unsafe or ahead of the host clock")

// recordUpdateClock persists the exact TUF reference time before any network
// refresh. Caller holds the exclusive cache lock. This detects observed clock
// rollback, not a frozen clock or malicious root changing protected state.
func recordUpdateClock(ctx context.Context, provisioned *os.Root, reference time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	next := reference.UnixNano()
	if provisioned == nil || next <= 0 {
		return errUpdateClock
	}
	expected, inspectErr := provisioned.Lstat("clock")
	if inspectErr != nil && !os.IsNotExist(inspectErr) {
		return errors.Join(errUpdateClock, inspectErr)
	}
	if expected != nil && !expected.Mode().IsRegular() {
		return errUpdateClock
	}
	checkpoint, err := provisioned.OpenFile("clock", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err == nil {
		info, statErr := checkpoint.Stat()
		var native unix.Stat_t
		if statErr != nil || expected == nil || !os.SameFile(expected, info) || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() < 2 || info.Size() > 21 || unix.Fstat(int(checkpoint.Fd()), &native) != nil || native.Uid != uint32(os.Geteuid()) || native.Nlink != 1 {
			checkpoint.Close()
			return errUpdateClock
		}
		data, readErr := io.ReadAll(io.LimitReader(checkpoint, 22))
		closeErr := checkpoint.Close()
		if err = errors.Join(readErr, closeErr); err != nil {
			return err
		}
		value := string(data)
		if len(value) < 2 || value[len(value)-1] != '\n' {
			return errUpdateClock
		}
		previous, parseErr := strconv.ParseInt(value[:len(value)-1], 10, 64)
		if parseErr != nil || previous <= 0 || strconv.FormatInt(previous, 10)+"\n" != value || next < previous {
			return errUpdateClock
		}
	} else if os.IsNotExist(err) {
		if expected != nil {
			return errUpdateClock
		}
		// Existing refreshed metadata without its clock checkpoint needs explicit
		// migration; never establish a new baseline that hides lost clock evidence.
		for _, name := range []string{"timestamp.json", "snapshot.json", "targets.json"} {
			if _, err = provisioned.Lstat("metadata/" + name); !os.IsNotExist(err) {
				return errUpdateClock
			}
		}
	} else {
		return errors.Join(errUpdateClock, err)
	}
	file, err := provisioned.OpenFile("clock.pending", os.O_CREATE|os.O_EXCL|os.O_WRONLY|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return errors.Join(errUpdateClock, err)
	}
	// An interrupted write remains explicit intent for repair. Never unlink an
	// unknown checkpoint or silently reset a previous successful baseline.
	_, writeErr := file.WriteString(strconv.FormatInt(next, 10) + "\n")
	syncErr := file.Sync()
	if err = errors.Join(writeErr, syncErr, file.Close()); err != nil {
		return err
	}
	if err = provisioned.Rename("clock.pending", "clock"); err != nil {
		return err
	}
	directory, err := provisioned.Open(".")
	if err != nil {
		return err
	}
	err = errors.Join(directory.Sync(), directory.Close())
	return errors.Join(err, ctx.Err())
}
