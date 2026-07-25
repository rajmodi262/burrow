// Burrow is a minimal, educational container runtime written in Go.
//
// It isolates a process using Linux namespaces (UTS, PID, mount, IPC and
// optionally network) and cgroups v2 (memory + CPU limits), then optionally
// pivots into its own root filesystem. A companion `inspect` command renders a
// live view of a running container's namespaces, cgroup usage and mounts.
//
// It exists to show what real runtimes such as runc / crun -- and therefore
// platforms like OpenShift -- do under the hood.
//
// Usage:
//
//	sudo burrow run [--mem 64m] [--cpu 0.5] [--net] [--rootfs DIR] <cmd> [args...]
//	sudo burrow inspect [--once] [PID]
//	burrow version
//
// NOT for production: no seccomp, no capability dropping, no user-namespace
// mapping. It is a learning tool, deliberately kept small and readable.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

const version = "0.4.0"

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
	case "inspect":
		inspect(os.Args[2:])
	case "pull":
		pullCmd(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("burrow", version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `burrow - a minimal, educational container runtime

usage:
  sudo burrow run [flags] <cmd> [args...]
    flags: --mem 64m  --cpu 0.5  --net  --image NAME|--rootfs DIR
           --drop-caps  --seccomp  --userns
  sudo burrow pull <image>               pull an OCI image into a local rootfs
  sudo burrow inspect [--once] [PID]     live view of a running container
  burrow version`)
}

type runOpts struct {
	mem, cpu, rootfs, image        string
	net, dropCaps, seccomp, userns bool
	cmd                            []string
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
		case "--cpu":
			i++
			if i < len(args) {
				o.cpu = args[i]
			}
		case "--rootfs":
			i++
			if i < len(args) {
				o.rootfs = args[i]
			}
		case "--image":
			i++
			if i < len(args) {
				o.image = args[i]
			}
		case "--net":
			o.net = true
		case "--drop-caps":
			o.dropCaps = true
		case "--seccomp":
			o.seccomp = true
		case "--userns":
			o.userns = true
		default:
			o.cmd = args[i:]
			return o
		}
		i++
	}
	return o
}

// run launches the target command inside fresh namespaces, wires up a cgroup
// (always, for tracking + optional limits + inspection), optionally sets up a
// veth network pair, and cleans everything up -- even on Ctrl-C.
func run(args []string) {
	o := parseRunArgs(args)
	if len(o.cmd) == 0 {
		usage()
		os.Exit(2)
	}
	pruneStaleCgroups()

	if o.image != "" {
		rf, err := pullImage(o.image)
		if err != nil {
			fmt.Fprintln(os.Stderr, "burrow: pull:", err)
			os.Exit(1)
		}
		o.rootfs = rf
	}

	childArgs := []string{"child"}
	if o.rootfs != "" {
		childArgs = append(childArgs, "--rootfs", o.rootfs)
	}
	if o.dropCaps {
		childArgs = append(childArgs, "--drop-caps")
	}
	if o.seccomp {
		childArgs = append(childArgs, "--seccomp")
	}
	childArgs = append(childArgs, o.cmd...)

	cmd := exec.Command("/proc/self/exe", childArgs...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	// Start-sync pipe: the child sets up its namespaces then blocks on fd 3
	// until we finish cgroup + network wiring, so nothing races the container
	// start (this is how runc/crun avoid the same race).
	syncR, syncW, perr := os.Pipe()
	if perr != nil {
		fmt.Fprintln(os.Stderr, "burrow: pipe:", perr)
		os.Exit(1)
	}
	cmd.ExtraFiles = []*os.File{syncR}

	var flags uintptr = syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID |
		syscall.CLONE_NEWNS | syscall.CLONE_NEWIPC
	if o.net {
		flags |= syscall.CLONE_NEWNET
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   flags,
		Unshareflags: syscall.CLONE_NEWNS,
	}
	if o.userns {
		cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWUSER
		cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}}
		cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}}
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "burrow: start:", err)
		os.Exit(1)
	}
	pid := cmd.Process.Pid
	syncR.Close()

	cg, err := newCgroup(pid)
	if err != nil {
		warn("cgroup", err)
	}
	if cg != nil {
		if o.mem != "" {
			if e := cg.setMemoryMax(o.mem); e != nil {
				warn("memory limit", e)
			}
		}
		if o.cpu != "" {
			if e := cg.setCPUMax(o.cpu); e != nil {
				warn("cpu limit", e)
			}
		}
		if e := cg.addProc(pid); e != nil {
			warn("attach pid", e)
		}
	}

	var netObj *veth
	if o.net {
		netObj = newVeth(pid)
		if e := netObj.setup(); e != nil {
			warn("network", e)
		}
	}

	// Everything is wired -- let the container's command actually start.
	_, _ = syncW.Write([]byte{1})
	syncW.Close()

	// Forward Ctrl-C / SIGTERM to the container so our deferred cleanup
	// (cgroup removal, veth teardown) always runs.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		_ = cmd.Process.Signal(syscall.SIGKILL)
	}()

	waitErr := cmd.Wait()

	// Explicit cleanup so it runs on every exit path -- normal exit AND
	// Ctrl-C / SIGTERM (os.Exit would skip deferred cleanup).
	if cg != nil {
		_ = cg.remove()
	}
	if netObj != nil {
		netObj.teardown()
	}
	if waitErr != nil {
		os.Exit(1)
	}
}

// child runs inside the new namespaces: it sets the hostname, makes the mount
// namespace private, optionally chroots into a rootfs, mounts a fresh /proc,
// and finally becomes (execs) the target command -- PID 1 of the container.
func child(args []string) {
	var rootfs string
	dropCaps, seccomp := false, false
	i := 0
childflags:
	for i < len(args) {
		switch args[i] {
		case "--rootfs":
			i++
			if i < len(args) {
				rootfs = args[i]
				i++
			}
		case "--drop-caps":
			dropCaps = true
			i++
		case "--seccomp":
			seccomp = true
			i++
		default:
			break childflags
		}
	}
	args = args[i:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	must("sethostname", syscall.Sethostname([]byte("burrow")))
	must("mount-private", syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""))

	if rootfs != "" {
		target := rootfs
		if merged, err := mountOverlay(rootfs); err == nil {
			target = merged // copy-on-write: writes go to the overlay upper layer
		} else {
			fmt.Fprintln(os.Stderr, "burrow child: overlay unavailable, using rootfs directly:", err)
		}
		must("chroot", syscall.Chroot(target))
		must("chdir", syscall.Chdir("/"))
	}

	_ = os.MkdirAll("/proc", 0o555)
	must("mount-proc", syscall.Mount("proc", "/proc", "proc", 0, ""))

	// Wait for the parent to finish cgroup + network setup.
	if f := os.NewFile(3, "sync"); f != nil {
		var one [1]byte
		_, _ = f.Read(one[:])
		f.Close()
	}

	// Apply hardening last -- after our own privileged setup (mount, chroot).
	if dropCaps {
		must("drop-caps", dropCapabilities())
	}
	if seccomp {
		must("seccomp", installSeccomp())
	}

	path, err := exec.LookPath(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "burrow child: command not found:", args[0])
		os.Exit(127)
	}
	must("exec", syscall.Exec(path, args, os.Environ()))
}

func must(what string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "burrow child: %s: %v\n", what, err)
		os.Exit(1)
	}
}

func warn(what string, err error) {
	fmt.Fprintf(os.Stderr, "burrow: warning: %s: %v\n", what, err)
}
