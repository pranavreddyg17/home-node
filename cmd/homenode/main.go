package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/control"
	"github.com/pranavreddyg17/home-node/internal/hostcheck"
	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/runtimeclient"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "doctor":
		doctor(os.Args[2:])
	case "serve":
		serve(os.Args[2:])
	case "setup-code":
		setupCode(os.Args[2:])
	default:
		usage()
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, "usage: homenode <doctor|serve|setup-code> [options]")
	os.Exit(2)
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
func doctor(args []string) {
	flags := flag.NewFlagSet("doctor", flag.ExitOnError)
	root := flags.String("data-root", "/var/lib/homenode", "workload data location")
	_ = flags.Parse(args)
	report := hostcheck.Inspect(*root)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fatal(err)
	}
	if !report.PrerequisitesMet {
		os.Exit(1)
	}
}
func setupCode(args []string) {
	flags := flag.NewFlagSet("setup-code", flag.ExitOnError)
	directory := flags.String("state-dir", "/var/lib/homenode/control", "private management state directory")
	_ = flags.Parse(args)
	store, err := state.Open(*directory)
	if err != nil {
		fatal(err)
	}
	defer store.Close()
	auth, err := identity.New(store, "http://localhost")
	if err != nil {
		fatal(err)
	}
	code, err := auth.CreateSetupCode(context.Background())
	if err != nil {
		fatal(err)
	}
	fmt.Println("Single-use enrollment code (expires in 10 minutes):")
	fmt.Println(code)
}
func serve(args []string) {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	supervisorSocket := flags.String("supervisor-socket", "", "protected runtime socket (empty disables execution)")
	transferSocket := flags.String("transfer-socket", "", "protected transfer socket")
	generation := flags.Int64("policy-generation", 1, "approved runtime policy generation")
	dev := flags.Bool("dev", false, "local development only; use HTTP on localhost")
	port := flags.Int("port", 8787, "listen port")
	address := flags.String("bind", "", "production Tailscale IP; development always uses 127.0.0.1")
	origin := flags.String("origin", "", "exact HTTPS browser origin (required in production)")
	directory := flags.String("state-dir", "/var/lib/homenode/control", "private management state directory")
	dataRoot := flags.String("data-root", "/var/lib/homenode", "workload data location")
	ui := flags.String("web-dir", "web/dist", "built first-party web assets")
	cert := flags.String("tls-cert", "", "certificate PEM path")
	key := flags.String("tls-key", "", "private key PEM path")
	_ = flags.Parse(args)
	if *port < 1 || *port > 65535 {
		fatal(fmt.Errorf("port must be 1..65535"))
	}
	if *dev {
		if *address != "" && *address != "127.0.0.1" {
			fatal(fmt.Errorf("development binds only loopback"))
		}
		*address = "127.0.0.1"
		if *origin == "" {
			*origin = "http://localhost:" + strconv.Itoa(*port)
		}
	} else {
		ip, err := netip.ParseAddr(*address)
		if err != nil || !netip.MustParsePrefix("100.64.0.0/10").Contains(ip) {
			fatal(fmt.Errorf("production bind must be the host's Tailscale IPv4 address"))
		}
		if *origin == "" || *cert == "" || *key == "" {
			fatal(fmt.Errorf("production requires --origin, --tls-cert and --tls-key"))
		}
	}
	store, err := state.Open(*directory)
	if err != nil {
		fatal(err)
	}
	defer store.Close()
	var backend workload.Backend
	if *supervisorSocket != "" || *transferSocket != "" {
		if *supervisorSocket == "" || *transferSocket == "" {
			fatal(fmt.Errorf("both workload sockets are required"))
		}
		if *dev {
			fatal(fmt.Errorf("development mode cannot connect to privileged workload services"))
		}
		backend = runtimeclient.New(*supervisorSocket, *transferSocket)
	}
	handler, err := control.New(store, control.Config{Runtime: backend, PolicyGeneration: *generation, Origin: *origin, Development: *dev, UI: os.DirFS(*ui), Report: func() hostcheck.Report { return hostcheck.Inspect(*dataRoot) }})
	if err != nil {
		fatal(err)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(*address, strconv.Itoa(*port)))
	if err != nil {
		fatal(err)
	}
	defer listener.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = handler.Workloads.Reconcile(ctx); err != nil {
		fatal(err)
	}
	go handler.Workloads.Run(ctx)
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13}}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := handler.Identity.Cleanup(ctx); err != nil && ctx.Err() == nil {
					slog.Error("identity cleanup failed")
				}
			}
		}
	}()
	slog.Info("HomeNode listening", "origin", *origin, "development", *dev)
	if *dev {
		err = server.Serve(listener)
	} else {
		err = server.ServeTLS(listener, *cert, *key)
	}
	if err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}
