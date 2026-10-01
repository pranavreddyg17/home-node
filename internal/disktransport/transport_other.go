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

func SendPacket(context.Context, *net.UnixConn, []byte) error      { return ErrPacket }
func ReceivePacket(context.Context, *net.UnixConn) ([]byte, error) { return nil, ErrPacket }
