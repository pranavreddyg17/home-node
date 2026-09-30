package backup

import "errors"

var ErrStagingCapacity = errors.New("backup staging would consume the host disk reserve")

const StagingReserveBytes int64 = 4 << 30

func stagingCapacity(availableBlocks uint64, blockBytes, remaining int64) error {
	if blockBytes <= 0 || remaining < 0 || remaining > 512<<30 {
		return ErrStagingCapacity
	}
	// Compare block counts, avoiding overflow in availableBlocks*blockBytes.
	needed := uint64(remaining + StagingReserveBytes)
	block := uint64(blockBytes)
	blocks := needed / block
	if needed%block != 0 {
		blocks++
	}
	if availableBlocks < blocks {
		return ErrStagingCapacity
	}
	return nil
}
