package supervisor

import "strconv"

type qemuOutputPipe struct {
	FD       int
	Target   string
	Identity qemuRegularIdentity
	Access   uint32
}

// Output-pipe shape only. Matching pipe inodes do not identify the reader or
// authenticate the ultimate log sink. No pipe is opened, read or written here.
func qualifyQEMUOutputPipes(stdout, stderr qemuOutputPipe) error {
	if stdout.FD != 1 || stderr.FD != 2 || stdout.Identity != stderr.Identity || stdout.Target != stderr.Target {
		return ErrPolicy
	}
	s := stdout.Identity
	if s.Device == 0 || s.Inode == 0 || s.Links != 1 || s.UID != 0 || s.GID != 0 || s.Mode != 0010000|0600 || stdout.Access != 1 || stderr.Access != 1 || stdout.Target != "pipe:["+strconv.FormatUint(s.Inode, 10)+"]" {
		return ErrPolicy
	}
	return nil
}
