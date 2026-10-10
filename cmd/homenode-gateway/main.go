package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/connectionbudget"
	"github.com/pranavreddyg17/home-node/internal/gatewaytransport"
	"github.com/pranavreddyg17/home-node/internal/networkcheck"
)

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func validateIdentity(platform string, uid, gid int, controllerUID, bridgeGID uint32, groups []int) error {
	if platform != "linux" || uid <= 0 || gid <= 0 || uid > 1<<31-1 || gid > 1<<31-1 || controllerUID == 0 || controllerUID > 1<<31-1 || bridgeGID == 0 || bridgeGID > 1<<31-1 || uint32(uid) == controllerUID || uint32(gid) == bridgeGID {
		return gatewaytransport.ErrTransport
	}
	for _, group := range groups {
		if group < 0 || group > 1<<31-1 || (group != gid && uint32(group) != bridgeGID) {
			return gatewaytransport.ErrTransport
		}
	}
	return nil
}

func installedIdentity(controllerUID, bridgeGID uint32) error {
	groups, err := os.Getgroups()
	if err != nil || validateIdentity(runtime.GOOS, os.Geteuid(), os.Getegid(), controllerUID, bridgeGID, groups) != nil {
		return gatewaytransport.ErrTransport
	}
	gateway, err := user.Lookup("homenode-gateway")
	if err != nil || gateway.Uid != strconv.Itoa(os.Geteuid()) {
		return gatewaytransport.ErrTransport
	}
	controller, err := user.Lookup("homenode")
	if err != nil || controller.Uid != strconv.FormatUint(uint64(controllerUID), 10) {
		return gatewaytransport.ErrTransport
	}
	ownGroup, err := user.LookupGroup("homenode-gateway")
	if err != nil || ownGroup.Gid != strconv.Itoa(os.Getegid()) {
		return gatewaytransport.ErrTransport
	}
	bridge, err := user.LookupGroup("homenode-proxy")
	if err != nil || bridge.Gid != strconv.FormatUint(uint64(bridgeGID), 10) {
		return gatewaytransport.ErrTransport
	}
	return nil
}

func main() {
	bind := flag.String("bind", "", "approved private Tailscale IPv4 address")
	port := flag.Int("port", 8787, "private HTTPS port")
	origin := flag.String("origin", "", "exact approved HTTPS origin")
	socket := flag.String("controller-socket", "/run/homenode-control/control.sock", "fixed private controller endpoint")
	controllerUID := flag.Uint("controller-uid", 0, "installed controller UID")
	bridgeGID := flag.Uint("access-gid", 0, "installed proxy group")
	certificate := flag.String("tls-cert", "/etc/homenode/tls/server.crt", "protected server certificate")
	privateKey := flag.String("tls-key", "/etc/homenode/tls/server.key", "protected gateway-only key")
	flag.Parse()
	if *controllerUID > 1<<31-1 || *bridgeGID > 1<<31-1 || installedIdentity(uint32(*controllerUID), uint32(*bridgeGID)) != nil {
		fatal(errors.New("gateway requires its dedicated Linux identity and proxy-only group membership"))
	}
	source, err := networkcheck.NewCertificateSource(networkcheck.Config{Bind: *bind, Port: *port, Origin: *origin}, *certificate, *privateKey)
	if err != nil {
		fatal(err)
	}
	config := gatewaytransport.Config{Origin: *origin, Socket: *socket, ControllerUID: uint32(*controllerUID), AccessGID: uint32(*bridgeGID)}
	proxy, closeProxy, err := gatewaytransport.NewProxy(config)
	if err != nil {
		fatal(err)
	}
	defer closeProxy()
	probe, cancelProbe := context.WithTimeout(context.Background(), 3*time.Second)
	err = gatewaytransport.CheckController(probe, config)
	cancelProbe()
	if err != nil {
		fatal(errors.New("private controller endpoint is not ready or its identity differs"))
	}
	raw, err := net.Listen("tcp", net.JoinHostPort(*bind, strconv.Itoa(*port)))
	if err != nil {
		fatal(err)
	}
	listener, err := connectionbudget.New(raw, connectionbudget.DefaultTotal, connectionbudget.DefaultPerAddress)
	if err != nil {
		raw.Close()
		fatal(err)
	}
	defer listener.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Handler: proxy, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, SessionTicketsDisabled: true, GetCertificate: source.GetCertificate}}
	done := make(chan error, 1)
	go func() {
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := server.Shutdown(deadline)
		if err != nil {
			err = errors.Join(err, server.Close())
		}
		done <- err
	}()
	slog.Info("HomeNode private gateway listening", "origin", *origin)
	err = server.ServeTLS(listener, "", "")
	stop()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	if err = errors.Join(err, <-done); err != nil {
		fatal(err)
	}
}
