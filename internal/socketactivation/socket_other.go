//go:build !linux

package socketactivation

import "net"

func TakePrivateListener(name, path string, backupGID uint32) (net.Listener, error) {
	return nil, ErrListener
}
