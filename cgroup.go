package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const cgroupRoot = "/sys/fs/cgroup"

// applyMemoryLimit creates a cgroups v2 group, writes memory.max, and moves
// the given pid into it. Requires a cgroup v2 unified hierarchy (the default
// on modern Linux, incl. WSL2 kernel 6.x).
func applyMemoryLimit(pid int, mem string) error {
	bytes, err := parseMemory(mem)
	if err != nil {
		return err
	}
	dir := filepath.Join(cgroupRoot, fmt.Sprintf("burrow.%d", pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create cgroup: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "memory.max"),
		[]byte(strconv.FormatInt(bytes, 10)), 0o644); err != nil {
		return fmt.Errorf("set memory.max: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cgroup.procs"),
		[]byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("attach pid: %w", err)
	}
	return nil
}

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
