package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/pranavreddyg17/home-node/internal/hostcheck"
	"github.com/pranavreddyg17/home-node/internal/server"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		doctor(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		serve(os.Args[2:])
		return
	}
	fmt.Fprintln(os.Stderr, "usage: homenode <doctor|serve> [options]")
	os.Exit(2)
}

func doctor(args []string) {
	flags := flag.NewFlagSet("doctor", flag.ExitOnError)
	dataRoot := flags.String("data-root", "/var/lib/homenode", "future workload data location")
	_ = flags.Parse(args)
	report := hostcheck.Inspect(*dataRoot)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !report.PrerequisitesMet {
		os.Exit(1)
	}
}

func serve(args []string) {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	port := flags.Int("port", 8787, "local diagnostics port")
	dataRoot := flags.String("data-root", "/var/lib/homenode", "future workload data location")
	_ = flags.Parse(args)
	if *port < 0 || *port > 65535 {
		fmt.Fprintln(os.Stderr, "port must be between 0 and 65535")
		os.Exit(2)
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(*port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer listener.Close()
	srv := &http.Server{
		Handler:           server.New(func() hostcheck.Report { return hostcheck.Inspect(*dataRoot) }),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	slog.Info("local diagnostics available", "address", listener.Addr().String())
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
