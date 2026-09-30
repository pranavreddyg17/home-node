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

## Adapter overlay staging

From a clean committed checkout on a Linux build runner using the committed Go
toolchain, run `sh packaging/guest/stage.sh files /absolute/new/output` (or select
`video`/`ai`). It builds the static Linux/amd64 adapter, stages only the selected
profile, unit/udev/sysusers sources and emits `overlay.json` with source revision,
toolchain, numeric guest identity and streamed payload hashes. Output publication
refuses an occupied path. Failed private staging is preserved; successful empty
staging is removed without recursive cleanup. Normal owner installation must
consume release artifacts, not build on the laptop.

`python3 packaging/guest/overlay.py verify /path/to/overlay` checks exact regular
file/directory inventory, modes, sizes/hashes, ELF architecture and quota. This
verifies integrity against metadata, not metadata authenticity: the release
pipeline still needs independent trust/signing and provenance. Metadata explicitly
states `bootable: false`. Root filesystem/kernel/bootloader assembly, numeric
identity collision checks, safe volume initialization, model/runtime packaging,
actual boot tests and signed catalog publication are not produced by staging.
The sysusers source fixes guest UID/GID 900; assembly must verify uniqueness and
exact nologin identity before releasing an image, never accept allocator fallback.
Linux CI stages all three overlays and runs portable mutation/refusal tests.

## Assembly archives

After staging, `python3 packaging/guest/overlay.py package /path/to/overlay
/absolute/new/output.tar SOURCE_TIMESTAMP` produces a USTAR assembly archive.
Use the source commit's Unix timestamp. Paths/modes are fixed, owner/group fields
are root, metadata is canonical, and each copied file stream must match its
verified size/hash before publication. The archive is synced and published with
no overwrite, including refusal of occupied symlink destinations. Output mode is
0444. Input verification runs again before publication.

CI packages each real compiled overlay twice, compares complete archive bytes
and retains only the original three archives as unsigned development artifacts
for 14 days after all checks pass. This is repeatability of packaging the same
overlay, not independent reproducibility of compiler output or filesystem images.
The archives contain `bootable: false` metadata and provide no publisher signature
or release qualification. Do not extract an arbitrary downloaded archive as root;
a trusted assembler with explicit member/schema/digest admission is still pending.

## Digest-pinned assembly import

`python3 packaging/guest/import_overlay.py ARCHIVE NEW_STAGING EXPECTED_SHA256
PROFILE EXPECTED_SOURCE_REVISION` snapshots and verifies the bounded archive
before admitting any members. Obtain the digest/revision from trusted build
records rather than accepting values supplied with an unknown archive. The new
staging directory's parent must be protected, owned by the builder and selected
with an absolute path. Existing output paths are never overwritten.

Only the exact selected-profile file/directory set, canonical modes and root
ownership headers are admitted. Links, special files, duplicate/missing/extra
members and path escapes are rejected. Files are created exclusively without
generic archive extraction and then verified against metadata/content/ELF/quota.
The expected source revision must match. Staging remains private mode 0700 on
failure and is made mode 0755 only after complete verification and sync; preserve
failed staging for diagnosis. CI imports each real built archive into fresh
staging and re-verifies it. Import does not install host files, create guest
accounts, format disks, build bootable images or establish publisher signing
trust. Safe rootfs/boot/data initialization and actual qualification are pending.

## Guest account admission

After applying sysusers inside the assembly root, run
`python3 packaging/guest/identity.py /path/to/image-root`. The checker reads bounded
protected local account/NSS files by descriptors, rejects symlinks and verifies
the exact locked nologin guest identity at UID/GID 900, private group and absence
of numeric aliases, foreign primary members or supplementary privileges. Local
`files` (optionally followed by `systemd`) identity sources are mandatory.
Admission errors are generic and never print password/shadow contents.

The [systemd sysusers format](https://manpages.debian.org/testing/systemd/sysusers.d.5.en.html)
supports explicit user/group IDs, but the requested declaration is not accepted
as evidence of the observed identity. The native disposable fixture creates the
identity with systemd-sysusers in an isolated temporary root and verifies a
conflicting UID/GID cannot pass admission. CI must prove this; the check is not
a boot, actual device access, data-disk ownership or base-image signature gate.
No account-check command edits or repairs account databases. Only the explicit
native fixture performs creation, restricted to its temporary root.

Account admission also requires the opened image root and each account parent
directory to be owned by the checker UID and not writable by group/others.
Reads are restricted to the four fixed account/NSS paths; unexpected paths and
intermediate symlinks are rejected. Run admission against assembler-controlled
staging with no concurrent writers. Descriptor pinning and permissions do not
make a mutable staging tree an immutable release artifact.

The sysusers source explicitly declares the private group as well as the user.
An explicit numeric primary GID requires an existing group; requesting both
numbers in the user line alone does not create that prerequisite. The native
fixture must pass for the pinned assembler systemd version before image release.
