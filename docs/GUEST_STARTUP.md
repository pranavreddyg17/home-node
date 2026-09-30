# Guest startup and readiness

The immutable guest release must provide a bootloader/kernel, read-only system
root, writable ephemeral OS directories, a separately mounted writable data
volume and `/dev/virtio-ports/org.homenode.adapter`. The current host domain
uses a read-only raw system disk and a separate raw data disk; the guest agent
alone does not format, mount or qualify those disks. A maintained guest image
builder, qualified bootable release artifacts and actual boot/VM isolation
acceptance are still pending.

`homenode-guest` uses a fixed workload profile and opens its object directory
before serving the constrained virtio-serial protocol. Files/video currently
report agent readiness; those responses are not a boot/image qualification
certificate or proof that FFmpeg/media capability tests have passed.

For AI, the agent now requires the model server's fixed loopback
`GET http://127.0.0.1:8080/health` endpoint to return HTTP 200 and canonical
`status: ok` before reporting `ready`. Loading, connection errors, redirects,
malformed/trailing/duplicate JSON, unexpected fields and responses exceeding
4 KiB report `starting`; model/server error text is not sent to the controller.
The probe has a two-second deadline, no proxy and a separate connection pool
from generation, so a generation stream cannot hold the probe's only connection.
Optional nonnegative `slots_idle`/`slots_processing` fields are accepted for the
compatible server health contract. A pinned guest release must test that
contract against its actual model server version.

