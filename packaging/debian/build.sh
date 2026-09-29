#!/bin/sh
set -eu
# Run on a Debian/Ubuntu build host. This creates a development package only;
# it neither signs a release nor provisions the owner's machine.
version=${1:-0.1.0~dev}
case "$version" in
  ''|*[!a-zA-Z0-9.+~:-]*) echo 'Invalid Debian version' >&2; exit 2 ;;
esac
case "$version" in [0-9]*) ;; *) echo 'Version must begin with a digit' >&2; exit 2 ;; esac
command -v dpkg-deb >/dev/null
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
output="$repo/artifacts"
mkdir -p "$output"
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT HUP INT TERM
root="$staging/package"
mkdir -p "$root/DEBIAN" "$root/usr/bin" "$root/usr/lib/homenode" "$root/usr/lib/homenode/guest" "$root/usr/share/homenode/web" "$root/usr/share/doc/homenode"
cd "$repo"
npm --prefix web run build
export GOOS=linux GOARCH=amd64 CGO_ENABLED=0
go build -trimpath -o "$root/usr/bin/homenode" ./cmd/homenode
go build -trimpath -o "$root/usr/lib/homenode/homenode-supervisor" ./cmd/homenode-supervisor
go build -trimpath -o "$root/usr/lib/homenode/homenode-transfer" ./cmd/homenode-transfer
go build -trimpath -o "$root/usr/lib/homenode/guest/homenode-guest" ./cmd/homenode-guest
cp -R web/dist/. "$root/usr/share/homenode/web/"
mkdir -p "$root/usr/share/homenode/systemd"
cp packaging/systemd/* "$root/usr/share/homenode/systemd/"
cp packaging/debian/OWNERSHIP.md "$root/usr/share/doc/homenode/"
cp docs/PROGRESS.md "$root/usr/share/doc/homenode/IMPLEMENTATION.md"
cat > "$root/DEBIAN/control" <<CONTROL
Package: homenode
Version: $version
Section: net
Priority: optional
Architecture: amd64
Maintainer: Pranav Reddy Gudipati <56127176+pranavreddyg17@users.noreply.github.com>
Depends: libvirt-daemon-system, libvirt-clients, qemu-system-x86, qemu-utils, apparmor
Description: Private home compute server (development build)
 Passkey control plane, isolated workload services, and browser interface.
 This unsigned development package requires explicit host provisioning.
CONTROL
# Normalize all installed permissions; package code is never writable by services.
find "$root" -type d -exec chmod 0755 {} +
find "$root" -type f -exec chmod 0644 {} +
chmod 0755 "$root/usr/bin/homenode" "$root/usr/lib/homenode/homenode-supervisor" "$root/usr/lib/homenode/homenode-transfer" "$root/usr/lib/homenode/guest/homenode-guest"
(cd "$root" && find usr -type f ! -name SHA256SUMS -print | LC_ALL=C sort | xargs sha256sum > usr/share/doc/homenode/SHA256SUMS)
archive="$output/homenode_${version}_amd64.deb"
dpkg-deb --root-owner-group --build "$root" "$archive"
sha256sum "$archive" > "$archive.sha256"
printf '%s\n' "$archive"
