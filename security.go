package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// capLast is a safe upper bound for the highest capability number to drop from
// the bounding set (currently CAP_CHECKPOINT_RESTORE = 40). Prctl silently
// ignores numbers the kernel doesn't know, so over-shooting is harmless.
const capLast = 40

// _LINUX_CAPABILITY_VERSION_3 selects the 64-bit capability ABI (two u32 words).
const capVersion3 = 0x20080522

// dropCapabilities strips every Linux capability from the container's PID 1:
// it clears the effective/permitted/inheritable sets via capset(2), drops the
// whole bounding set via prctl(PR_CAPBSET_DROP), and sets no_new_privs so a
// setuid binary can't regain them. The process stays uid 0 but loses the power
// to mount, sethostname, load modules, etc. -- the crux of container hardening.
func dropCapabilities() error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("no_new_privs: %w", err)
	}
	for c := 0; c <= capLast; c++ {
		_ = unix.Prctl(unix.PR_CAPBSET_DROP, uintptr(c), 0, 0, 0)
	}
	hdr := unix.CapUserHeader{Version: capVersion3, Pid: 0} // 0 == calling thread
	var data [2]unix.CapUserData                            // all-zero: empty sets
	if err := unix.Capset(&hdr, &data[0]); err != nil {
		return fmt.Errorf("capset: %w", err)
	}
	return nil
}

// x86_64 syscall numbers for a representative "dangerous" blocklist.
var seccompBlock = map[int]string{
	165: "mount", 166: "umount2", 161: "chroot", 101: "ptrace",
	169: "reboot", 167: "swapon", 168: "swapoff",
	175: "init_module", 313: "finit_module", 176: "delete_module",
	246: "kexec_load", 272: "unshare", 308: "setns",
}

const (
	auditArchX86_64    = 0xC000003E
	seccompRetAllow    = 0x7FFF0000
	seccompRetErrno    = 0x00050000
	seccompRetKillProc = 0x80000000
	seccompModeFilter  = 1

	bpfLDW = 0x20 // BPF_LD | BPF_W | BPF_ABS
	bpfJEQ = 0x15 // BPF_JMP | BPF_JEQ | BPF_K
	bpfRET = 0x06 // BPF_RET | BPF_K
)

// installSeccomp sets no_new_privs and loads a classic-BPF seccomp filter that
// returns EPERM for the blocklisted syscalls and allows everything else -- the
// same mechanism the OCI runtime spec uses, here assembled by hand instead of
// via libseccomp.
func installSeccomp() error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("no_new_privs: %w", err)
	}

	var f []unix.SockFilter
	stmt := func(code uint16, k uint32) { f = append(f, unix.SockFilter{Code: code, K: k}) }
	jump := func(code uint16, k uint32, jt, jf uint8) {
		f = append(f, unix.SockFilter{Code: code, Jt: jt, Jf: jf, K: k})
	}

	n := len(seccompBlock)
	allowIdx := 3 + n
	errnoIdx := allowIdx + 1
	killIdx := errnoIdx + 1

	stmt(bpfLDW, 4)                                    // [0] A = arch
	jump(bpfJEQ, auditArchX86_64, 0, uint8(killIdx-2)) // [1] arch ok? else kill
	stmt(bpfLDW, 0)                                    // [2] A = syscall nr
	for k := range seccompBlock {                      // [3..] block checks
		off := uint8(errnoIdx - (len(f) + 1))
		jump(bpfJEQ, uint32(k), off, 0)
	}
	stmt(bpfRET, seccompRetAllow)                    // allow
	stmt(bpfRET, seccompRetErrno|uint32(unix.EPERM)) // EPERM
	stmt(bpfRET, seccompRetKillProc)                 // kill (arch mismatch)

	prog := unix.SockFprog{Len: uint16(len(f)), Filter: &f[0]}
	if _, _, errno := unix.Syscall(unix.SYS_SECCOMP, seccompModeFilter, 0,
		uintptr(unsafe.Pointer(&prog))); errno != 0 {
		return fmt.Errorf("seccomp load: %v", errno)
	}
	return nil
}
