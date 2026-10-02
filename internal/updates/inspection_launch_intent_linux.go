//go:build linux

package updates

import (
	"context"
	"encoding/json"
	"errors"
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
	record := struct {
		Schema          int    `json:"schema"`
		OperationID     string `json:"operationId"`
		Release         string `json:"release"`
		PackageSHA256   string `json:"packageSha256"`
		PackageLength   int64  `json:"packageLength"`
		BootID          string `json:"bootId"`
		NotBeforeMicros uint64 `json:"notBeforeMicros"`
	}{1, s.identity.OperationID, s.identity.Release, s.identity.PackageSHA256, s.identity.PackageLength, epoch.BootID, epoch.NotBeforeMicros}
	data, err := json.Marshal(record)
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
