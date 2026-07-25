# 🐇 Burrow

**A container runtime in Go, from scratch — build a container, harden it, and watch it work.**

Burrow pulls a real **OCI image**, isolates it with **Linux namespaces** and
**cgroups v2**, layers a **copy-on-write overlayfs** root, gives it its own
**network** over a veth pair, hardens it with **seccomp** + **capability
dropping**, supports **rootless** execution via **user namespaces**, and ships a
**live TUI inspector**. It is a *runc / crun in miniature* — written to
understand what container runtimes, and platforms like **OpenShift**, actually
do under the hood, in ~1,000 lines of dependency-light Go.

> ⚠️ **Educational, not production.** The goal is a small, readable codebase you
> can reason about end to end — not a hardened replacement for runc/crun.

## Features

| Area | What Burrow does |
|---|---|
| **Namespaces** | UTS, PID (command runs as **PID 1**), mount, IPC, and optional network |
| **cgroups v2** | `--mem 64m` (`memory.max`) and `--cpu 0.5` (`cpu.max`), kernel-enforced |
| **OCI images** | `--image alpine` pulls from Docker Hub via a **from-scratch registry v2 client** (pure stdlib) |
| **overlayfs** | copy-on-write root — container writes never touch the base image |
| **networking** | `--net` → own netns + **veth pair** (host `10.200.1.1` ↔ container `10.200.1.2`) |
| **seccomp** | `--seccomp` loads a hand-built **classic-BPF** filter that `EPERM`s dangerous syscalls |
| **capabilities** | `--drop-caps` empties all cap sets + bounding set via raw `capset`/`prctl` |
| **rootless** | `--userns` maps container uid 0 → your unprivileged host uid (no sudo) |
| **inspector** | `burrow inspect` → live Bubble Tea TUI of namespaces, cgroup usage, mounts |
| **correctness** | start-sync pipe (no start races), signal-safe cleanup, stale-cgroup pruning |

## Quickstart

```bash
make build

# pull and run a real Alpine image
sudo ./burrow run --image alpine /bin/sh

# resource limits, enforced by the kernel
sudo ./burrow run --image alpine --mem 32m --cpu 0.5 /bin/sh

# its own network namespace + veth pair
sudo ./burrow run --image alpine --net /bin/sh

# hardened: seccomp filter + zero capabilities
sudo ./burrow run --image alpine --seccomp --drop-caps /bin/sh

# rootless container (NO sudo): container root == your host user
./burrow run --userns /bin/sh

# watch a running container live, in another terminal
sudo ./burrow inspect
```

> Requires Linux with cgroup v2 (default on modern kernels, incl. **WSL2** 6.x).
> Everything except `--userns` needs root, so use `sudo`.

## Security demos you can reproduce

```console
$ sudo ./burrow run --image alpine --seccomp /bin/sh -c 'mount -t tmpfs none /mnt'
mount: permission denied (are you root?)        # seccomp EPERMs the mount(2) syscall

$ sudo ./burrow run --image alpine --drop-caps /bin/sh -c 'grep CapEff /proc/self/status; hostname x'
CapEff: 0000000000000000                        # every capability dropped
hostname: sethostname: Operation not permitted  # ...so root can't rename the host

$ ./burrow run --userns /bin/sh -c 'id; cat /proc/self/uid_map'
uid=0(root) gid=0(root)                         # root inside...
         0       1000          1                # ...mapped to host uid 1000 outside
```

## How it works

`burrow run` re-executes its **own binary** as a hidden `child`, launched with
`CLONE_NEWUTS|NEWPID|NEWNS|NEWIPC` (`+NEWNET` with `--net`, `+NEWUSER` with
`--userns`). The child sets its hostname, makes `/` private, mounts an
**overlayfs** and `chroot`s into it, mounts a fresh `/proc`, then **blocks on a
sync pipe** while the parent writes cgroup limits and moves a veth end into its
netns. Released, the child drops capabilities, installs the seccomp filter, and
`syscall.Exec`s the target — which *becomes* PID 1.

```
burrow run ─clone(NEWUTS|NEWPID|NEWNS|NEWIPC[|NEWNET][|NEWUSER])─► child
                       │ sethostname / mount --make-rprivate /
                       │ mount -t overlay (lower = image rootfs) ; chroot
                       │ mount -t proc proc /proc
                       │ ── block on sync pipe (fd 3) ──────────────┐
   parent: pull image · cgroup memory.max/cpu.max · veth pair ──────┘ (release)
                       │ drop capabilities (capset) ; install seccomp (BPF)
                       └ exec(command)  ← PID 1
```

## Roadmap

- [x] Namespaces (UTS/PID/mount/IPC) + private `/proc`
- [x] cgroups v2 memory **and** CPU limits
- [x] Live `/proc` inspector TUI (Bubble Tea + Lipgloss)
- [x] overlayfs copy-on-write rootfs
- [x] Network namespace + veth pair
- [x] **OCI image pull** from a registry (from-scratch v2 client)
- [x] **seccomp** BPF filtering + **capability dropping**
- [x] **user namespaces** (rootless) + start-sync pipe
- [ ] Future — user-supplied seccomp/OCI `config.json` profiles; rootless
      outbound networking (slirp); image signature verification; cgroup
      delegation for unprivileged limits

## Known omissions (on purpose)

`/sys` is not mounted; networking has no NAT/outbound path (host↔container
only); the seccomp blocklist and dropped-caps set are fixed rather than
policy-driven; no image signature/layer-digest verification. Each is a good
"what would it take to do this properly?" discussion.

## Development

Built and tested on **WSL2** (Ubuntu, kernel 6.6, cgroup v2, Go 1.22+).
Dependencies: `bubbletea`, `lipgloss`, `golang.org/x/sys`.

```bash
make vet && make test && make build
```

## License

MIT © 2026 Raj Modi
