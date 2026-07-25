package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// pruneStaleCgroups removes leftover burrow.<pid> cgroup directories whose
// process is no longer alive -- e.g. after a hard SIGKILL that skipped the
// normal deferred cleanup. Called at the start of every `run`.
func pruneStaleCgroups() {
	matches, _ := filepath.Glob(filepath.Join(cgroupRoot, "burrow.*"))
	for _, m := range matches {
		var p int
		if _, err := fmt.Sscanf(filepath.Base(m), "burrow.%d", &p); err != nil || p == 0 {
			continue
		}
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", p)); err != nil {
			_ = os.Remove(m)
		}
	}
}
