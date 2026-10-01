//go:build linux

package main

import (
	"bytes"
	"context"
	"testing"
)

func TestWorkerRefusesMissingOrUnexpectedArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--package", "/tmp/package.deb"}, {"--release", "0.1.0", "extra"}, {"--release", ""}} {
		var output bytes.Buffer
		if err := run(context.Background(), args, &output); err == nil || output.Len() != 0 {
			t.Fatal("invalid worker invocation accepted", args, err)
		}
	}
}
