//go:build !linux

package disktransport

import "os"

func AdmitDisk(*os.File, Metadata) error { return ErrPacket }
