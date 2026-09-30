package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/pranavreddyg17/home-node/internal/guest"
	"github.com/pranavreddyg17/home-node/internal/guestmount"
)

func main() {
	kind := flag.String("workload", "", "files, video or ai")
	data := flag.String("data-dir", "/data/objects", "guest data directory")
	quota := flag.Int64("quota-bytes", 16<<30, "guest volume budget")
	channel := flag.String("channel", "/dev/virtio-ports/org.homenode.adapter", "virtio serial device")
	flag.Parse()
	if *data != "/data/objects" || guestmount.Check() != nil {
		fmt.Fprintln(os.Stderr, "guest data mount admission failed")
		os.Exit(1)
	}
	agent, err := guest.New(*data, *kind, *quota)
	if err != nil {
		fmt.Fprintln(os.Stderr, "guest initialization failed:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	_ = runGuest(ctx, agent, func() (io.ReadWriteCloser, error) { return os.OpenFile(*channel, os.O_RDWR, 0) })
	if err = agent.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "guest shutdown failed:", err)
		os.Exit(1)
	}
}
