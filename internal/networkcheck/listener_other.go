//go:build !linux

package networkcheck

import (
	"context"
	"net"
)

func ListenTailnet(context.Context, Config) (net.Listener, error) {
	return nil, ErrIdentity
}

func CheckIngressInterface(Config) error { return ErrIdentity }
