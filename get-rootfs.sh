#!/usr/bin/env bash
# Build a tiny rootfs from a static busybox so you can demo --rootfs
# without Docker. Creates ./rootfs with a handful of applets.
set -euo pipefail
BB_URL="https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
mkdir -p rootfs/bin rootfs/proc
echo ">>> downloading static busybox"
curl -fL "$BB_URL" -o rootfs/bin/busybox
chmod +x rootfs/bin/busybox
for a in sh ls cat echo hostname ps mount uname sleep; do
	ln -sf busybox "rootfs/bin/$a"
done
echo ">>> rootfs ready. Try:"
echo "    sudo ./burrow run --rootfs ./rootfs /bin/sh"
