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
command -v python3 >/dev/null
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
go build -trimpath -o "$root/usr/lib/homenode/homenode-backup" ./cmd/homenode-backup
go build -trimpath -o "$root/usr/lib/homenode/homenode-inspect" ./cmd/homenode-inspect
go build -trimpath -o "$root/usr/lib/homenode/guest/homenode-guest" ./cmd/homenode-guest
cp -R web/dist/. "$root/usr/share/homenode/web/"
mkdir -p "$root/usr/share/homenode/systemd"
cp packaging/systemd/*.service packaging/systemd/*.socket packaging/systemd/*.slice packaging/systemd/README.md "$root/usr/share/homenode/systemd/"
cp packaging/debian/OWNERSHIP.md "$root/usr/share/doc/homenode/"
cp docs/PROGRESS.md "$root/usr/share/doc/homenode/IMPLEMENTATION.md"
cat > "$root/DEBIAN/control" <<CONTROL
Package: homenode
Version: $version
Section: net
Priority: optional
Architecture: amd64
Maintainer: Pranav Reddy Gudipati <56127176+pranavreddyg17@users.noreply.github.com>
Depends: libvirt-daemon-system, libvirt-clients, qemu-system-x86, qemu-utils, e2fsprogs, apparmor, restic
Description: Private home compute server (development build)
 Passkey control plane, isolated workload services, and browser interface.
 This unsigned development package requires explicit host provisioning.
CONTROL
# Normalize all installed permissions; package code is never writable by services.
find "$root" -type d -exec chmod 0755 {} +
find "$root" -type f -exec chmod 0644 {} +
chmod 0755 "$root/usr/bin/homenode" "$root/usr/lib/homenode/homenode-supervisor" "$root/usr/lib/homenode/homenode-transfer" "$root/usr/lib/homenode/homenode-backup" "$root/usr/lib/homenode/homenode-inspect" "$root/usr/lib/homenode/guest/homenode-guest"
(cd "$root" && find usr -type f ! -name SHA256SUMS -print | LC_ALL=C sort | xargs sha256sum > usr/share/doc/homenode/SHA256SUMS)
archive="$output/homenode_${version}_amd64.deb"
dpkg-deb --root-owner-group --build "$root" "$archive"
sha256sum "$archive" > "$archive.sha256"
# External evidence avoids a package/SBOM hash cycle; inventory is incomplete.
python3 packaging/debian/sbom.py "$root" "$archive" "$version" > "$archive.sbom.json"
go run ./packaging/debian/modules "$root" go.sum > "$archive.gomodules.json"
# Verify downloaded module cache contents before and after source notice reads.
# Metadata/checksums alone do not qualify the files in the source directory.
go mod verify
go list -m -json all > "$staging/go-source-modules.json"
PYTHONDONTWRITEBYTECODE=1 python3 packaging/debian/go_notices.py "$archive.gomodules.json" "$staging/go-source-modules.json" "$staging/go-notices.txt" > "$staging/go-notices.json"
go mod verify
cp "$staging/go-notices.json" "$archive.go-notices.json"
cp "$staging/go-notices.txt" "$archive.go-notices.txt"
cp web/build-evidence/dependencies.json "$archive.frontend.json"
cp web/build-evidence/frontend-notices.txt "$archive.frontend-notices.txt"
(cd "$output" && sha256sum "homenode_${version}_amd64.deb.sbom.json" "homenode_${version}_amd64.deb.gomodules.json" "homenode_${version}_amd64.deb.go-notices.json" "homenode_${version}_amd64.deb.go-notices.txt" "homenode_${version}_amd64.deb.frontend.json" "homenode_${version}_amd64.deb.frontend-notices.txt" > "homenode_${version}_amd64.deb.evidence.sha256")
printf '%s\n' "$archive"
