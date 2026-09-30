package backup

import (
	"errors"
	"math"
	"testing"
)

func TestStagingCapacityKeepsReserveWithRoundedBlockAccounting(t *testing.T) {
	const block int64 = 4096
	const disk int64 = 16 << 20
	needed := uint64((disk + StagingReserveBytes) / block)
	for _, tc := range []struct {
		name             string
		available        uint64
		block, remaining int64
		allowed          bool
	}{
		{"exact", needed, block, disk, true},
		{"short", needed - 1, block, disk, false},
		{"round-up", needed, block, disk + 1, false},
		{"rounded-capacity", needed + 1, block, disk + 1, true},
		{"reserve-only", uint64(StagingReserveBytes / block), block, 0, true},
		{"reserve-short", uint64(StagingReserveBytes/block) - 1, block, 0, false},
		{"overflow-free", math.MaxUint64, block, 512 << 30, true},
		{"invalid-block", math.MaxUint64, 0, disk, false},
		{"negative-block", math.MaxUint64, -1, disk, false},
		{"invalid-remaining", math.MaxUint64, block, -1, false},
		{"oversized", math.MaxUint64, block, (512 << 30) + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := stagingCapacity(tc.available, tc.block, tc.remaining)
			if tc.allowed && err != nil || !tc.allowed && !errors.Is(err, ErrStagingCapacity) {
				t.Fatal(err)
			}
		})
	}
}
