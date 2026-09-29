//go:build linux

package doctor

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// SystemSource is the Doctor's view of the local host: hostname,
// kernel, OS, arch, uptime, CPU/memory/storage counters and file
// descriptor pressure. The source reads /proc and /etc/os-release
// directly; it never executes shell commands.
type SystemSource struct {
	GeneratedAt time.Time
	BootID      string
}

func (s *SystemSource) Name() string { return "system" }
func (s *SystemSource) Contribute(b *Builder) {
	b.SetSystem(readSystem())
	b.SetResources(readResources())
}

func readSystem() System {
	out := System{Architecture: runtime.GOARCH}
	if runtime.GOARCH == "arm" {
		out.Architecture = "armv7"
	}
	if h, err := os.Hostname(); err == nil {
		out.Hostname = strings.TrimSpace(h)
	}
	if data, err := readBounded("/proc/sys/kernel/osrelease"); err == nil {
		out.Kernel = strings.TrimSpace(string(data))
	}
	if data, err := readBounded("/etc/os-release"); err == nil {
		out.OS = parseOSRelease(data)
	}
	return out
}

func parseOSRelease(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			v = strings.Trim(v, "'\"")
			return v
		}
	}
	return ""
}

func readResources() Resources {
	r := Resources{CPULogicalCores: runtime.NumCPU()}
	if data, err := readBounded("/proc/loadavg"); err == nil {
		r.LoadAverage = parseLoad(data)
	}
	if data, err := readBounded("/proc/meminfo"); err == nil {
		r.Memory = parseMemory(data)
	}
	if data, err := readBounded("/proc/mounts"); err == nil {
		r.Storage = parseStorage(data)
	}
	if data, err := readBounded("/proc/sys/fs/file-nr"); err == nil {
		used, limit, ok := parseFileNR(data)
		if ok {
			r.FDUsed = used
			r.FDLimit = limit
		}
	}
	return r
}

func parseLoad(data []byte) []float64 {
	f := strings.Fields(string(data))
	if len(f) < 3 {
		return nil
	}
	out := make([]float64, 0, 3)
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return out
		}
		out = append(out, v)
	}
	return out
}

func parseMemory(data []byte) Memory {
	var total, available uint64
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = parseKBField(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			available = parseKBField(line)
		}
	}
	if total == 0 || available > total {
		return Memory{}
	}
	return Memory{TotalBytes: total, AvailableBytes: available, UsedBytes: total - available}
}

func parseKBField(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[2] != "kB" {
		return 0
	}
	v, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return v * 1024
}

func parseStorage(data []byte) []Storage {
	seen := map[string]bool{}
	var out []Storage
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		device, mountpoint, fs := fields[0], fields[1], fields[2]
		if seen[mountpoint] {
			continue
		}
		if !shouldIncludeMount(mountpoint, fs) {
			continue
		}
		seen[mountpoint] = true
		out = append(out, Storage{
			Mountpoint: mountpoint,
			Filesystem: fs,
			Device:     device,
		})
		if len(out) >= MaxStorage {
			break
		}
	}
	return out
}

func shouldIncludeMount(mountpoint, fs string) bool {
	if mountpoint == "" || mountpoint == "/proc" {
		return false
	}
	switch fs {
	case "proc", "sysfs", "devpts", "tmpfs", "devtmpfs", "cgroup", "cgroup2",
		"overlay", "squashfs", "autofs", "binfmt_misc", "configfs", "debugfs",
		"fusectl", "fuse.gvfsd-fuse", "hugetlbfs", "mqueue", "pstore",
		"ramfs", "rpc_pipefs", "securityfs", "selinuxfs", "tracefs", "nsfs":
		return false
	}
	return true
}

func parseFileNR(data []byte) (uint64, uint64, bool) {
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, false
	}
	used, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	limit, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return used, limit, true
}
