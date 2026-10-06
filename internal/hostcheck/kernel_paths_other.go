//go:build !linux

package hostcheck

func probeKernelPaths() (bool, bool) { return false, false }

func probeDescriptorChmod() bool { return false }
