package hostcheck

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const gib = uint64(1024 * 1024 * 1024)

type Status string

const (
	Pass Status = "pass"
	Warn Status = "warn"
	Fail Status = "fail"
)

type Check struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      Status `json:"status"`
	Detail      string `json:"detail"`
	Remediation string `json:"remediation,omitempty"`
}

type Host struct {
	OS                  string `json:"os"`
	Architecture        string `json:"architecture"`
	Distribution        string `json:"distribution,omitempty"`
	DistributionVersion string `json:"distributionVersion,omitempty"`
	MemoryBytes         uint64 `json:"memoryBytes,omitempty"`
	AvailableDiskBytes  uint64 `json:"availableDiskBytes,omitempty"`
	DiskProbePath       string `json:"diskProbePath,omitempty"`
}

type Report struct {
	GeneratedAt      time.Time `json:"generatedAt"`
	Host             Host      `json:"host"`
	Checks           []Check   `json:"checks"`
	PrerequisitesMet bool      `json:"prerequisitesMet"`
	ExecutionEnabled bool      `json:"executionEnabled"`
}

// Facts are observations, not an isolation certification. Keeping evaluation
// separate makes the supported-host policy testable without a privileged host.
type Facts struct {
	OS                  string
	Architecture        string
	Distribution        string
	DistributionVersion string
	SafePathResolution  bool
	MountIdentity       bool
	DescriptorChmod     bool
	KVMUsable           bool
	AppArmorEnforcing   bool
	LibvirtReachable    bool
	MemoryBytes         uint64
	AvailableDiskBytes  uint64
	DiskProbePath       string
}

func Inspect(dataRoot string) Report {
	facts := probe(dataRoot)
	return Evaluate(facts, time.Now().UTC())
}

func Evaluate(f Facts, now time.Time) Report {
	checks := make([]Check, 0, 7)
	add := func(id, title string, good bool, detail, remediation string) {
		status := Fail
		if good {
			status = Pass
			remediation = ""
		}
		checks = append(checks, Check{id, title, status, detail, remediation})
	}
	add("os", "Linux host", f.OS == "linux", f.OS, "Install the supported Linux release on a dedicated machine.")
	add("architecture", "x86-64 processor", f.Architecture == "amd64", f.Architecture, "Use a supported x86-64 host; other architectures need a separate validation.")
	add("distribution", "Supported Linux release", f.Distribution == "ubuntu" && f.DistributionVersion == "24.04", valueOrUnknown(f.Distribution)+" "+valueOrUnknown(f.DistributionVersion), "Use Ubuntu Server 24.04 LTS for the initial supported host matrix.")
	add("safe-path-resolution", "Kernel safe path resolution", f.SafePathResolution, boolDetail(f.SafePathResolution, "openat2 protection is available", "openat2 protection is unavailable"), "Use the supported kernel and permit the required path-resolution syscall.")
	add("mount-identity", "Kernel mount identity", f.MountIdentity, boolDetail(f.MountIdentity, "Descriptor mount identity is available", "Descriptor mount identity is unavailable"), "Use the supported kernel and permit statx mount identity queries.")
	add("descriptor-chmod", "Pinned socket permissions", f.DescriptorChmod, boolDetail(f.DescriptorChmod, "Descriptor permission changes are available", "Descriptor permission changes are unavailable"), "Use the supported kernel and permit fchmodat2 with AT_EMPTY_PATH.")
	add("kvm", "Hardware virtualization", f.KVMUsable, boolDetail(f.KVMUsable, "KVM is accessible", "KVM is unavailable or cannot be opened"), "Enable CPU virtualization in firmware and grant the service appropriate KVM access.")
	add("apparmor", "AppArmor", f.AppArmorEnforcing, boolDetail(f.AppArmorEnforcing, "AppArmor is enabled", "AppArmor is unavailable or disabled"), "Enable and verify AppArmor on the host.")
	add("libvirt", "Local VM service", f.LibvirtReachable, boolDetail(f.LibvirtReachable, "libvirt is reachable", "libvirt is unavailable or inaccessible"), "Install and start the supported libvirt service; check local socket permissions.")
	if f.MemoryBytes == 0 {
		checks = append(checks, Check{"memory", "System memory", Fail, "Memory capacity could not be measured", "Run the checker on a supported Linux host."})
	} else {
		add("memory", "System memory", f.MemoryBytes >= 8*gib, fmt.Sprintf("%.1f GiB detected", float64(f.MemoryBytes)/float64(gib)), "At least 8 GiB is required for the initial host profile; model capacity is checked separately.")
	}
	if f.AvailableDiskBytes == 0 {
		checks = append(checks, Check{"disk", "Available storage", Fail, "Free space could not be measured", "Check the future data filesystem and rerun the preflight."})
	} else {
		add("disk", "Available storage", f.AvailableDiskBytes >= 40*gib, fmt.Sprintf("%.1f GiB available at %s", float64(f.AvailableDiskBytes)/float64(gib), f.DiskProbePath), "Provide at least 40 GiB of free space on the future data filesystem. This is a provisional admission floor, not an AI model guarantee.")
	}
	ready := true
	for _, check := range checks {
		if check.Status == Fail {
			ready = false
		}
	}
	return Report{
		GeneratedAt: now,
		Host:        Host{OS: f.OS, Architecture: f.Architecture, Distribution: f.Distribution, DistributionVersion: f.DistributionVersion, MemoryBytes: f.MemoryBytes, AvailableDiskBytes: f.AvailableDiskBytes, DiskProbePath: f.DiskProbePath},
		Checks:      checks, PrerequisitesMet: ready, ExecutionEnabled: false,
	}
}

