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
