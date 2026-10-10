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
AI has separate development model-service sources and hash-pinned runtime/model
assembly inputs. Full release license/profile review and actual guest execution
remain pending. The host
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

The overlay also carries the guest-only `data.mount` source for the fixed named
virtio data disk, using ext4 and `nodev,nosuid,noexec`. Adapter mount requirements
pull it into the startup transaction. A blank disk is not formatted by this
unit. Safe initialization, filesystem admission and private object ownership
are still required before image qualification; never install it on the host.

The guest-only `homenode-data-init.service` runs the verified initializer binary
after mount admission and before the adapter. It creates only the private guest
object leaf under a root:root mode 0755 mount root. Existing foreign/unsafe or
incomplete roots are refused, never repaired. The unit and binary are included
in overlay manifests/imports, with ELF checks on both executables. This does not
format data disks; durable incomplete-creation recovery and boot qualification
remain pending. No initializer is installed by ordinary host packaging.

## Ephemeral operating-system writes

The overlay supplies guest-only `tmp.mount` and `var.mount`. Each uses tmpfs with
an explicit 64 MiB byte limit and 8192 inode limit, root ownership and
`nodev,nosuid,noexec`; `/tmp` is mode 1777 and `/var` mode 0755. The adapter
requires both mount units before starting. These files never enter host packaging.
The [kernel tmpfs contract](https://www.kernel.org/doc/html/latest/filesystems/tmpfs.html)
defines independent byte/inode ceilings and volatile contents. This is a 128 MiB
combined ceiling, not reserved memory or proof of a complete guest memory budget.

The unfinished builder must provide mountpoint directories in the immutable
root, enable these units before local filesystem initialization, and populate
required ephemeral `/var` subdirectories using the pinned distribution's tmpfiles
rules. Persistent workload bytes belong only on `/data`; persistent journals,
swap and services depending on durable `/var` are excluded from the appliance
profile. Existing distribution fstab/generator settings must be reconciled so
no competing mount changes these limits. CI verifies source syntax and overlay
integrity; real boot ordering, journal behavior, exhaustion, memory pressure and
shutdown still require assembled-image qualification.

The overlay itself now includes empty `/data`, `/tmp` and `/var` mountpoints,
mode 0755 before mounting. The same fixed directory inventory is used for
staging verification, archive creation and strict import. Missing mountpoints,
symlinks, writable modes or unexpected contents are rejected. The mounted
`/tmp` root gains its sticky mode from the mount options, not from this directory.
These development overlays deliberately reject older inventories without the
mountpoints. A base-root merger must preserve existing distribution contents
beneath `/tmp` and `/var` according to its reviewed assembly policy; this importer
still creates only a fresh overlay and does not merge or edit an existing rootfs.

## Volatile OS directory and journal inputs

The overlay carries `usr/lib/tmpfiles.d/homenode-volatile.conf`, creating only
`/var/tmp` (root:root 1777), `/var/log` and `/var/lib` (root:root 0755).
Initializer and adapter explicitly require and follow the distribution's
`systemd-tmpfiles-setup.service`, which must run after the guest `/var` mount.
These rules neither recurse into workload objects nor remove existing files.
The opt-in Linux fixture runs the actual installed `systemd-tmpfiles` twice
against a fresh temporary root and verifies modes, replay and workload marker
preservation. It never applies the rules to the owner's host.

`usr/lib/systemd/journald.conf.d/60-homenode.conf` selects volatile logs under
`/run`, requests 16 MiB total use, 4 MiB files, four retained files and one-day
retention, and disables syslog/kernel/console/wall forwarding. Rate limiting
requests 1000 messages per 30 seconds; systemd adjusts effective limits according
to free space. Journal retention is not a hard filesystem quota: active files
and rotation can exceed configured targets. See the pinned upstream
[journal contract](https://raw.githubusercontent.com/systemd/systemd/v255/man/journald.conf.xml)
and [tmpfiles contract](https://raw.githubusercontent.com/systemd/systemd/v255/man/tmpfiles.d.xml).
Actual `/run` bounds, default-namespace configuration precedence, rotation under
load and aggregate VM memory measurements still need image qualification.
Guest-local logs disappear at reboot; authoritative owner audit history remains
in the host controller. No native journal daemon or guest boot is exercised by
the directory fixture.

## Runtime admission of OS temporary filesystems

Before opening its workload store, the production adapter also admits the opened
`/tmp` and `/var` filesystem views. Pinned directory descriptors must report root
ownership and exact modes (1777 and 0755), separate tmpfs devices, nodev/nosuid/
noexec flags, positive byte limits no greater than 64 MiB and positive inode
limits no greater than 8192. Kernel mount IDs must resolve to the expected paths
and filesystem type in bounded mountinfo. `/var` must expose the filesystem root;
`/tmp` may expose the adapter's PrivateTmp subdirectory of its bounded tmpfs.
The adapter's `/tmp` must be writable; `/var` may be read-only under its existing
ProtectSystem sandbox. This check changes no mounts or permissions and does not
prove the underlying system disk signature or a complete VM memory envelope.

The opt-in native CI fixture creates an independent private mount namespace,
mounts the two reviewed source definitions there, and executes the real checker.
It then tests unsafe modes, excessive byte/inode limits, missing protective flags
and read-only temporary storage, followed by private-temp/read-only-var views.
All mounts disappear when that child namespace exits. It does not boot a guest or
execute the production adapter systemd unit; actual boot and source-unit startup
qualification remain mandatory.

## Merged-root finalization

`finalize_root.py ROOT VERIFIED_OVERLAY` provides the admission/enablement phase
for exclusively controlled assembly staging. The CLI requires Linux root and
explicit `HOMENODE_GUEST_IMAGE_BUILD=1`; never target an installed host. It checks
the overlay inventory, every merged payload's digest/size/mode/owner, protected
non-symlink parents and the admitted local guest account before enabling mounts
and the selected adapter instance. Accounts must already have been created by
the assembler's sysusers phase. The merged base may contain distribution files;
this helper does not authenticate their source or establish publisher trust.

Enablement creates only three known symlinks below `/etc/systemd/system`, with
no overwrite of foreign entries. Existing matching links support replay; another
profile in the normal `/etc` or `/usr/lib` multi-user target is refused. It is not
a general scan of all possible custom units, generators or startup mechanisms:
the maintained base's complete enabled-unit inventory must also be qualified.
Failed staging is retained. Directory/link creation does not provide an atomic
three-link transaction or independent physical crash evidence.

The BIOS supervisor contract requires an actual GRUB/kernel/initramfs/system
image. The pinned [mkosi v25 build contract](https://github.com/systemd/mkosi/tree/52448a27f6f869c108352ed82fcfb9be633703fb)
provides a candidate BIOS GRUB assembly path; no recipe/build or maintained
release acceptance is established by root finalization. Base packages, tool
security coverage, initramfs content, bootloader and all workload runtimes still
need assembly/provenance and boot qualification.

## Development disk assembly

`build_image.py ARCHIVE SHA256 PROFILE REVISION NEW_OUTPUT MKOSI_CHECKOUT` is an
explicit disposable Linux/root pipeline (`HOMENODE_GUEST_IMAGE_BUILD=1`). It
requires a private root-owned output parent, exclusive new staging and an
unchanged root-owned mkosi checkout at commit
`52448a27f6f869c108352ed82fcfb9be633703fb`. It imports the bounded digest-pinned
overlay, copies only the reviewed recipe/helpers, and runs mkosi summary/build.
Failures retain staging. It does not install host services or write physical
disks, and the normal owner installer never builds these images.

The recipe requests Ubuntu noble x86-64 packages with repository signature
checks, BIOS/EFI GRUB, a distribution kernel/initramfs, a 512 MiB ESP, 1 MiB BIOS
partition and 2–4 GiB ext4 root with the read-only partition flag and `ro` kernel
argument. Root login is locked and autologin disabled. The finalization hook
checks merged adapter bytes/identity and enables its mounts and selected unit.
Distribution tmpfiles may leave the underlying `/tmp` mode 1777; this protected
root-owned sticky directory is accepted alongside the overlay's 0755 mode.
Files and video are assembled, with FFmpeg added for video. AI is explicitly
refused until its reviewed runtime/model assembly exists.

The separate `Development guest images` workflow performs real builds on
throwaway Ubuntu runners and retains unsigned disks and package inventories for
seven days. `development-build.json` binds source/overlay/tool identities and
result bytes, and explicitly records `releaseQualified: false` and
`bootValidated: false`. The source package repositories currently resolve
maintained versions at build time: signature checking and recorded package
inventories do not provide an independently reproducible pinned package closure.
No source-signature promotion, current tool security qualification, complete
license inventory, model/runtime qualification, VM boot/health acceptance or
signed release publication is implied. These remain mandatory release work.

## Development VM boot fixture

`boot_image.py IMAGE DEVELOPMENT_MANIFEST NEW_OUTPUT` requires an unprivileged
Linux user and `HOMENODE_GUEST_BOOT_INTEGRATION=1`. The fixture bounds and verifies
the manifest/image digest, creates only new private fixture staging, and formats
an exclusive 128 MiB test data file. It never mounts guest filesystems on the host.
QEMU runs q35 BIOS/TCG with a read-only system descriptor, separate data descriptor,
512 MiB RAM/one vCPU for files and video, 1536 MiB/two vCPUs for AI,
virtio RNG/serial and no NIC or display. A fixture-private Unix socket carries
bounded typed adapter frames; a separate private QMP socket controls poweroff.

Explicit `HOMENODE_GUEST_BOOT_ACCELERATOR=kvm` selects development KVM boot.
It first opens the fixed character device and checks `KVM_GET_API_VERSION == 12`,
as required by the [kernel API documentation](https://docs.kernel.org/virt/kvm/api.html).
Unknown modes, denied access, foreign device identity and unsupported APIs are
refused; KVM never falls back to TCG. Evidence records the accelerator and separate
TCG/KVM round-trip flags, with `releaseQualified` still false. Both image workflows
attempt this additional boot when the KVM device and group exist, running as
the same non-root runner user with a process-only `kvm` primary group through
`runuser`. They do not change device permissions or account memberships, and
explicitly report missing hardware/group support. These direct development boots do not
qualify the installed supervisor, signed catalog, reserved UID isolation or release.

Before retaining successful boot evidence, the fixture validates its exact fields,
build image identity, accelerator flags, chunk replay, profile-specific results,
cancellation/deletion and guest shutdown/filesystem checks. A software boot
cannot emit a KVM success flag; incomplete profile evidence and release claims
are refused. These checks validate development reports, not independently signed
attestations or production activation authority.

Within a bounded boot deadline the fixture requires adapter health and a small
upload/finalize/download/content-verification/delete-ack round trip. It retains
at most 8 MiB of console diagnostics and writes separate boot evidence on success;
it does not edit the build manifest's unqualified flags. The CI workflow retains
console/evidence even after a failed boot, with no fixture data disk upload.
Success now also requires ACPI powerdown, QMP guest-initiated shutdown evidence,
zero QEMU exit and a successful read-only e2fsck of the disposable data disk.
Failure termination is teardown of test-only state and cannot count as shutdown
evidence. These checks do not establish full backup consistency, physical
power-loss durability or production-host shutdown qualification.

Inspected assembled files/video boot and shutdown results are recorded in
docs/PROGRESS.md. AI shutdown and production-host lifecycle qualification
still require separate evidence. TCG and a tiny test disk also
do not prove KVM/AppArmor confinement, production disk quotas, video conversion,
model loading, memory/thermal envelopes, client networking or full host lifecycle.
The real image build and boot results must be inspected before those claims.

The development recipe explicitly includes Ubuntu's universe component. Actual
Linux build logs showed that mkosi's default initramfs package set requires
`erofs-utils`, which was unavailable with main alone; video also requires FFmpeg.
Repository signature checking remains enabled. Availability in a signed archive
is not proof of timely vulnerability fixes or maintenance coverage for every
package: the release qualification still needs a reviewed support/patch policy
and pinned package/license inventory for both system and initramfs closures.

The image runner additionally requires `mtools`: actual ESP population failed
without its `mcopy` binary, after successful ext4/initramfs generation. This is a
host build dependency, not a new guest service. The template has no default
instance; otherwise distribution `preset-all` enabled the files profile inside
the video image. The profile conflict gate refused that image before publication.
Only root finalization enables the selected instance. The recipe uses mkosi's
conventional `mkosi.finalize` and `mkosi.repart` discovery rather than listing
them a second time, avoiding duplicated finalize invocations and definitions.

Actual video assembly reached EFI population after the explicit-profile fix,
but `mcopy` reported disk full with the original 256 MiB ESP. The recipe now
uses the pinned upstream default's 512 MiB ESP size. This remains within the
system-image size admission limit. Successful formatting, BIOS installation
and boot must still be demonstrated; a larger partition alone is not evidence
of a complete image or sufficient space for every future kernel closure.

Linux run `36732215628` built both files/video disks and executed each boot
fixture successfully. Retained evidence binds files SHA256
`ef6d79e38b461978cae8e06c0376a5e2251c498b7f7a37be7a0341f7f0f5455f`
and video SHA256
`ca385aa0923d4c246c3e8da58351f50959c2f11a074930165ac56c1b4c566680`
to TCG health/object round trips. This establishes that narrow boot/startup/data
workflow for these development images, not their release qualification.

Run `36735256003` additionally uploaded a generated one-frame 1080p source,
executes both fixed presets inside the booted adapter, waits for durable success,
retrieves/chunk-hashes the bounded outputs and checks H.264 codec and expected
720p/1080p dimensions. Only the disposable unprivileged CI child probes those
test outputs, with address-space/CPU/core limits; production host services never
run FFmpeg/ffprobe on guest video bytes. Both presets passed. The current fixture
uses 30 generated frames, adds immediate cancellation/deletion followed by
conversion, and checks a multi-chunk object transfer with acknowledged replay.
Those extensions await Linux results. Audio, arbitrary codecs, already-running
subprocess cancellation, reboot recovery, quality/performance and the complete
controller/supervisor/UI lifecycle remain separate gates.

## Development CPU inference runtime

`build_ai_runtime.py` requires explicit `HOMENODE_AI_RUNTIME_BUILD=1` on an
unprivileged disposable Linux x86-64 builder. It admits clean source revision
`7fe450e19305b828c199d602c23a8337aaa1f03b` (the peeled v0.5.0 tag), configures
and builds only the server target with two compiler workers, then records the
ELF binary digest, size and build options. Its dedicated workflow retains an
unsigned candidate and upstream LICENSE for seven days. No host service is
installed and no model is fetched or included.

The candidate disables host-native/AVX tuning, GPU/RPC backends, dynamic ggml
backend loading, server subprocess support, OpenSSL and automatic prebuilt UI
downloads. Static project libraries do not make the entire executable statically
linked; distribution runtime dependencies still need inventory and guest-image
integration. These build options do not remove every server endpoint or provide
network isolation. The eventual guest must enforce loopback binding, no NIC,
restricted local adapter access and reviewed runtime arguments.

Actual Linux compile results, model source/digest/license admission, complete
third-party notices/SBOM, vulnerability maintenance, model-server source unit,
booted inference/cancellation and measured memory/context/CPU profiles remain
unfinished. No model SKU is selected as the production default by this build.

### Guest ACPI shutdown preparation

Development image assembly includes the system D-Bus package and explicitly
starts systemd-logind. The admitted adapter overlay contains a guest-only logind
drop-in selecting poweroff for the power key and ignoring suspend, hibernate,
lid and idle actions. These paths belong inside the image; do not copy this
policy onto the owner host. The drop-in is included in the protected overlay
inventory and root finalization preserves conflicting enablement instead of
overwriting it.

This prepares ACPI handling for the bounded cooperative shutdown primitive.
It does not qualify clean shutdown: an assembled image must demonstrate ACPI
handling, adapter/model termination, data.mount unmount and domain exit under
idle/load/failure conditions. Inhibitor handling and the added package/service
closure need security review and release inventory qualification. Domain exit
alone does not authorize copying a data disk. Supervisor maintenance coordination,
independent disk-copy checks and guaranteed restart remain required.
