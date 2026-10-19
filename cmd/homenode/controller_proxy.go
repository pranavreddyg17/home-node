package main

import (
	"errors"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"github.com/pranavreddyg17/home-node/internal/gatewaytransport"
)

func validateControllerProxy(platform, socket string, dev bool, uid, gid int, gatewayUID, bridgeGID uint32, cert, key string) error {
	if platform != "linux" || dev || uid <= 0 || gid <= 0 || gatewayUID == 0 || gatewayUID > 1<<31-1 || bridgeGID == 0 || bridgeGID > 1<<31-1 || uint32(uid) == gatewayUID || uint32(gid) == bridgeGID || cert != "" || key != "" || !filepath.IsAbs(socket) || filepath.Clean(socket) != socket {
		return gatewaytransport.ErrTransport
	}
	return nil
}

func controllerProxyIdentity(socket string, dev bool, gatewayUID, bridgeGID uint32, cert, key string) error {
	if err := validateControllerProxy(runtime.GOOS, socket, dev, os.Geteuid(), os.Getegid(), gatewayUID, bridgeGID, cert, key); err != nil {
		return err
	}
	var gatewayPrimaryGroup string
	for name, expected := range map[string]string{"homenode": strconv.Itoa(os.Geteuid()), "homenode-gateway": strconv.FormatUint(uint64(gatewayUID), 10)} {
		account, err := user.Lookup(name)
		if err != nil || account.Uid != expected {
			return gatewaytransport.ErrTransport
		}
		if name == "homenode" && account.Gid != strconv.Itoa(os.Getegid()) {
			return gatewaytransport.ErrTransport
		}
		if name == "homenode-gateway" {
			gatewayPrimaryGroup = account.Gid
		}
	}
	allowed := map[int]bool{}
	roleGroups := map[string]uint32{}
	for _, name := range []string{"homenode", "homenode-gateway", "homenode-proxy", "homenode-runtime"} {
		group, err := user.LookupGroup(name)
		if err != nil {
			return gatewaytransport.ErrTransport
		}
		id, err := strconv.Atoi(group.Gid)
		if err != nil || id <= 0 || id > 1<<31-1 {
			return gatewaytransport.ErrTransport
		}
		roleGroups[name] = uint32(id)
		if name == "homenode-gateway" {
			if group.Gid != gatewayPrimaryGroup {
				return gatewaytransport.ErrTransport
			}
			continue // private key group must never become an allowed controller group
		}
		if name == "homenode" && id != os.Getegid() {
			return gatewaytransport.ErrTransport
		}
		if name == "homenode-proxy" && uint32(id) != bridgeGID {
			return gatewaytransport.ErrTransport
		}
		allowed[id] = true
	}
	if err := gatewaytransport.ValidateRoleGroups(roleGroups["homenode"], roleGroups["homenode-gateway"], roleGroups["homenode-proxy"], roleGroups["homenode-runtime"]); err != nil {
		return err
	}
	groups, err := os.Getgroups()
	if err != nil {
		return err
	}
	bridge := false
	for _, group := range groups {
		if !allowed[group] {
			return gatewaytransport.ErrTransport
		}
		if uint32(group) == bridgeGID {
			bridge = true
		}
	}
	if !bridge {
		return gatewaytransport.ErrTransport
	}
	return nil
}

// Refuse existing endpoints rather than unlinking a possibly live controller.
// The installed service must supply a fresh, private RuntimeDirectory.
func listenControllerProxy(socket string, bridgeGID uint32) (net.Listener, error) {
	parent, err := os.Lstat(filepath.Dir(socket))
	if err != nil || !parent.IsDir() || parent.Mode().Perm()&0022 != 0 || parent.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, gatewaytransport.ErrTransport
	}
	owner, ok := parent.Sys().(*syscall.Stat_t)
	if !ok || (owner.Uid != 0 && owner.Uid != uint32(os.Geteuid())) {
		return nil, gatewaytransport.ErrTransport
	}
	if _, err := os.Lstat(socket); !errors.Is(err, os.ErrNotExist) {
		return nil, gatewaytransport.ErrTransport
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(socket, 0600); err == nil {
		err = os.Chown(socket, os.Geteuid(), int(bridgeGID))
	}
	if err == nil {
		err = os.Chmod(socket, 0660)
	}
	if err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}
