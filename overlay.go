package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// mountOverlay layers a writable upper directory over a read-only rootfs
// (the lower layer), giving copy-on-write semantics -- the same mechanism
// Docker / OpenShift use to stack image layers. The upper + work dirs live on
// an ephemeral tmpfs, so all container writes vanish when it exits and the
// base rootfs is never modified. Returns the merged mountpoint to chroot into.
func mountOverlay(rootfs string) (string, error) {
	lower, err := filepath.Abs(rootfs)
	if err != nil {
		return "", err
	}
	root := "/run/burrow-overlay"
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	// Ephemeral scratch: upper/work disappear with the mount namespace.
	if err := syscall.Mount("tmpfs", root, "tmpfs", 0, ""); err != nil {
		return "", fmt.Errorf("tmpfs: %w", err)
	}
	upper := filepath.Join(root, "upper")
	work := filepath.Join(root, "work")
	merged := filepath.Join(root, "merged")
	for _, d := range []string{upper, work, merged} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
	}
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lower, upper, work)
	if err := syscall.Mount("overlay", merged, "overlay", 0, opts); err != nil {
		return "", fmt.Errorf("mount overlay: %w", err)
	}
	return merged, nil
}
