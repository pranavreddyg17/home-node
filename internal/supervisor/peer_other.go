//go:build !linux

package supervisor

import (
	"errors"
	"net"
)

func PeerUID(*net.UnixConn) (uint32, error) {
	return 0, errors.New("production peer authentication requires Linux")
}
