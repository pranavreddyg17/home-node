//go:build !linux

package networkcheck

import (
	"os"
	"syscall"
)

// Production host installation is Linux-only. Other platforms retain the
// portable identity checks for development and certificate validation tests.
func protectedPEMACL(file *os.File) error { return nil }

func tlsChangeTimeUnchanged(before, after *syscall.Stat_t) bool { return true }
