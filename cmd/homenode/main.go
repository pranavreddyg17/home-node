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
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/control"
	"github.com/pranavreddyg17/home-node/internal/hostcheck"
	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/install"
	"github.com/pranavreddyg17/home-node/internal/networkcheck"
	"github.com/pranavreddyg17/home-node/internal/runtimeclient"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/workload"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "install-check":
		installCheck(os.Args[2:])
	case "install-prepare":
		installPrepare(os.Args[2:])
	case "doctor":
		doctor(os.Args[2:])
	case "serve":
		serve(os.Args[2:])
	case "accounts-provision":
		accountsProvision(os.Args[2:])
	case "accounts-check":
		accountsCheck(os.Args[2:])
	case "network-check":
		networkCheck(os.Args[2:])
	case "setup-code":
		setupCode(os.Args[2:])
	default:
		usage()
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, "usage: homenode <install-check|install-prepare|doctor|accounts-provision|accounts-check|network-check|serve|setup-code> [options]")
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
func accountsProvision(args []string) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fatal(fmt.Errorf("account provisioning requires Linux and root"))
	}
	flags := flag.NewFlagSet("accounts-provision", flag.ExitOnError)
	directory := flags.String("journal-dir", "/var/lib/homenode-install", "private journal directory; the default is created securely")
	_ = flags.Parse(args)
	var engine *install.Engine
	var err error
	if *directory == "/var/lib/homenode-install" {
		engine, err = install.OpenSystem()
	} else {
		engine, err = install.Open("/", *directory)
	}
	if err != nil {
		fatal(err)
	}
	defer engine.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	accounts, err := engine.ProvisionAccounts(ctx)
	if err != nil {
		fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"accountsProvisioned": true, "accounts": accounts, "servicesActivated": false}); err != nil {
		fatal(err)
	}
}
func accountsCheck(args []string) {
	flags := flag.NewFlagSet("accounts-check", flag.ExitOnError)
	_ = flags.Parse(args)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	accounts, err := install.InspectLocalAccounts(ctx)
	if err != nil {
		fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"accountsValid": true, "accounts": accounts, "servicesActivated": false}); err != nil {
		fatal(err)
	}
}
func networkCheck(args []string) {
	flags := flag.NewFlagSet("network-check", flag.ExitOnError)
	bind := flags.String("bind", "", "host Tailscale IPv4 address")
	port := flags.Int("port", 8787, "unprivileged HTTPS port")
	origin := flags.String("origin", "", "exact HTTPS origin")
	cert := flags.String("tls-cert", "/etc/homenode/tls/server.crt", "certificate PEM path")
	key := flags.String("tls-key", "/etc/homenode/tls/server.key", "protected private key PEM path")
	_ = flags.Parse(args)
	certificate, err := networkcheck.Inspect(networkcheck.Config{Bind: *bind, Port: *port, Origin: *origin}, *cert, *key)
	if err != nil {
		fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"identityValid": true, "certificateExpires": certificate.Leaf.NotAfter, "tailnetPolicyVerified": false, "phoneReachabilityVerified": false}); err != nil {
		fatal(err)
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
	var certificateSource *networkcheck.CertificateSource
	if *dev {
		if *address != "" && *address != "127.0.0.1" {
			fatal(fmt.Errorf("development binds only loopback"))
		}
		*address = "127.0.0.1"
		if *origin == "" {
			*origin = "http://localhost:" + strconv.Itoa(*port)
		}
	} else {
		var err error
		certificateSource, err = networkcheck.NewCertificateSource(networkcheck.Config{Bind: *address, Port: *port, Origin: *origin}, *cert, *key)
		if err != nil {
			fatal(err)
		}
		certificate, err := certificateSource.GetCertificate(nil)
		if err != nil {
			fatal(err)
		}
		if time.Until(certificate.Leaf.NotAfter) < 14*24*time.Hour {
			slog.Warn("TLS certificate needs renewal before expiry")
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
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, SessionTicketsDisabled: true}}
	if certificateSource != nil {
		server.TLSConfig.GetCertificate = certificateSource.GetCertificate
	}
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
		err = server.ServeTLS(listener, "", "")
	}
	if err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}
