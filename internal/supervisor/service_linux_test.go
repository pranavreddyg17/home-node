//go:build linux

package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNativeSupervisorServiceIsolation(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_SUPERVISOR_SOURCE_FIXTURE") != "1" {
		t.Skip("opt-in disposable source-unit fixture")
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	observed := map[string]uint64{}
	for _, line := range strings.Split(string(status), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "CapEff:", "CapPrm:", "CapBnd:", "CapInh:", "CapAmb:":
			value, err := strconv.ParseUint(fields[1], 16, 64)
			if err != nil {
				t.Fatal(err)
			}
			observed[fields[0]] = value
		case "NoNewPrivs:":
			if fields[1] != "1" {
				t.Fatal("new privileges not disabled")
			}
			observed[fields[0]] = 1
		}
	}
	for _, key := range []string{"CapEff:", "CapPrm:", "CapBnd:"} {
		if observed[key] != 0x8000b {
			t.Fatal("unexpected supervisor capabilities")
		}
	}
	for _, key := range []string{"CapInh:", "CapAmb:"} {
		if value, ok := observed[key]; !ok || value != 0 {
			t.Fatal("unexpected inherited capabilities")
		}
	}
	if observed["NoNewPrivs:"] != 1 {
		t.Fatal("missing privilege restriction")
	}
	for _, path := range []string{"/var/lib/homenode/control/fixture-secret", "/etc/homenode/tls/fixture-secret"} {
		if _, err := os.ReadFile(path); err == nil {
			t.Fatal("excluded fixture data exposed")
		}
	}
	hidden := os.Getenv("HOMENODE_SUPERVISOR_HIDDEN_PATH")
	if hidden == "" {
		t.Fatal("missing private tmp evidence")
	}
	if _, err := os.Stat(hidden); !os.IsNotExist(err) {
		t.Fatal("host tmp fixture exposed")
	}
	probe := fmt.Sprintf("/usr/lib/homenode-fixtures/supervisor-readonly-probe-%d", os.Getpid())
	file, err := os.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		file.Close()
		os.Remove(probe)
		t.Fatal("system path writable")
	}
	if !errors.Is(err, unix.EROFS) {
		t.Fatal("missing readonly namespace evidence")
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	unix.Close(fd)
	for _, family := range []int{unix.AF_INET, unix.AF_INET6} {
		fd, err := unix.Socket(family, unix.SOCK_STREAM, 0)
		if err == nil {
			unix.Close(fd)
			t.Fatal("supervisor network family allowed")
		}
		if !errors.Is(err, unix.EAFNOSUPPORT) && !errors.Is(err, unix.EPERM) {
			t.Fatal("unexpected socket refusal", err)
		}
	}
	cgroups, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		t.Fatal(err)
	}
	group := ""
	for _, line := range strings.Split(string(cgroups), "\n") {
		if strings.HasPrefix(line, "0::") {
			if group != "" {
				t.Fatal("ambiguous cgroup")
			}
			group = strings.TrimPrefix(line, "0::")
		}
	}
	if !strings.HasPrefix(group, "/system.slice/homenode-supervisor-fixture-") || filepath.Clean(group) != group {
		t.Fatal("missing source-unit cgroup")
	}
	for name, expected := range map[string]string{"memory.max": "536870912", "pids.max": "64"} {
		value, err := os.ReadFile(filepath.Join("/sys/fs/cgroup", group, name))
		if err != nil || strings.TrimSpace(string(value)) != expected {
			t.Fatal("source-unit resource bound mismatch", name, err)
		}
	}
}
