//go:build linux

package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// PublishLaunchIntent durably records an admitted package and boot-bound
// prelaunch boundary before any manager start request. It grants no installation
// authority and refuses existing or interrupted launch records. The caller must
// have independently verified effective policy and dormant manager state.
func (s *InspectionStage) PublishLaunchIntent(ctx context.Context, parent *os.Root, epoch InspectionLaunchEpoch) error {
	if os.Geteuid() != 0 {
		return ErrInspectionResult
	}
	return s.publishLaunchIntentOwned(ctx, parent, epoch)
}

func (s *InspectionStage) publishLaunchIntentOwned(ctx context.Context, parent *os.Root, epoch InspectionLaunchEpoch) (resultErr error) {
	if s == nil || parent == nil {
		return ErrInspectionResult
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.verifyEnvironmentLocked(ctx, parent); err != nil {
		return err
	}
	if err := VerifyInspectionLaunchEpoch(ctx, epoch); err != nil {
		return err
	}
	data, err := inspectionLaunchRecord(s.identity, epoch)
	if err != nil {
		return err
	}
	if _, err := parent.Lstat("inspection.launch"); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrInspectionResult, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeInitializationFile(parent, "inspection.launch.pending", data); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	directory, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	if err := unix.Renameat2(int(directory.Fd()), "inspection.launch.pending", int(directory.Fd()), "inspection.launch", unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	return errors.Join(directory.Sync(), ctx.Err())
}

func inspectionLaunchRecord(identity InspectionIdentity, epoch InspectionLaunchEpoch) ([]byte, error) {
	if !validInspectionIdentity(identity) || !validInspectionBootID(epoch.BootID) || epoch.NotBeforeMicros == 0 {
		return nil, ErrInspectionResult
	}
	record := struct {
		Schema          int    `json:"schema"`
		OperationID     string `json:"operationId"`
		Release         string `json:"release"`
		PackageSHA256   string `json:"packageSha256"`
		PackageLength   int64  `json:"packageLength"`
		BootID          string `json:"bootId"`
		NotBeforeMicros uint64 `json:"notBeforeMicros"`
	}{1, identity.OperationID, identity.Release, identity.PackageSHA256, identity.PackageLength, epoch.BootID, epoch.NotBeforeMicros}
	return json.Marshal(record)
}

// VerifyLaunchIntent verifies private canonical publication against the
// independently retained epoch and locked package. It does not recover an
// unknown epoch from disk or authenticate execution of a manager invocation.
func (s *InspectionStage) VerifyLaunchIntent(ctx context.Context, parent *os.Root, epoch InspectionLaunchEpoch) error {
	if os.Geteuid() != 0 {
		return ErrInspectionResult
	}
	return s.verifyLaunchIntentOwned(ctx, parent, epoch)
}

func (s *InspectionStage) verifyLaunchIntentOwned(ctx context.Context, parent *os.Root, epoch InspectionLaunchEpoch) error {
	if s == nil || parent == nil {
		return ErrInspectionResult
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.verifyLaunchIntentLocked(ctx, parent, epoch)
}

func (s *InspectionStage) verifyLaunchIntentLocked(ctx context.Context, parent *os.Root, epoch InspectionLaunchEpoch) error {
	if err := s.verifyEnvironmentLocked(ctx, parent); err != nil {
		return err
	}
	if err := VerifyInspectionLaunchEpoch(ctx, epoch); err != nil {
		return err
	}
	if _, err := parent.Lstat("inspection.launch.pending"); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrInspectionResult, err)
	}
	file, err := openInspectionFile(parent, "inspection.launch", 0600)
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 2049))
	closeErr := file.Close()
	expected, err := inspectionLaunchRecord(s.identity, epoch)
	if err != nil || readErr != nil || closeErr != nil || !bytes.Equal(data, expected) {
		return errors.Join(ErrInspectionResult, err, readErr, closeErr)
	}
	return ctx.Err()
}
