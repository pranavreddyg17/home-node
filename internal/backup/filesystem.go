package backup

import (
	"encoding/binary"
	"io"
)

// requireCleanExt4Header is a preliminary refusal check, not a filesystem
// integrity certificate. A full read-only checker and exclusive runtime lease
// are also required before copying a guest disk.
func requireCleanExt4Header(source io.ReaderAt) error {
	if source == nil {
		return ErrManifest
	}
	var block [1024]byte
	if _, err := source.ReadAt(block[:], 1024); err != nil {
		return ErrManifest
	}
	// Linux ext4 superblock: magic, state, journal recovery incompatibility.
	if binary.LittleEndian.Uint16(block[0x38:]) != 0xef53 || binary.LittleEndian.Uint16(block[0x3a:]) != 1 || binary.LittleEndian.Uint32(block[0x60:])&4 != 0 {
		return ErrManifest
	}
	return nil
}
