// Burrow is a minimal, educational container runtime written in Go.
//
// It isolates a process using Linux namespaces (UTS, PID, mount) and a
// cgroups v2 memory limit, then optionally pivots into its own root
// filesystem -- demonstrating what real runtimes such as runc / crun
// (and therefore OpenShift) do under the hood.
//
// Usage:
//
//	sudo burrow run [--mem 64m] [--rootfs ./rootfs] <command> [args...]
//
// NOT for production: no seccomp, no capability dropping, no user-namespace
// mapping. It is a learning tool, on purpose kept small and readable.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		run(os.Args[2:])
	case "child": // internal: re-executed inside the new namespaces
		child(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("burrow", version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: sudo burrow run [--mem 64m] [--rootfs DIR] <command> [args...]")
}

type runOpts struct {
	mem    string
	rootfs string
	cmd    []string
}

func parseRunArgs(args []string) runOpts {
	var o runOpts
	i := 0
	for i < len(args) {
		switch args[i] {
		case "--mem":
			i++
			if i < len(args) {
				o.mem = args[i]
			}
		case "--rootfs":
			i++
			if i < len(args) {
				o.rootfs = args[i]
			}
		default:
			o.cmd = args[i:]
			return o
		}
		i++
	}
	return o
}

// run re-executes this same binary as "child" inside a fresh set of
// namespaces, then (best effort) caps the child's memory with cgroups v2.
func run(args []string) {
	o := parseRunArgs(args)
	if len(o.cmd) == 0 {
		usage()
		os.Exit(2)
	}

	childArgs := []string{"child"}
	if o.rootfs != "" {
		childArgs = append(childArgs, "--rootfs", o.rootfs)
	}
	childArgs = append(childArgs, o.cmd...)

	cmd := exec.Command("/proc/self/exe", childArgs...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// New UTS (hostname), PID (own process tree) and mount namespaces.
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		// Keep our mounts from leaking back to the host.
		Unshareflags: syscall.CLONE_NEWNS,
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "burrow: start:", err)
		os.Exit(1)
	}

	if o.mem != "" {
		if err := applyMemoryLimit(cmd.Process.Pid, o.mem); err != nil {
			fmt.Fprintln(os.Stderr, "burrow: warning: memory limit not applied:", err)
		}
	}

	if err := cmd.Wait(); err != nil {
		os.Exit(1)
	}
}

// child runs inside the new namespaces. It sets the hostname, makes the
// mount namespace private, optionally chroots into a rootfs, mounts a fresh
// /proc, and finally *becomes* the target command (PID 1 of the container).
func child(args []string) {
	rootfs := ""
	if len(args) >= 2 && args[0] == "--rootfs" {
		rootfs, args = args[1], args[2:]
	}
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	must("sethostname", syscall.Sethostname([]byte("burrow")))
	// Make "/" private (recursively) so mounts below don't propagate out.
	must("mount-private", syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""))

	if rootfs != "" {
		must("chroot", syscall.Chroot(rootfs))
		must("chdir", syscall.Chdir("/"))
	}

	_ = os.MkdirAll("/proc", 0o555)
	must("mount-proc", syscall.Mount("proc", "/proc", "proc", 0, ""))

	path, err := exec.LookPath(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "burrow child: command not found:", args[0])
		os.Exit(127)
	}
	// Replace this process image, so the command becomes PID 1 in the
	// container -- exactly how an init process behaves in a real container.
	must("exec", syscall.Exec(path, args, os.Environ()))
}

func must(what string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "burrow child: %s: %v\n", what, err)
		os.Exit(1)
	}
}
