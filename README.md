# 🐇 Burrow

**A container runtime in Go, from scratch — pull an image, run it, harden it, network it, watch it work.**

Burrow pulls a real **OCI image** from a registry, isolates it with **Linux
namespaces** and **cgroups v2**, layers a **copy-on-write overlayfs** root,
gives it **internet access** over a NAT'd veth pair, honours the image's
**ENTRYPOINT/ENV**, hardens it with **seccomp** + **capability dropping**,
runs **rootless** via **user namespaces**, and ships `ps`, `images` and a live
**inspector TUI**. A *runc / crun / podman in miniature* — built to understand
what container runtimes, and platforms like **OpenShift**, do under the hood,
in ~1,300 lines of dependency-light Go.

> ⚠️ **Educational, not production.** A small, readable codebase you can reason
> about end to end — not a hardened runc/crun replacement.

## Features

| Area | What Burrow does |
|---|---|
| **Namespaces** | UTS, PID (command is **PID 1**), mount, IPC, optional network + user |
| **cgroups v2** | `--mem 64m` and `--cpu 0.5`, kernel-enforced |
| **OCI images** | `--image alpine` — a **from-scratch registry v2 client** (token auth, multi-arch index, gzip layers + whiteouts) |
| **Image config** | honours the image's **ENTRYPOINT / CMD / ENV / WORKDIR** (`burrow run --image hello-world` just works) |
| **overlayfs** | copy-on-write root — writes never touch the base image |
| **Networking** | `--net` → veth + **NAT/MASQUERADE + DNS**: the container reaches the **internet** |
| **seccomp** | `--seccomp` — a hand-built **classic-BPF** filter `EPERM`s dangerous syscalls |
| **Capabilities** | `--drop-caps` — empties all sets + bounding set via raw `capset`/`prctl` |
| **Rootless** | `--userns` — container uid 0 → your unprivileged host uid (no sudo) |
| **Tooling** | `burrow ps`, `burrow images`, `burrow inspect` (live Bubble Tea TUI) |
| **Correctness** | start-sync pipe (no start races), signal-safe cleanup, stale-cgroup pruning |

## Quickstart

```bash
make build

sudo ./burrow run --image alpine /bin/sh                 # real Alpine shell
sudo ./burrow run --image hello-world                    # uses the image ENTRYPOINT
sudo ./burrow run --image alpine --mem 32m --cpu 0.5 /bin/sh
sudo ./burrow run --image alpine --net /bin/sh           # internet via NAT + DNS
sudo ./burrow run --image alpine --seccomp --drop-caps /bin/sh
./burrow run --userns /bin/sh                            # rootless, no sudo

sudo ./burrow ps          # running containers
./burrow images           # pulled images
sudo ./burrow inspect     # live TUI of a running container
```

> Requires Linux with cgroup v2 (default on modern kernels, incl. **WSL2** 6.x).
> Everything except `--userns` needs root. Networking needs `iptables`.

## Demos you can reproduce

```console
$ sudo ./burrow run --image alpine --net /bin/sh -c 'wget -qO- http://example.com | grep -o "<title>.*</title>"'
<title>Example Domain</title>                       # real DNS + HTTP over NAT

$ sudo ./burrow run --image alpine --seccomp /bin/sh -c 'mount -t tmpfs none /mnt'
mount: permission denied                            # seccomp EPERMs mount(2)

$ sudo ./burrow run --image alpine --drop-caps /bin/sh -c 'grep CapEff /proc/self/status; hostname x'
CapEff: 0000000000000000                            # zero capabilities
hostname: sethostname: Operation not permitted

$ ./burrow run --userns /bin/sh -c 'id; cat /proc/self/uid_map'
uid=0(root) gid=0(root)
         0       1000          1                    # root inside -> host uid 1000
```

## How it works

`burrow run` re-executes its **own binary** as a hidden `child`, launched with
`CLONE_NEWUTS|NEWPID|NEWNS|NEWIPC` (`+NEWNET`/`+NEWUSER` on demand). The child
sets its hostname, makes `/` private, mounts an **overlayfs** of the image and
`chroot`s in, mounts `/proc` + `/sys`, then **blocks on a sync pipe** while the
parent writes cgroup limits and NATs a veth pair into its netns. Released, the
child applies WORKDIR, drops capabilities, installs the seccomp filter, and
`syscall.Exec`s the (image ENTRYPOINT or given) command — which becomes PID 1.

```
burrow run ─clone(NEWUTS|NEWPID|NEWNS|NEWIPC[|NEWNET][|NEWUSER])─► child
                       │ sethostname · mount --make-rprivate /
                       │ overlayfs(lower=image) · chroot · mount /proc,/sys
                       │ ── block on sync pipe (fd 3) ───────────────┐
   parent: pull image+config · cgroup mem/cpu · veth+NAT+DNS ────────┘ release
                       │ chdir WORKDIR · drop caps (capset) · seccomp (BPF)
                       └ exec(ENTRYPOINT/cmd)  ← PID 1
```

## Roadmap

- [x] Namespaces + cgroups v2 (mem + cpu)
- [x] Live inspector TUI · overlayfs CoW · veth networking
- [x] OCI image pull (from-scratch registry client) + image config (entrypoint/env)
- [x] Container internet via NAT + DNS
- [x] seccomp (BPF) · capability dropping · user namespaces (rootless)
- [x] `ps` / `images`, start-sync pipe, signal-safe cleanup
- [ ] Future — user-supplied seccomp/OCI `config.json` profiles; image signature
      verification; port publishing; cgroup delegation for unprivileged limits

## Development

Built and tested on **WSL2** (Ubuntu, kernel 6.6, cgroup v2, Go 1.22+).
Deps: `bubbletea`, `lipgloss`, `golang.org/x/sys`. `make vet && make test`.

## License

MIT © 2026 Raj Modi
