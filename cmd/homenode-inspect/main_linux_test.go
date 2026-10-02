//go:build linux

package main

import (
	"bytes"
	"context"
	"strings"
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

func TestWorkerArgumentBoundsBeforeHostAdmission(t *testing.T) {
	operation := "inspection-fixture-000001"
	if !validWorkerArguments(operation, "0.1.0~ci") {
		t.Fatal("valid launcher identity refused")
	}
	for _, pair := range [][2]string{{operation, ""}, {operation, "release"}, {operation, "0.1.0\n"}, {operation, strings.Repeat("1", 65)}, {strings.Repeat("a", 65), "0.1.0"}, {strings.Repeat("a", 1<<20), "0.1.0"}, {operation, strings.Repeat("1", 1<<20)}} {
		if validWorkerArguments(pair[0], pair[1]) {
			t.Fatal("invalid worker identity admitted")
		}
	}
}
