# Guest image service inputs

These sources belong inside the immutable guest image, not in host systemd or
udev directories. The maintained image builder is still pending. No command in
this directory installs services, formats disks or modifies the owner's host.

The builder must create the locked nologin `homenode-guest` account, install the
reviewed binary at `/usr/lib/homenode/guest/homenode-guest`, and install the unit
and udev rule in guest system directories. Profile environment files go to
`/etc/homenode/guest/` as root-owned, non-writable files. Enable exactly one of
`homenode-guest@files.service`, `@video.service` or `@ai.service` in its respective
image. Profiles match current catalog minimum data sizes and can only be changed
by a reviewed signed image release.

The separate data disk must be initialized and mounted at `/data` before service
startup and expose a guest-user-owned private object directory. The mounted-disk
condition prevents silently using an ordinary root-filesystem directory. It does
not prove the correct disk identity or perform safe initialization; those checks
belong to the unfinished builder/boot initialization contract. The virtio port
rule restricts adapter access to the dedicated guest account. Validate the rule
against the booted guest kernel/device and prove access before release.

The service has no capabilities and requires writable data plus the fixed virtio
port. Files/video images must provide their reviewed runtime dependencies. The
AI image also needs a separate reviewed, licensed, hash-pinned CPU model server
bound to loopback; that model unit/artifact pipeline remains pending. The host
VM's absent NIC is the primary network boundary. RestrictAddressFamilies inside
the guest is not an egress-policy certificate.

Linux CI validates unit syntax after package installation supplies the guest
binary. Static validation does not start the unit, prove mount/udev ownership,
verify guest confinement, boot a VM or qualify shutdown timing. See
`docs/GUEST_STARTUP.md` for protocol readiness and actual acceptance gates.

The rule uses the port name exported by the
[Linux virtio-console driver](https://github.com/torvalds/linux/blob/master/drivers/char/virtio_console.c)
and explicitly creates the fixed device symlink. Ownership/mode keys follow the
[systemd udev rule contract](https://cgit.freedesktop.org/systemd/systemd/tree/man/udev.xml).
These upstream contracts guide the rule; they do not replace a booted-device
permission check on the pinned guest kernel and udev build.
