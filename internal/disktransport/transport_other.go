//go:build !linux

package disktransport

import (
	"context"
	"net"
	"os"
)

func SendFile(context.Context, *net.UnixConn, []byte, *os.File) error { return ErrPacket }
func ReceiveFile(context.Context, *net.UnixConn) ([]byte, *os.File, error) {
	return nil, nil, ErrPacket
}
