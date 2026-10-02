//go:build linux

package updates

import (
	"context"
	"math"

	"golang.org/x/sys/unix"
)

// InspectionLaunchBoundary reads the kernel monotonic clock in the units used
// by systemd process timestamps. Capture it immediately before requesting start
// and retain it for this boot's operation; wall-clock time is not interchangeable.
func InspectionLaunchBoundary(ctx context.Context) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var clock unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &clock); err != nil {
		return 0, err
	}
	if clock.Sec < 0 || clock.Nsec < 0 || clock.Nsec >= 1_000_000_000 || uint64(clock.Sec) > (math.MaxUint64-999_999)/1_000_000 {
		return 0, ErrInspectionResult
	}
	boundary := uint64(clock.Sec)*1_000_000 + uint64(clock.Nsec)/1000
	if boundary == 0 {
		return 0, ErrInspectionResult
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return boundary, nil
}
