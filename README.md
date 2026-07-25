# 🐇 Burrow

**A minimal, educational container runtime in Go — build your own container, then watch it work.**

Burrow isolates a process with **Linux namespaces** and **cgroups v2**, layers a
**copy-on-write overlayfs** root filesystem, optionally gives it its own
**network namespace** over a veth pair, and ships a **live TUI inspector** that
shows the isolation happening in real time. It is a *runc / crun in miniature*,
written to understand what container runtimes — and therefore platforms like
**OpenShift** — actually do under the hood.

> ⚠️ **Educational, not production.** No seccomp, no capability dropping, no
> user-namespace mapping. The goal is a small, readable codebase you can reason
> about end to end (~700 lines of Go).

## Features

| Area | What Burrow does |
|---|---|
| **UTS namespace** | container gets its own hostname (`burrow`) |
| **PID namespace** | the command runs as **PID 1** with its own process tree |
| **Mount + IPC namespaces** | private mount tree, fresh `/proc`, isolated IPC |
| **Network namespace** | `--net` → own netns + **veth pair** (host `10.200.1.1` ↔ container `10.200.1.2`) |
| **cgroups v2** | `--mem 64m` (`memory.max`) and `--cpu 0.5` (`cpu.max`) enforced by the kernel |
| **overlayfs rootfs** | `--rootfs DIR` → copy-on-write; container writes never touch the base image |
| **live inspector** | `burrow inspect` → a Bubble Tea TUI of namespaces, cgroup usage and mounts |
| **clean lifecycle** | cgroups/veth removed on exit *and* on Ctrl-C; stale groups pruned on start |

## Quickstart

```bash
make build                                   # -> ./burrow

sudo ./burrow run /bin/sh                     # isolated shell (host binaries)
sudo ./burrow run --mem 32m --cpu 0.5 /bin/sh # kernel-enforced limits
sudo ./burrow run --net /bin/sh               # own network namespace + veth

./get-rootfs.sh                               # tiny busybox rootfs -> ./rootfs
sudo ./burrow run --rootfs ./rootfs /bin/sh   # overlayfs copy-on-write root

# in another terminal, watch a running container live:
sudo ./burrow inspect
```

> Requires Linux with a cgroup v2 unified hierarchy (default on modern kernels,
> including **WSL2** kernel 6.x). Namespace operations need root, so use `sudo`.

## The inspector

`burrow inspect` auto-discovers the most recent running container (or takes a
PID) and refreshes every 0.5 s. `--once` prints a single snapshot:

```
container  pid=10389  comm=sleep  hostname=burrow

namespaces  (isolated = different inode than host)
  uts     uts:[4026532290]         ISOLATED
  pid     pid:[4026532296]         ISOLATED
  mnt     mnt:[4026532298]         ISOLATED
  net     net:[4026531840]         shared
  ipc     ipc:[4026532295]         ISOLATED
  user    user:[4026531837]        shared
  cgroup  cgroup:[4026531835]      shared

cgroup  /burrow.10389
  memory.current = 1.3 MiB    memory.max = 32.0 MiB
  cpu.max        = 50000 100000 cpu.usage  = 7564 us

mounts  42 total (showing up to 6)
  /
  /dev
  ...
```

## How it works

`burrow run` re-executes its **own binary** as a hidden `child` subcommand,
launched with `CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS | CLONE_NEWIPC`
(`+ CLONE_NEWNET` with `--net`). The child sets its hostname, makes `/`
private, mounts an **overlayfs** and `chroot`s into it (with `--rootfs`),
mounts a fresh `/proc`, and finally `syscall.Exec`s the target so it *becomes*
PID 1. The parent then writes the cgroup v2 limits and, for `--net`, moves one
end of a veth pair into the child's netns.

```
burrow run ─clone(NEWUTS|NEWPID|NEWNS|NEWIPC[|NEWNET])─► /proc/self/exe child
                                          │ sethostname("burrow")
                                          │ mount --make-rprivate /
                                          │ mount -t overlay (lower=rootfs)   (--rootfs)
                                          │ chroot(merged)
                                          │ mount -t proc proc /proc
                                          └ exec(command)  ← PID 1
     parent: cgroup v2 memory.max / cpu.max  +  veth pair (--net)  +  cleanup
```

## Roadmap

- [x] **Day 1** — UTS/PID/mount/IPC namespaces + private `/proc`
- [x] **Day 2** — cgroups v2 **memory** cap
- [x] **Day 3** — live `/proc` **inspector TUI** (Bubble Tea + Lipgloss)
- [x] **Day 4** — **overlayfs** copy-on-write rootfs
- [x] **Day 5** — **CPU** cap (`cpu.max`), **network namespace + veth**, signal-safe cleanup
- [ ] Future — pull/unpack a real **OCI image** from a registry; **seccomp** + capability dropping; user-namespace UID/GID mapping

## Known omissions (on purpose)

Security hardening you would find in runc/crun but not here: dropping Linux
capabilities, seccomp syscall filtering, user-namespace UID/GID mapping,
read-only/masked kernel paths, and registry image pulls. These are great
follow-ups — and good interview talking points about *why* they matter.

## Development

Built and tested on **WSL2** (Ubuntu, kernel 6.6, cgroup v2, Go 1.22+).

```bash
make vet && make test     # unit tests cover the parsing/formatting helpers
make build
```

## License

MIT © 2026 Raj Modi
