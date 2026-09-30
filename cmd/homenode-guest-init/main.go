// Guest-only startup helper. Never run on the owner's host.
package main

import (
	"fmt"
	"github.com/pranavreddyg17/home-node/internal/guestinit"
	"github.com/pranavreddyg17/home-node/internal/guestmount"
	"os"
)

func main() {
	if len(os.Args) != 1 || os.Geteuid() != 0 || guestmount.Check() != nil || guestinit.Prepare("/data") != nil {
		fmt.Fprintln(os.Stderr, "guest object directory initialization failed")
		os.Exit(1)
	}
}
