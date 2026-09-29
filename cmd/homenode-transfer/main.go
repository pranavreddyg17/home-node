package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/runtimeclient"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
	"github.com/pranavreddyg17/home-node/internal/transfer"
)

func main() {
	socket := flag.String("socket", "/run/homenode/transfer.sock", "transfer service socket")
	runtimeSocket := flag.String("supervisor", "/run/homenode/supervisor.sock", "runtime service socket")
	channels := flag.String("channels", "/run/homenode/guests", "guest channels")
	uid := flag.Uint("controller-uid", 0, "authorized controller UID")
	gid := flag.Int("access-gid", -1, "controller-access group")
	generation := flag.Int64("policy-generation", 0, "runtime policy generation")
	flag.Parse()
	if runtime.GOOS != "linux" || os.Geteuid() == 0 || *uid == 0 || *gid < 1 || *generation < 1 {
		fmt.Fprintln(os.Stderr, "requires Linux, an unprivileged transfer identity, and explicit controller policy")
		os.Exit(1)
	}
	listener, err := net.Listen("unix", *socket)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer listener.Close()
	if err = os.Chown(*socket, -1, *gid); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = os.Chmod(*socket, 0660); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	handler := transfer.New(runtimeclient.New(*runtimeSocket, ""), *channels, uint32(*uid), *generation)
	server := &http.Server{Handler: handler, ConnContext: supervisor.PeerContext, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 25 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(deadline)
	}()
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