The contract follows the upstream [llama.cpp server health endpoint](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md#get-health-returns-health-check-result).
The image must run the reviewed model server on loopback only, with tools and
external integrations disabled and an independently licensed/hash-pinned CPU
model. These probes do not prove model licensing, model identity, acceptable
inference latency or resource isolation; release qualification must prove those.
The controller's existing bounded app-start loop waits for guest readiness and
reports failure if it never arrives.

## Agent shutdown

The guest process handles SIGINT/SIGTERM by closing its active virtio channel
and interrupting reconnect waits. Once the serving loop exits, the agent rejects
new requests, cancels workers and waits for their final journal writes before
syncing/closing its data root. Interrupted tasks persist as interrupted and are
not automatically executed on reopen. Repeated agent close is idempotent;
late model probes cannot advertise ready after shutdown. Journal write failures
are retained as shutdown errors instead of silently claiming successful closure.
The existing FFmpeg process cancellation and bounded wait remain in effect.

Tests use real task journals and blocked framed-channel substitutes. Actual
virtio device opening/closing and systemd shutdown timing still require booted
VM acceptance. An interrupted agent is not a cleanly powered-off VM: the current
host ordinary stop still destroys the domain. This work does not provide the
maintenance drain lease, guest quiesce acknowledgement, filesystem unmount or
hypervisor power-off evidence required for a consistent backup.

## Task recovery admission

Startup enumerates the object directory in bounded batches and admits at most
1,000 task records. Journals must be private mode 0600 regular files owned by
the guest service UID; final symlinks and special files are rejected with
no-follow/nonblocking descriptors. Reads are bounded before decoding to 256 KiB,
which accommodates the existing 32 KiB text limit even when JSON escaping
expands it. The writer enforces the same journal bound.

Canonical fields, unique keys, no trailing JSON, recognized states, profile
identities, bounded text/size and lowercase hashes are required. Files guests
cannot recover executable tasks. Video success requires bounded nonempty output
metadata; AI records cannot carry video fields. The complete task set is
validated before any running intent is rewritten as interrupted. Invalid
recovery data fails startup and is preserved for diagnosis. This is application
journal admission; disk corruption recovery, filesystem mount/init and booted
image qualification remain separate requirements.

## Image service sources

`packaging/guest/` now contains a guest-only service template, fixed profile
quotas and a named virtio-port access rule. The template requires `/data` to be a
mountpoint, uses a dedicated nologin guest account with no capabilities, restricts
writes to data and closes the worker process group on timeout. Builder-installed
account, mount ownership and runtime dependencies are mandatory prerequisites;
these sources are not installed or enabled on the owner's host by packaging.
Static unit validation is included in Linux CI after the development guest
binary is installed. Actual boot, user/device access and confinement remain
unverified until maintained images are built and exercised.

## Named port startup ordering

The adapter unit now orders startup after and binds its lifetime to the fixed
virtio-port device unit. The matching guest-only udev rule tags the port for
systemd and sets its fixed absolute device alias, in addition to its private
ownership and symlink. The service no longer relies on an ordering-only edge
to a global udev settle service: a port that appears later must satisfy the
actual device dependency before service conditions are evaluated. Device loss
stops the adapter. The enabled profile is started on normal guest boot; automatic
restart after device removal/reappearance is not claimed.

This follows the upstream [systemd device-unit tag/alias contract](https://github.com/systemd/systemd/blob/main/man/systemd.device.xml) and unit dependencies. Static unit validation and path escaping are checked in Linux CI;
real virtio enumeration, delayed arrival, disappearance and shutdown behavior
still require booted-image qualification.

## Guest disk role identifiers

The fixed supervisor XML exposes serial `homenode-system` on the read-only
system disk and `homenode-data` on the separate writable data disk. These are
role identifiers within one isolated VM, not globally unique volume IDs or
publisher authentication. Future guest initialization must verify the named
virtio device, actual block identity, expected size, read-only/writeable role
and filesystem state before mounting or initializing data. Positions `vda`/`vdb`
alone do not establish those checks. No formatter or mount action is enabled
by this change; existing disk bytes are untouched.

Tests parse generated XML to bind both serials to their expected source paths,
targets and access modes. Linux CI additionally uses the installed libvirt
schema. Actual kernel serial/by-id discovery and safe blank-disk initialization
remain booted-VM acceptance requirements.

## Guest data mount source

The verified adapter overlay now includes `data.mount`, selected by the adapter's
existing `RequiresMountsFor=/data` dependency. It mounts only the fixed
`/dev/disk/by-id/virtio-homenode-data` as ext4 at `/data` with `nodev,nosuid,noexec`.
A missing named disk or unmountable filesystem prevents adapter startup. The
unit belongs inside guest images; host packaging does not install it.

This does not initialize blank raw disks, repair filesystems, admit expected
block-device size/state or create the private guest-owned object directory.
Those prerequisites remain mandatory before useful startup. The overlay digest
inventory, deterministic packaging and fixed-member importer include this unit.
Actual mount options, disk role/by-id discovery, initialization safety and
power-loss recovery still require booted guest qualification.

## Agent object-root admission

Before recovering journals or serving requests, the agent requires its object
root to be a mode 0700 directory owned by its effective UID. A final symlink,
foreign owner or nonprivate permissions fail startup; admission does not chmod,
chown or remove existing data. It may create the requested leaf at mode 0700,
but does not create missing parents. The opened root is checked against the
observed directory identity before it is used.

This requires the unfinished guest boot initializer to provide an appropriate
parent and object directory after mounting the correct disk. It does not verify
parent ancestry, mount provenance, filesystem type, available capacity or
physical disk integrity. Those remain separate boot admission requirements.

## Binary data-mount admission

The production guest entrypoint now admits only `/data/objects`. Before opening
the agent it checks the named data path is a block device, kernel serial is
`homenode-data`, kernel read-only flag is zero and the current mount namespace
contains the `/data` mount actually opened by the process, of the same major/minor device. The mount
must expose the whole ext4 filesystem with rw,nodev,nosuid,noexec and writable
superblock options. Kernel mountinfo/attribute reads are bounded; non-Linux
startup fails admission. The service still denies raw block-device access: the
checker reads kernel metadata without opening the disk for I/O.

Parser tests cover missing/duplicate mounts, another disk, subtree/bind roots,
wrong filesystem, absent confinement flags, readonly superblocks and malformed
or oversized evidence. Linux cross compilation passed. Real kernel/udev mount
identity, safe formatting and post-crash filesystem behavior remain unverified
until booted-image acceptance. This check does not initialize or repair storage.

Mount admission pins `/data` with a no-follow directory descriptor, checks its
actual filesystem device and selects mountinfo by the kernel `mnt_id` in bounded
`/proc/self/fdinfo`. Stacked entries from service namespace protections can
share a path; hidden mount entries cannot establish admission for the visible
mount. A missing/duplicate selected ID or unsafe visible overmount fails. Parser
tests cover a valid visible mount above a hidden entry and an unsafe visible
mount above a valid hidden entry. Real systemd namespace/boot acceptance remains
required; these parser tests do not prove that runtime behavior.

A Linux-only descriptor fixture opens a real temporary directory and compares
the production fdinfo reader's mount ID with the current kernel mountinfo
record and descriptor filesystem device. It rejects invalid descriptors. This
is native descriptor evidence only: it does not emulate a virtio disk, establish
the `/data` contract, exercise systemd protections or prove boot admission.

## Guest object directory initializer

A separate guest-only root oneshot now runs after the admitted data mount and
before the unprivileged adapter. It calls the same mount admission, requires a
root:root mode 0755 mount root, creates only the `objects` leaf privately, sets
its fixed UID/GID 900 and syncs the directory and parent. Existing objects roots
must already be UID/GID 900 and private mode 0700; symlinks, foreign ownership,
unsafe modes and incomplete root-owned directories are refused without repair
or data removal. It has CAP_CHOWN for new directory ownership and CAP_DAC_READ_SEARCH to inspect existing private guest directories, with no raw-device access. The adapter
requires successful initialization.

The helper and unit are now part of verified guest overlays; both guest
executables require ELF x86-64 admission. Host packaging does not install or
enable the initializer. CI installs its binary only in the disposable runner
for static unit verification and uses opt-in temporary-root ownership fixtures.

This prepares a directory on an already mounted filesystem. Blank-volume
formatting is still absent. A durable root-owned intent is now synced before directory creation. A crash
between creation and ownership publication can resume only an empty root-owned
private leaf covered by that admitted intent. Corrupt active intent data and nonempty incomplete directories are preserved
and refused. Intent publication now uses synced private staging and Linux
rename-without-replacement, so an interrupted staging write is not an active
intent. Physical power-loss qualification remains required before release.

Initializer intent reads use bounded no-follow/nonblocking regular-file
descriptors, exact root ownership/mode and single-link admission. The intent
file and parent are synced before creating the object leaf. Without admitted
intent, existing root-owned leaves cannot be adopted. An admitted already
guest-owned private object root can reopen without recursively changing data.
This depends on exclusive trusted startup and the root-controlled mount parent;
it does not authorize arbitrary directory repair or offline disk formatting.

Linux CI also repeats initializer fixtures under an observed two-capability
profile (CHOWN and DAC_READ_SEARCH), no inherited/ambient capabilities and
no-new-privileges. It verifies the actual process sets before directory tests.
This checks initializer operations under the requested capabilities; actual
systemd mount namespace, device policy and full guest boot remain separate gates.

Before directory creation, complete intent bytes are written and synced in a
new private staging file, then published using Linux RENAME_NOREPLACE and the
parent is synced. Existing active intent is never overwritten. Interrupted
staging files are retained unchanged; a retry can publish a fresh complete
record. Startup refuses after 64 retained staging records or more than 1,024
mount-root entries, with enumeration in batches of 128. Retained metadata
cleanup/diagnostics and physical storage fault testing remain release work.

A separate opt-in disposable Linux CI fixture runs the compiled initializer
tests through systemd using the guest initializer source's reviewed service
properties. Tests create their temporary data under the fixture's exclusively
created ordinary `/data` directory, verify capability evidence and check that
PrivateTmp hides a host-side marker. The existing `/data` path is never adopted
or removed; fixture cleanup removes only its own empty directory.

This executes filesystem/privilege operations under unit protections, with a
test executable replacing ExecStart and RemainAfterExit disabled so the fixture
can terminate. Unit mount dependencies/conditions are not replicated; actual
virtio data mounts, production entrypoint admission and guest boot remain
unverified. No fixture is run on the owner's machine by default.
