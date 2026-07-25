package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const cgroupRoot = "/sys/fs/cgroup"

// cgroup is a single cgroups v2 group living at cgroupRoot/burrow.<pid>.
type cgroup struct{ dir string }

func newCgroup(pid int) (*cgroup, error) {
	dir := filepath.Join(cgroupRoot, fmt.Sprintf("burrow.%d", pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create cgroup: %w", err)
	}
	return &cgroup{dir: dir}, nil
}

func (c *cgroup) write(file, val string) error {
	return os.WriteFile(filepath.Join(c.dir, file), []byte(val), 0o644)
}

func (c *cgroup) setMemoryMax(mem string) error {
	b, err := parseMemory(mem)
	if err != nil {
		return err
	}
	return c.write("memory.max", strconv.FormatInt(b, 10))
}

func (c *cgroup) setCPUMax(cpus string) error {
	quota, err := parseCPU(cpus)
	if err != nil {
		return err
	}
	return c.write("cpu.max", quota)
}

func (c *cgroup) addProc(pid int) error {
	return c.write("cgroup.procs", strconv.Itoa(pid))
}

func (c *cgroup) remove() error { return os.Remove(c.dir) }

// parseMemory converts "64m", "1g", "512k" or a raw byte count into bytes.
func parseMemory(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty memory value")
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'k':
		mult, s = 1<<10, s[:len(s)-1]
	case 'm':
		mult, s = 1<<20, s[:len(s)-1]
	case 'g':
		mult, s = 1<<30, s[:len(s)-1]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q", s)
	}
	return n * mult, nil
}

// parseCPU turns a fractional core count ("0.5", "1", "2") into a cgroup v2
// cpu.max string: "<quota_us> <period_us>" over a 100ms period.
func parseCPU(s string) (string, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 {
		return "", fmt.Errorf("invalid cpu value %q", s)
	}
	const period = 100000
	quota := int64(f * float64(period))
	return fmt.Sprintf("%d %d", quota, period), nil
}
