//go:build linux

package updates

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
)

// InspectionLaunchEpoch binds a monotonic launch boundary to its kernel boot.
// Recovery must verify this boot before comparing retained manager timestamps.
// This value is not launch authorization or a durable intent record.
type InspectionLaunchEpoch struct {
	BootID          string `json:"bootId"`
	NotBeforeMicros uint64 `json:"notBeforeMicros"`
}

func CaptureInspectionLaunchEpoch(ctx context.Context) (InspectionLaunchEpoch, error) {
	var zero InspectionLaunchEpoch
	boot, err := inspectionBootID(ctx)
	if err != nil {
		return zero, err
	}
	boundary, err := InspectionLaunchBoundary(ctx)
	if err != nil {
		return zero, err
	}
	return InspectionLaunchEpoch{BootID: boot, NotBeforeMicros: boundary}, nil
}

func VerifyInspectionLaunchEpoch(ctx context.Context, epoch InspectionLaunchEpoch) error {
	if !validInspectionBootID(epoch.BootID) || epoch.NotBeforeMicros == 0 {
		return ErrInspectionResult
	}
	current, err := CaptureInspectionLaunchEpoch(ctx)
	if err != nil {
		return err
	}
	if current.BootID != epoch.BootID || current.NotBeforeMicros < epoch.NotBeforeMicros {
		return ErrInspectionResult
	}
	return ctx.Err()
}

func inspectionBootID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := os.Open("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 38))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr, ctx.Err()); err != nil {
		return "", err
	}
	boot := strings.TrimSuffix(string(data), "\n")
	if !validInspectionBootID(boot) {
		return "", ErrInspectionResult
	}
	return boot, nil
}

func validInspectionBootID(value string) bool {
	if len(value) != 36 {
		return false
	}
	nonzero := false
	for i := range len(value) {
		c := value[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
		nonzero = nonzero || c != '0'
	}
	return nonzero
}
