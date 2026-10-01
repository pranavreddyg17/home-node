//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func verifyInspectionServiceLimits(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		t.Fatal(err)
	}
	var root string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "0::/") {
			name := strings.TrimPrefix(line, "0::")
			if filepath.Clean(name) != name {
				t.Fatal("noncanonical cgroup membership")
			}
			root = filepath.Join("/sys/fs/cgroup", strings.TrimPrefix(name, "/"))
		}
	}
	if root == "" {
		t.Fatal("missing unified cgroup membership")
	}
	for name, expected := range map[string]string{"memory.max": "268435456", "memory.swap.max": "0", "pids.max": "32", "memory.oom.group": "1"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || strings.TrimSpace(string(data)) != expected {
			t.Fatalf("unexpected %s: %q %v", name, data, err)
		}
	}
	data, err = os.ReadFile(filepath.Join(root, "cpu.max"))
	fields := strings.Fields(string(data))
	if err != nil || len(fields) != 2 {
		t.Fatal("missing CPU limit", err)
	}
	quota, quotaErr := strconv.ParseUint(fields[0], 10, 64)
	period, periodErr := strconv.ParseUint(fields[1], 10, 64)
	if quotaErr != nil || periodErr != nil || quota == 0 || period == 0 || quota != period/2 {
		t.Fatal("unexpected CPU quota", string(data))
	}
}
