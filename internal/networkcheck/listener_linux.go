//go:build linux

package networkcheck

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// ListenTailnet opens only the supported kernel-TUN ingress path. The daemon
// identity and certificate must be admitted separately before calling this.
// No wildcard/interface override or firewall-dependent fallback is supported.
func ListenTailnet(ctx context.Context, c Config) (net.Listener, error) {
	if _, err := ValidateConfiguration(c); err != nil {
		return nil, err
	}
	device, err := tailnetInterface(c)
	if err != nil {
		return nil, err
	}
	listener, err := listenOnDevice(ctx, net.JoinHostPort(c.Bind, strconv.Itoa(c.Port)), device)
	if err != nil {
		return nil, err
	}
	return watchListener(listener, time.Second, func() bool {
		current, err := tailnetInterface(c)
		return err == nil && current.Index == device.Index
	}), nil
}

func tailnetInterface(c Config) (*net.Interface, error) {
	device, err := net.InterfaceByName("tailscale0")
	if err != nil || device.Flags&net.FlagUp == 0 || device.Flags&net.FlagLoopback != 0 || device.Flags&net.FlagPointToPoint == 0 {
		return nil, fmt.Errorf("private ingress requires the active tailscale0 TUN interface: %w", ErrIdentity)
	}
	addresses, err := device.Addrs()
	if err != nil {
		return nil, fmt.Errorf("cannot inspect private ingress addresses: %w", ErrIdentity)
	}
	want, _ := netip.ParseAddr(c.Bind)
	found := false
	for _, address := range addresses {
		prefix, parseErr := netip.ParsePrefix(address.String())
		if parseErr == nil && prefix.Addr() == want {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("private bind address is not assigned to tailscale0: %w", ErrIdentity)
	}
	return device, nil
}

func listenOnDevice(ctx context.Context, address string, device *net.Interface) (net.Listener, error) {
	if device == nil || device.Index <= 0 || device.Name == "" {
		return nil, ErrIdentity
	}
	config := net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
		var socketErr error
		err := raw.Control(func(fd uintptr) {
			// Linux allows an unprivileged initial binding. Failure (including
			// unavailable kernel support) must prevent bind/listen entirely.
			socketErr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, device.Name)
			if socketErr != nil {
				return
			}
			index, readErr := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BINDTOIFINDEX)
			if readErr != nil {
				socketErr = readErr
			} else if index != device.Index {
				socketErr = ErrIdentity
			}
		})
		if err != nil {
			return err
		}
		return socketErr
	}}
	listener, err := config.Listen(ctx, "tcp4", address)
	if err != nil {
		return nil, fmt.Errorf("cannot enforce private ingress interface binding: %w", err)
	}
	return listener, nil
}
