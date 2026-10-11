//go:build linux

package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"reflect"

	"golang.org/x/sys/unix"
)

// Caller holds the installation lock and separately retains the activation
// barrier and owned identity authority. This read-only observer admits exactly
// the recorded inode set at an acknowledged or single pending permission phase.
func (e *Engine) observeGatewayTLSRetirementLocked(ctx context.Context, a *gatewayTLSAdmission, intent gatewayTLSRetirementIntent) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a == nil || len(a.files) != 3 || len(a.Sources) != 3 || len(intent.Sources) != 3 || !reflect.DeepEqual(a.Plan, intent.Plan) || !reflect.DeepEqual(a.Sources, intent.Sources) {
		return 0, ErrConflict
	}
	observed := make([]ConfigurationRemovalFile, 0, 3)
	for index, file := range a.files {
		if file == nil {
			return 0, ErrConflict
		}
		source := intent.Sources[index]
		if err := e.parents(source.Path); err != nil {
			return 0, err
		}
		var stat unix.Stat_t
		if unix.Fstat(int(file.Fd()), &stat) != nil || uint64(stat.Dev) != source.Device || stat.Ino != source.Inode || stat.Uid != 0 || stat.Gid > 1<<31-1 {
			return 0, ErrConflict
		}
		info, err := file.Stat()
		named, nameErr := e.host.Lstat(source.Path)
		directory := index == 2
		if err != nil || nameErr != nil || !os.SameFile(info, named) || info.IsDir() != directory || (!directory && (!info.Mode().IsRegular() || stat.Nlink != 1 || info.Size() != source.Size || info.Size() <= 0 || info.Size() > 16<<10)) {
			return 0, ErrConflict
		}
		attributes := []string{"system.posix_acl_access"}
		if directory {
			attributes = append(attributes, "system.posix_acl_default")
		}
		for _, attribute := range attributes {
			if _, err := unix.Fgetxattr(int(file.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
				return 0, ErrConflict
			}
		}
		metadata := ConfigurationRemovalFile{Path: source.Path, UID: 0, GID: int(stat.Gid), Mode: stat.Mode & 07777}
		if !directory {
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return 0, err
			}
			hash := sha256.New()
			size, err := io.Copy(hash, io.LimitReader(contextReader{ctx, file}, (16<<10)+1))
			if err != nil || size != source.Size {
				return 0, ErrConflict
			}
			metadata.SHA256 = hex.EncodeToString(hash.Sum(nil))
		}
		// Recheck the descriptor and named path after hashing. A changing source
		// cannot be acknowledged merely because its initial metadata was admissible.
		var after unix.Stat_t
		if unix.Fstat(int(file.Fd()), &after) != nil || after.Dev != stat.Dev || after.Ino != stat.Ino || after.Uid != stat.Uid || after.Gid != stat.Gid || after.Mode != stat.Mode || after.Size != stat.Size || after.Nlink != stat.Nlink || after.Mtim != stat.Mtim || after.Ctim != stat.Ctim {
			return 0, ErrConflict
		}
		named, nameErr = e.host.Lstat(source.Path)
		if nameErr != nil || !os.SameFile(info, named) {
			return 0, ErrConflict
		}
		if err := e.parents(source.Path); err != nil {
			return 0, err
		}
		observed = append(observed, metadata)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return gatewayTLSRetirementState(intent.Plan, intent.Completed, intent.Ready, observed)
}
