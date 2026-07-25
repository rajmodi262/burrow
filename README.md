# 🐇 Burrow

**A minimal, educational container runtime in Go — build your own container, then see how it works.**

Burrow isolates a process using **Linux namespaces** and **cgroups v2**, then
optionally pivots into its own root filesystem. It is a *runc / crun in
miniature*, written to understand what container runtimes — and therefore
platforms like **OpenShift** — actually do under the hood.

> ⚠️ **Educational, not production.** No seccomp, no capability dropping, no
> user-namespace mapping. The goal is a small, readable codebase you can reason
> about end to end.

## What it demonstrates

| Concept | How Burrow uses it |
|---|---|
| **UTS namespace** | the container gets its own hostname (`burrow`) |
| **PID namespace** | the command runs as **PID 1** with its own process tree |
| **Mount namespace** | a private mount tree + a fresh `/proc` |
| **cgroups v2** | a `memory.max` cap enforced by the kernel |
| **chroot / rootfs** | run against a real busybox root filesystem |

## Quickstart

```bash
# build
make build            # -> ./burrow

# run an isolated shell using the host's own binaries (no rootfs needed)
sudo ./burrow run /bin/sh
#   inside: `hostname` -> burrow,  `echo $$` -> 1

# cap memory at 16 MB via cgroups v2
sudo ./burrow run --mem 16m /bin/sh

# run against a self-contained busybox rootfs
./get-rootfs.sh
sudo ./burrow run --rootfs ./rootfs /bin/sh
```

> Requires Linux with a cgroup v2 unified hierarchy (default on modern kernels,
> including **WSL2** kernel 6.x). Namespace operations need root, so use `sudo`.

## How it works

`burrow run` re-executes its **own binary** as a hidden `child` subcommand,
but launches it with `CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS` set. The child
then sets its hostname, makes `/` private, (optionally) `chroot`s into a
rootfs, mounts a fresh `/proc`, and finally `syscall.Exec`s the target command
so that command *becomes* PID 1 — the same trick real runtimes use.

```
burrow run  ──clone(NEWUTS|NEWPID|NEWNS)──►  /proc/self/exe child
                                              │ sethostname("burrow")
                                              │ mount --make-rprivate /
                                              │ chroot(rootfs)      (optional)
                                              │ mount -t proc proc /proc
                                              └ exec(command)  ← PID 1
        parent then writes memory.max into a cgroup v2 group for the child
```

## Roadmap

- [x] **Day 1** — UTS/PID/mount namespaces + private `/proc` (isolated shell)
- [x] **Day 2** — cgroups v2 memory cap (`--mem`)
- [ ] **Day 3** — live `/proc` **inspector TUI** (namespaces vs host, cgroup usage, mounts)
- [ ] **Day 4** — real **OCI image** unpack + **overlayfs** copy-on-write rootfs
- [ ] **Day 5** — CPU cap (`cpu.max`), network namespace + `veth`, polish

## Known omissions (on purpose)

Security hardening you would find in runc/crun but not here: dropping Linux
capabilities, seccomp syscall filtering, user-namespace UID/GID mapping, and
read-only/masked kernel paths. These are great follow-ups — and good interview
talking points about *why* they matter.

## License

MIT © 2026 Raj Modi
