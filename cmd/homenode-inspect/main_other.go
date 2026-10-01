//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() { fmt.Fprintln(os.Stderr, "package inspection worker requires Linux"); os.Exit(1) }
