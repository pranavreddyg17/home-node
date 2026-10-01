//go:build linux

package updates

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// PublishEnvironment writes the fixed service configuration without replacing
// any existing launch state. The root coordinator must supply the verified
// installer-owned update parent. Publication activates no service; interrupted
// or existing launch state requires explicit recovery.
func (s *InspectionStage) PublishEnvironment(ctx context.Context, parent *os.Root) error {
	if os.Geteuid() != 0 {
		return ErrInspectionResult
	}
	return s.publishEnvironmentOwned(ctx, parent)
}

func (s *InspectionStage) publishEnvironmentOwned(ctx context.Context, parent *os.Root) (resultErr error) {
	if s == nil || parent == nil {
		return ErrInspectionResult
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.packageFile == nil || s.directory == nil {
		return ErrInspectionResult
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	directory, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	var stat unix.Stat_t
	if unix.Fstat(int(directory.Fd()), &stat) != nil || stat.Mode != unix.S_IFDIR|0700 || stat.Uid != uint32(os.Geteuid()) || stat.Gid != uint32(os.Getegid()) {
		return ErrInspectionResult
	}
	// Bind publication to this parent's fixed staging inode, rather than allowing
	// a valid stage to configure an unrelated service package path.
	child, err := parent.OpenFile("inspection", os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	childInfo, childErr := child.Stat()
	stageInfo, stageErr := s.directory.Stat()
	closeErr := child.Close()
	if childErr != nil || stageErr != nil || closeErr != nil || !os.SameFile(childInfo, stageInfo) {
		return errors.Join(ErrInspectionResult, childErr, stageErr, closeErr)
	}
	digest, length, err := PackageIdentity(ctx, s.packageFile)
	if err != nil || digest != s.identity.PackageSHA256 || length != s.identity.PackageLength {
		return errors.Join(ErrInspectionResult, err)
	}
	data, err := InspectionEnvironment(s.identity)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := parent.Lstat("inspection.env"); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrInspectionResult, err)
	}
	// Publish only a synchronized complete record. A crash or cancellation before
	// rename retains pending state and never exposes partial service inputs.
	if err := writeInitializationFile(parent, "inspection.env.pending", data); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := unix.Renameat2(int(directory.Fd()), "inspection.env.pending", int(directory.Fd()), "inspection.env", unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	return errors.Join(directory.Sync(), ctx.Err())
}
