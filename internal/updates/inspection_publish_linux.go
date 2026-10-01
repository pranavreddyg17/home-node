//go:build linux

package updates

import (
	"bytes"
	"context"
	"errors"
	"io"
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
	if err := s.verifyServicePackagePath(parent); err != nil {
		return err
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

// VerifyEnvironment checks published service inputs against the admitted
// operation before launch. It authenticates configuration, not execution.
func (s *InspectionStage) VerifyEnvironment(ctx context.Context, parent *os.Root) error {
	if os.Geteuid() != 0 {
		return ErrInspectionResult
	}
	return s.verifyEnvironmentOwned(ctx, parent)
}

func (s *InspectionStage) verifyEnvironmentOwned(ctx context.Context, parent *os.Root) (resultErr error) {
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
	if _, err := parent.Lstat("inspection.env.pending"); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrInspectionResult, err)
	}
	file, err := openInspectionFile(parent, "inspection.env", 0600)
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 1025))
	closeErr = file.Close()
	expected, err := InspectionEnvironment(s.identity)
	if err != nil || readErr != nil || closeErr != nil || !bytes.Equal(data, expected) {
		return errors.Join(ErrInspectionResult, err, readErr, closeErr)
	}
	if err := s.verifyServicePackagePath(parent); err != nil {
		return err
	}
	digest, length, err := PackageIdentity(ctx, s.packageFile)
	if err != nil || digest != s.identity.PackageSHA256 || length != s.identity.PackageLength {
		return errors.Join(ErrInspectionResult, err)
	}
	return ctx.Err()
}

// Called with the stage mutex held. systemd opens a fixed package path, so a
// still-readable retained descriptor is insufficient if that path was replaced.
func (s *InspectionStage) verifyServicePackagePath(parent *os.Root) (resultErr error) {
	file, err := openInspectionFile(parent, "inspection/package.deb", 0400)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	liveInfo, liveErr := file.Stat()
	pinnedInfo, pinnedErr := s.packageFile.Stat()
	if liveErr != nil || pinnedErr != nil || !os.SameFile(liveInfo, pinnedInfo) {
		return errors.Join(ErrInspectionResult, liveErr, pinnedErr)
	}
	return nil
}
