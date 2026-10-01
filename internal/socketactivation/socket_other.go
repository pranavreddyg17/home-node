//go:build !linux

package socketactivation

import "net"

func TakePrivateListener(name, path string, backupGID uint32) (net.Listener, error) {
	return nil, ErrListener
}

func TakePrivatePacketListener(name, path string, peerGID uint32) (net.Listener, error) {
	return nil, ErrListener
}
