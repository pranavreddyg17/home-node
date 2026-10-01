//go:build linux

package backup

import (
	"bytes"
	"context"
	"os"

	"golang.org/x/sys/unix"
)

// CreateRepositoryPassword creates an anonymous immutable credential descriptor.
// Trusted handoff must authenticate its recipient; no dispatch message carries
// this credential. Caller owns descriptor closure and clearing its input bytes.
func CreateRepositoryPassword(password []byte) (*os.File, error) { return passwordDescriptor(password) }

// ReadRepositoryPassword accepts only an anonymous, sealed, close-on-exec
// regular descriptor with a bounded immutable payload. It does not seek or
// change the shared offset. Caller owns descriptor closure and clearing returned
// bytes. Disk files, pipes, mutable memory files, and invalid payloads refuse.
func ReadRepositoryPassword(ctx context.Context, file *os.File) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if file == nil {
		return nil, ErrRepository
	}
	info, err := file.Stat()
	var native unix.Stat_t
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 8192 || unix.Fstat(int(file.Fd()), &native) != nil || native.Nlink != 0 {
		return nil, ErrRepository
	}
	seals, err := unix.FcntlInt(file.Fd(), unix.F_GET_SEALS, 0)
	required := unix.F_SEAL_WRITE | unix.F_SEAL_GROW | unix.F_SEAL_SHRINK | unix.F_SEAL_SEAL
	flags, flagErr := unix.FcntlInt(file.Fd(), unix.F_GETFD, 0)
	if err != nil || seals&required != required || flagErr != nil || flags&unix.FD_CLOEXEC == 0 {
		return nil, ErrRepository
	}
	data := make([]byte, int(info.Size()))
	n, err := file.ReadAt(data, 0)
	if err != nil || n != len(data) || bytes.ContainsAny(data, "\x00\r\n") {
		clear(data)
		return nil, ErrRepository
	}
	if err = ctx.Err(); err != nil {
		clear(data)
		return nil, err
	}
	return data, nil
}