func probe(dataRoot string) Facts {
	f := Facts{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	if runtime.GOOS == "linux" {
		f.SafePathResolution, f.MountIdentity = probeKernelPaths()
		f.DescriptorChmod = probeDescriptorChmod()
		f.Distribution, f.DistributionVersion = readOSRelease("/etc/os-release")
		f.KVMUsable = probeKVMDevice()
		if b, err := os.ReadFile("/sys/module/apparmor/parameters/enabled"); err == nil {
			f.AppArmorEnforcing = strings.TrimSpace(string(b)) == "Y"
		}
		for _, path := range []string{"/run/libvirt/virtqemud-sock", "/run/libvirt/libvirt-sock"} {
			conn, err := net.DialTimeout("unix", path, 300*time.Millisecond)
			if err == nil {
				f.LibvirtReachable = true
				_ = conn.Close()
				break
			}
		}
		f.MemoryBytes = readMemory("/proc/meminfo")
	}
	f.DiskProbePath, f.AvailableDiskBytes = availableAt(dataRoot)
	return f
}

func readOSRelease(path string) (string, string) {
	file, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok {
			values[key] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return values["ID"], values["VERSION_ID"]
}

func readMemory(path string) uint64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if value, ok := strings.CutPrefix(scanner.Text(), "MemTotal:"); ok {
			fields := strings.Fields(value)
			if len(fields) >= 2 && fields[1] == "kB" {
				kib, _ := strconv.ParseUint(fields[0], 10, 64)
				return kib * 1024
			}
		}
	}
	return 0
}

func availableAt(root string) (string, uint64) {
	if root == "" {
		return "", 0
	}
	path, err := filepath.Abs(root)
	if err != nil {
		return "", 0
	}
	for {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(path, &stat); err == nil {
			return path, stat.Bavail * uint64(stat.Bsize)
		} else if !errors.Is(err, os.ErrNotExist) {
			return path, 0
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path, 0
		}
		path = parent
	}
}

func valueOrUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

func boolDetail(ok bool, good, bad string) string {
	if ok {
		return good
	}
	return bad
}

// PreparationPrerequisitesMet keeps host admission while deferring the
// provisional free-space floor to the installer catalog's stronger allocation
// calculation. This permits cleanup of journaled partial image staging before
// measuring remaining space again. It is not permission to activate workloads.
func (r Report) PreparationPrerequisitesMet() bool {
	if r.Host.AvailableDiskBytes == 0 || len(r.Checks) == 0 {
		return false
	}
	diskSeen := false
	for _, check := range r.Checks {
		if check.ID == "disk" {
			diskSeen = true
			continue
		}
		if check.Status == Fail {
			return false
		}
	}
	return diskSeen
}
