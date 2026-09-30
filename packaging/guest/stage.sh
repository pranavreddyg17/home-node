#!/bin/sh
set -eu
profile=${1:-}
output=${2:-}
case "$profile" in files|video|ai) ;; *) echo 'Expected files, video or ai profile' >&2; exit 2 ;; esac
case "$output" in /*) ;; *) echo 'Output must be an absolute new directory' >&2; exit 2 ;; esac
[ "$(uname -s)" = Linux ] || { echo 'Use a Linux release build runner' >&2; exit 2; }
[ ! -e "$output" ] && [ ! -L "$output" ] || { echo 'Output already exists' >&2; exit 2; }
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo"
[ -z "$(git status --porcelain)" ] || { echo 'Build from a clean committed checkout' >&2; exit 2; }
revision=$(git rev-parse HEAD)
toolchain=$(go env GOVERSION)
expected=$(awk '$1 == "toolchain" { print $2 }' go.mod)
[ "$toolchain" = "$expected" ] || { echo 'Use the committed Go toolchain' >&2; exit 2; }
staging=$(mktemp -d)
trap 'rmdir "$staging" 2>/dev/null || :' EXIT HUP INT TERM
root="$staging/root"
mkdir -p "$root/usr/lib/homenode/guest" "$root/usr/lib/systemd/system" "$root/usr/lib/udev/rules.d" "$root/usr/lib/sysusers.d" "$root/etc/homenode/guest"
GOENV=off GOFLAGS= GOWORK=off GOEXPERIMENT= GOTOOLCHAIN=local GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=true -o "$root/usr/lib/homenode/guest/homenode-guest" ./cmd/homenode-guest
GOENV=off GOFLAGS= GOWORK=off GOEXPERIMENT= GOTOOLCHAIN=local GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=true -o "$root/usr/lib/homenode/guest/homenode-guest-init" ./cmd/homenode-guest-init
cp packaging/guest/homenode-data-init.service "$root/usr/lib/systemd/system/"
cp packaging/guest/data.mount "$root/usr/lib/systemd/system/"
cp packaging/guest/homenode-guest@.service "$root/usr/lib/systemd/system/"
cp packaging/guest/60-homenode-adapter.rules "$root/usr/lib/udev/rules.d/"
cp packaging/guest/homenode-guest.conf "$root/usr/lib/sysusers.d/"
cp "packaging/guest/$profile.env" "$root/etc/homenode/guest/$profile.env"
find "$root" -type d -exec chmod 0755 {} +
find "$root" -type f -exec chmod 0644 {} +
chmod 0755 "$root/usr/lib/homenode/guest/homenode-guest" "$root/usr/lib/homenode/guest/homenode-guest-init"
python3 packaging/guest/overlay.py create "$root" "$profile" "$revision" "$toolchain"
python3 packaging/guest/overlay.py verify "$root"
mv -T -n "$root" "$output"
[ ! -e "$root" ] || { echo 'Output appeared during publication' >&2; exit 1; }
printf '%s\n' "$output"
