//go:build !linux

package hostcheck

func probeKVMDevice() bool { return false }
