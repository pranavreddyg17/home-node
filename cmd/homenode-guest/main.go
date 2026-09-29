package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guest"
)

func main() {
	kind := flag.String("workload", "", "files, video or ai")
	data := flag.String("data-dir", "/data/objects", "guest data directory")
	quota := flag.Int64("quota-bytes", 16<<30, "guest volume budget")
	channel := flag.String("channel", "/dev/virtio-ports/org.homenode.adapter", "virtio serial device")
	flag.Parse()
	agent, err := guest.New(*data, *kind, *quota)
	if err != nil {
		fmt.Fprintln(os.Stderr, "guest initialization failed:", err)
		os.Exit(1)
	}
	defer agent.Close()
	for {
		stream, err := os.OpenFile(*channel, os.O_RDWR, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, "guest channel unavailable")
			time.Sleep(time.Second)
			continue
		}
		_ = agent.Serve(stream)
		_ = stream.Close()
		time.Sleep(time.Second)
	}
}
