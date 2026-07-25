package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// psCmd lists running Burrow containers, discovered from their cgroup dirs.
func psCmd(args []string) {
	matches, _ := filepath.Glob(filepath.Join(cgroupRoot, "burrow.*"))
	fmt.Printf("%-8s %-16s %-10s %s\n", "PID", "COMMAND", "MEMORY", "CGROUP")
	n := 0
	for _, m := range matches {
		var p int
		if _, err := fmt.Sscanf(filepath.Base(m), "burrow.%d", &p); err != nil || p == 0 {
			continue
		}
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", p)); err != nil {
			continue
		}
		comm := readFile(fmt.Sprintf("/proc/%d/comm", p))
		mem := human(readFile(filepath.Join(m, "memory.current")))
		fmt.Printf("%-8d %-16s %-10s %s\n", p, comm, mem, filepath.Base(m))
		n++
	}
	if n == 0 {
		fmt.Println("(no running containers)")
	}
}

// imagesCmd lists images pulled into the local cache.
func imagesCmd(args []string) {
	root := imageCacheRoot()
	entries, _ := os.ReadDir(root)
	fmt.Printf("%-28s %s\n", "IMAGE", "SIZE")
	n := 0
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, ".done")); err != nil {
			continue
		}
		ref := strings.TrimSpace(readFile(filepath.Join(dir, "ref")))
		if ref == "" {
			ref = e.Name()
		}
		size := human(fmt.Sprint(dirSize(filepath.Join(dir, "rootfs"))))
		fmt.Printf("%-28s %s\n", ref, size)
		n++
	}
	if n == 0 {
		fmt.Println("(no images)")
	}
}

func dirSize(path string) int64 {
	var total int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}
