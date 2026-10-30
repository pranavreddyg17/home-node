# Native HTTPS video workflow qualification

Status: image admission, two-guest catalog/policy staging and initial browser
job orchestration implemented; combined build/workflow execution is unfinished. No complete native video
workflow has passed. Existing guest codec tests and controller component tests
do not qualify the combined path.

The disposable root wrapper accepts paired Files/video image manifests only
under explicit video, Files HTTPS and browser opt-ins with bounded 32 KiB Files
setup. Both manifests must validate before either identity is published. Source
and builder revisions must match, image hashes must differ, and profile-specific
overlay hashes remain independently recorded. Each image is subsequently copied
through a retained source descriptor, SHA-256 checked, protected and synced by
the supervisor fixture. These unsigned images and ephemeral catalog signatures
are development inputs, not publisher release artifacts.

The video-specific fixture policy admits two instances, two vCPUs and 2048 MiB
with two distinct reserved guest UID slots. Both images enter the fixture-signed
catalog and protected-storage inventory. Files-only cases retain their existing
single-instance policy. Cleanup must inspect every admitted allocation and
observe exit before discarding storage; a third unexpected allocation fails.

Remaining implementation/qualification: consume paired build artifacts in a
reproducible workflow; demonstrate enrollment, upload, approved conversion and
verified output through the new scenario; exercise cancellation/retry/reconnect;
require explicit job/cleanup markers and read-only journal assertions. Exercise
failure and foreign-guest/host access paths with both guests present. Physical
networking, signed installation and full release gates remain separate.

The initial browser scenario now requests jobs capability only for explicit video
mode. It generates two seconds of bounded test media as the unprivileged browser
account through a fixed, parent-admitted FFmpeg binary, uploads via Files, selects
an approved 720p preset in Jobs, waits for success, downloads and decodes the
output and checks its published checksum. The parent requires an additional
video completion marker. This is implemented syntax/compilation coverage, not
native execution evidence.

Review found an expected lifecycle blocker: `Manager.purge` rejects reserved
UID/volume/channel ownership evidence, while job cleanup still calls that legacy
operation. The scenario requires the cleanup-pending UI state to clear before
emitting its completion marker. It cannot pass solely because conversion and
download succeed. The fix must add inode-qualified, journaled reserved-video
retirement and safe UID release; removing the existing refusal is unacceptable.
The complete video workflow remains unqualified until this and the other gates
are implemented and demonstrated.

The two-guest fixture budget includes the supervisor's required 512 MiB emulator
reserve for each guest: two 512 MiB image profiles therefore require 2048 MiB,
not 1536 MiB. The admission formula is unchanged; the explicit fixture policy
now supplies the full required budget instead of bypassing capacity checks.

Reserved video retirement now has a read-only provenance snapshot. It joins the
stopped video record, immutable lease/pool/group identity, volume inode and size,
channel directory and bounded socket history in one transaction. It rejects
foreign workloads, active/desired-running state, stale request revisions,
maintenance, missing records, mismatched identities/inodes and more than 256
socket records. Errors return no partial receipt. Tests assert that reading a
snapshot creates no runtime operation and changes no instance state.

This snapshot grants no unlink, ownership-change, restart or UID-release
authority. The live stopped/process barrier, descriptor-qualified deletion,
interruption journal and final lease retirement are still required before the
legacy reserved-resource purge refusal can be replaced.

Retirement preparation now atomically records a bounded canonical intent and its
pending operation identity before any physical effect. Exact replay must match
the original request, ownership snapshot and a recognized pending stage; a
changed request/inode, orphan operation or missing/unknown stage refuses.
Preparation rechecks a caller-supplied live stopped guard and rereads provenance
before returning. Losing that guard retains only the pending journal and returns
no receipt. Tests inject a stage-write failure and require rollback of both the
intent and operation. Preparation preserves the runtime state and UID lease.

This journal is not yet routed from reserved purge and grants no filesystem
mutation authority. Descriptor-qualified socket/volume/directory retirement,
resumption after each physical interruption and final checked UID release remain
unfinished. The legacy refusal remains in place until those steps are proved.

Pending checkpoint advancement now authenticates the original canonical intent,
current stopped provenance, pending operation hash and exact prior stage in one
transaction. Only the next forward edge is allowed; an exact retry can observe
its already-committed target. Neither advancing nor reading repairs missing
records. The live guard is rechecked after the update, followed by fresh journal
validation; a lost guard returns no receipt while preserving the resumable phase.
Tests cover forward/replay, skipped/backward transitions, changed ownership,
missing operation, corrupted intent/stage, update failure and interruption after
commit. Checkpoints cannot publish completion or release the reserved UID.
Physical mutation and final retirement are still not wired to the purge path.

An inode-qualified Linux socket-retirement scope is now implemented. It pins the
root-owned channel parent, recorded guest-owned directory and exact socket with
no-follow descriptors, verifies ownership/modes/mount identity and rejects extra
entries, hardlinks, replacements and quarantine collisions. It checkpoints
before a no-replace quarantine rename, syncs, unlinks only the recorded name,
syncs again and checks retained-inode link count and absence before advancing.
Interrupted checkpoint/rename/unlink cases retain the UID and journal; a socket
already absent is reconciled only under its pending removal checkpoint.

The scope requires `fs.protected_hardlinks=1` throughout its checks, in addition
to a live stopped/UID-process exclusion supplied by the caller. The
[kernel hardlink policy documentation](https://docs.kernel.org/admin-guide/sysctl/fs.html#protected-hardlinks)
and [kernel link admission implementation](https://github.com/torvalds/linux/blob/master/fs/namei.c)
describe the non-owner restrictions for special-file hardlinks. Host root and
configuration remain trusted; no claim is made against malicious root changes.

A separate disposable Ubuntu component workflow exercises real socket inodes,
interruption replay, descriptor closure, foreign entries/replacements/symlinks,
hardlinks and collision preservation. Its explicitly opted-in negative case
temporarily disables the hardlink control, requires refusal without effects and
restores/verifies the original value. This never runs on the owner Mac. The
component injects its stopped guard and does not qualify actual KVM/process
exclusion, volume/directory retirement, UID release or the full video workflow.
The scope is not yet wired into reserved purge. Native component execution passed at `19b735a`; live KVM/process exclusion remains unqualified.

The [disposable Linux component run](https://github.com/pranavreddyg17/home-node/actions/runs/40074693919)
at `19b735a` passed all ten filesystem cases, including interruption at the
checkpoint, rename and unlink boundaries, descriptor closure, foreign-resource
refusal and disabled-hardlink-policy refusal/restoration. [Retained evidence](evidence/video-socket-retirement-19b735a.json)
records exact source, kernel, compiler, log hash and cases. Its injected stopped
guard and retained UID lease make this socket component evidence only.

A separate volume-retirement scope now retains the recorded empty channel and
pins the raw volume with an `O_PATH` descriptor. It authenticates the immutable
intent, inode, size, owner, mode, single link and mount identity before a durable
`volume-removing` checkpoint, no-replace quarantine rename and unlink. Each
boundary rechecks the empty channel, stopped guard and provenance. Interrupted
removal can resume only from its recorded checkpoint; successful replay retains
the channel directory, ownership metadata and UID lease. No guest data is read.

The Linux component fixture uses an opaque synthetic raw file and an injected
stopped guard. It exercises replay, descriptor closure, interruption boundaries,
replacement/collision/symlink/hardlink refusal, changed mode/size, recreated
socket and substituted parent. Local Linux cross-compilation and the full supervisor race suite passed.
[Disposable Linux execution](https://github.com/pranavreddyg17/home-node/actions/runs/40077382129)
at `4747795` passed all twelve volume cases and ten socket regression cases;
[retained evidence](evidence/video-volume-retirement-4747795.json) records source,
environment, cases and log hash. This is not a formatted
guest disk, live process-exclusion, directory-retirement or UID-release proof,
and the reserved purge path continues to refuse destructive cleanup.

The directory-retirement scope now pins the channel parent and recorded empty
directory, checks exact ownership/mode/inode/mount identity and requires both
volume names to remain absent. A durable `directory-removing` checkpoint precedes
a no-replace quarantine rename and `rmdir`, each followed by parent sync and
retained-descriptor checks. Replay accepts absence only under the recorded
removal checkpoint. It retains all provenance and the UID lease. Native tests
cover descriptor closure, each interruption boundary and refusal of replacement,
collision, symlink, changed mode, foreign entry, recreated volume, substituted
parent and unrecorded absence. [Disposable Linux execution](https://github.com/pranavreddyg17/home-node/actions/runs/40077650562)
at `f45e729` passed twelve directory cases plus all twenty-two socket/volume
regression cases. [Retained evidence](evidence/video-directory-retirement-f45e729.json)
records source, environment, cases and log hash. This does
not establish live process exclusion or safe UID reuse; reserved purge remains
disabled.

The start-exclusion wrapper now retains the manager start lock through a trusted
retirement callback and its descriptor closure, with repeated backend absence,
real procfs reserved-UID conflict and immutable ownership checks. Its caller must
already hold shared runtime admission. It refuses active stops and shutdown.
This is not wired to purge yet. The disposable component test injects backend
absence but uses real procfs, a short-lived reserved-UID sleep process and the
actual start mutex to test process refusal, lock retention and error release.
[Native execution](https://github.com/pranavreddyg17/home-node/actions/runs/40077917375)
at `3e1d58e` passed all four exclusion cases plus thirty-four filesystem
regression cases; [retained evidence](evidence/video-start-exclusion-3e1d58e.json)
records source and log hash. A procfs scan is an observation, not a proof against
an escaped process racing fork; safe UID reuse still requires separate verified
exit and effective filesystem confinement. No UID-release authority is added.

An internal completion transaction now authenticates the exact canonical intent,
`directory-removed` checkpoint and pending operation before atomically retiring
socket/channel/volume/group provenance, releasing the UID lease, updating the
runtime revision and recording completion. Any failure rolls back the entire
transaction. Completed replay verifies the immutable receipt and removed runtime
without invoking physical checks against a UID that may now belong to a new
instance. It rejects altered operation hashes, runtime revisions and receipts.
The receipt remains retained; historical metadata collection is a separate gate.

This transaction requires a trusted qualified-absence callback after descriptor
closure and under retained runtime/start exclusion. SQL-only tests inject that
callback and synthetic checkpoints; they prove atomicity and replay binding,
not actual process exit, disk absence or safe UID reuse. No production purge path
calls the transaction yet. The [Linux regression job](https://github.com/pranavreddyg17/home-node/actions/runs/40078186274)
at `1f5a003` passed both its retirement race step and native component step.
Local focused and full supervisor race suites and Linux cross-compilation also
passed. [Retained status evidence](evidence/video-completion-1f5a003.json) includes
the authoritative job/step result; detailed log download failed twice with
connection resets, so no log hash is recorded. Release qualification remains
incomplete.

Completed-request recovery now reads a bounded receipt from the durable journal
and authenticates the exact purge request, canonical JSON, succeeded operation,
removed runtime revision and absence of old-instance ownership records in one
transaction. It needs no backend, UID pool, process observation or filesystem
access. The public Manager purge path uses this read-only replay before legacy
cleanup; pending or absent receipts do not claim completion. Physical reserved
cleanup and its completion transaction are still not connected to public purge.

SQL-only recovery tests use a fresh manager after simulated UID reassignment,
including the actual Apply/purge replay path, and verify the replacement lease
remains intact. They refuse request drift, unknown/duplicate JSON fields,
trailing JSON, oversized records/phases, orphan/missing receipt data, unknown
phases and altered runtime/operation records. These tests do not qualify actual
crash/power-loss or safe UID reuse. The [Linux regression run](https://github.com/pranavreddyg17/home-node/actions/runs/40107127003)
at `eed5ad5` passed its retirement race and native component steps.
[Retained evidence](evidence/video-recovery-eed5ad5.json) records the source and
log hash. The fresh manager shares an existing Store; actual database reopen and
crash/power-loss recovery remain separate unqualified cases.

Volume preflight now composes with socket retirement: it authenticates the
pending intent and pins the recorded raw volume using `O_PATH` before any socket
effect. Every socket guard also checks volume inode/owner/mode/size/single-link,
parent/name/mount identity and absence of preparation/quarantine collisions.
The scope closes all retained descriptors before returning and keeps the UID
lease and provenance. Volume/directory component fixtures now use this composed
socket scope. Native preflight cases additionally require volume refusal before
socket mutation and loss-of-volume-authority refusal at the socket checkpoint,
followed by an authenticated retry. Linux cross-compilation passed; disposable
[execution](https://github.com/pranavreddyg17/home-node/actions/runs/40107402249)
at `0bd4688` passed all nine preflight cases and all existing native retirement
scopes. [Retained evidence](evidence/video-preflight-0bd4688.json) records source,
cases and log hash. This remains internal, uses injected stopped guards in its
fixture and does not qualify process exit, safe UID reuse or full video cleanup.

A physical retirement sequencer now authenticates the durable phase and resumes
only its corresponding socket-with-volume-preflight, volume or directory scope.
It requalifies final resource absence on replay and returns only after component
descriptors close. Successful physical retirement still retains all ownership
metadata and the UID lease; it neither calls the completion transaction nor
changes the public reserved-purge refusal. Native tests cover the full generated
resource sequence, stale-record replay, descriptor closure, interruption at all
six durable removal phases and refusal/preservation of a recreated volume after
physical completion. Linux cross-compilation and the [disposable run](https://github.com/pranavreddyg17/home-node/actions/runs/40107735685)
at `b2f6bf1` passed; [retained evidence](evidence/video-sequence-b2f6bf1.json)
records all ten sequence cases and the source/log hash. Injected stopped guards and synthetic disk bytes do not qualify actual
process exit, safe UID reuse or the complete application workflow.

Review of the composed sequence found that leftover per-instance preparation
paths also need refusal. Socket and volume guards now reject a channel
preparation path or premature directory quarantine; volume and final directory
guards reject volume preparation paths too. Final physical replay refuses and
preserves either recreated preparation path. These checks cover known managed
resource names; they do not establish absence of arbitrary UID-owned files
outside managed roots. The updated run at `b2f6bf1` passed both final preparation
refusal cases and all ten preflight cases, including refusal before any socket
effect when a channel preparation path exists.

Orderly journal reopen qualification is now explicit. At each of the six physical
removal phases, the native sequence test closes SQLite, discards its in-memory
intent, opens the existing journal again and constructs a fresh manager. The
exact request must reconstruct the pending canonical intent and phase before
physical retirement resumes. The completed-request recovery test also closes and
reopens the database after simulated UID reassignment, then invokes public purge
through a manager with no backend or pool and checks the replacement lease.
Local recovery race tests and Linux cross-compilation passed. The
[disposable native run](https://github.com/pranavreddyg17/home-node/actions/runs/40107953990)
at `cce3415` passed all six reopen-and-resume cases plus the existing retirement
scopes. [Retained evidence](evidence/video-reopen-cce3415.json) records source,
cases and log hash. This is orderly database close/reopen, not process-kill,
reboot, power-loss, live KVM or safe-UID-reuse qualification.

The disposable SIGKILL recovery harness now starts a separate root test child
with bounded generated configuration through stdin and a one-byte readiness
pipe. It requires an acknowledged target and actual SIGKILL termination, rejecting
timeout or unrelated child failures. Twelve targets cover all six durable phase
boundaries and the socket/volume/directory rename and unlink boundaries. The
child is killed with SQLite and resource descriptors open; parent recovery opens
the existing journal in a fresh manager, authenticates the original request and
requires physical completion with the UID lease retained. No fixture credentials
or workload data are passed in arguments or logs. Linux cross-compilation and
the [disposable run](https://github.com/pranavreddyg17/home-node/actions/runs/40108222447)
at `cbc66b9` passed all twelve SIGKILL cases and the existing retirement scopes.
[Retained evidence](evidence/video-sigkill-cbc66b9.json) records source, cases and
log hash. Generated resources and injected stopped guards do not
qualify power-loss, reboot, live VM exit, safe UID reuse or full video cleanup.

Safe UID reuse still requires effective write-confinement qualification. The
[upstream libvirt QEMU AppArmor abstraction](https://github.com/libvirt/libvirt/blob/master/src/security/apparmor/libvirt-qemu)
includes owner-writable temporary PulseAudio and shared-memory paths in addition
to dynamically added resource permissions. This is upstream source research,
not a claim about the effective Ubuntu profile on the runner or installed host.
An enforcing label alone therefore cannot prove that all UID-owned persistent
objects are among the retired HomeNode paths. Before enabling reuse, inspect the
exact shipped/generated profile, included abstractions, mount namespace and DAC
constraints; test positive allowed operations and negative writes outside the
retired resource set. Current cleanup remains fail-closed at this gate.

A dedicated disposable native confinement workflow now captures the generated
AppArmor profile source beside a live enforcing synthetic QEMU domain. The
fixture pins a protected source descriptor, runs the fixed protected parser's
`--preprocess` command with bounded output, checks the source and process label
again and retains expanded sources by SHA-256. The workflow requires KVM rather
than skipping absence, records runtime package versions and retains the source
artifacts for seven days. [Ubuntu's parser documentation](https://manpages.ubuntu.com/manpages/noble/man8/apparmor_parser.8.html)
classifies preprocessing as an unprivileged profile command that flattens
includes; this fixture does not load or replace profiles.

Linux cross-compilation and the [native run](https://github.com/pranavreddyg17/home-node/actions/runs/40108717734)
at `1c09326` passed. [Retained evidence](evidence/confinement-source-1c09326.json)
records package versions, both verified expanded-source hashes, selected rules
and the initial working-directory fixture failure. Both expanded profiles contain
owner-writable PulseAudio temporary paths outside HomeNode managed roots. This
inventory is
not proof that source and currently loaded kernel policy are equivalent. It
also does not measure write permission, mount-namespace restrictions or positive
and negative access under QEMU credentials. Those observations are still needed
before safe UID reuse or reserved public cleanup can be qualified.

The disposable write-access probe now uses a protected single-threaded C helper
linked to libapparmor. It joins the live QEMU mount namespace through an inherited
namespace descriptor, verifies namespace identity, adopts the actual AppArmor
profile, drops all IDs/groups, requires zero effective/permitted capabilities
and sets no-new-privileges. The parent independently verifies its enforcing task
label after a readiness message and acknowledges admission before file probes.
The helper opens the managed data
volume for writing as a positive control, requires refusal of a write-open on
the readonly system image, then attempts a generated temporary file write under
a PulseAudio-shaped path. It cleans its generated file and reports whether the
temporary directory matches the host's inode. No existing workload bytes are
written. The helper is a disposable test artifact, not a product executable.

Go/Linux cross-compilation, warning-strict Linux C compilation and the
[native probe run](https://github.com/pranavreddyg17/home-node/actions/runs/40109589284)
at `f74b8df` passed. [Retained evidence](evidence/confinement-write-f74b8df.json)
records both profile hashes, observations and prior self-label-read failures.
Both live-domain attempts allowed generated temporary writes and reported the
same directory inode visible on the host. The managed write-open positive and
readonly system-image negative controls also passed. This is a confirmed write
boundary finding; a successful observational job does not pass the security gate.
An allowed temporary write is reported as a finding, not a security qualification.
This probes a host task under the selected profile/namespace and DAC identity;
it does not run through a guest exploit, reproduce QEMU seccomp or establish safe
UID reuse. Public reserved cleanup remains refused.

Required remediation is now concrete: constrain each HomeNode VM's effective
writes to its admitted resources and explicitly lifecycle-managed private
scratch, then rerun positive managed operations and negative host temporary,
shared-memory, foreign-guest and management-path tests under the actual profile,
namespace, credentials and syscall policy. Avoid changing global libvirt policy
for unrelated VMs. Keep UID leases retained and public physical purge refused
until confinement and verified exit jointly support safe reuse.

The initial per-domain remediation renders a bounded static AppArmor profile
whose deny rules cover the complement of a finite write allowlist: the admitted
data file, adapter socket and fixed KVM/null/zero devices. A path-prefix trie
emits wrong-next-character, wrong-slash and truncated-path denials; deny rules
override writable permissions inherited from the distro abstraction. Literal
paths are bounded and reject policy metacharacters, readonly/write aliases and
missing reserved identity. The internal Domain selector binds the profile name
to the protected instance ID. Normal production domains do not enable it yet.

The disposable fixture loads only its separate profile, never changes the global
TEMPLATE or abstraction, launches the synthetic VM with that static label and
requires temporary writes to fail while managed write-open and readonly refusal
controls pass. It retains policy on uncertain VM/UID exit and removes it only
after observed exit. Focused Domain race tests and Linux cross-compilation passed;
the [native experiment](https://github.com/pranavreddyg17/home-node/actions/runs/40111880543)
at `ec01021` passed two launches under the same exact profile. Managed write-open
and readonly refusal passed, while host temporary writes were denied on both
attempts. [Retained evidence](evidence/confinement-boundary-ec01021.json) records
source/profile/log hashes and the development failures. Complete write-boundary,
network, inherited-descriptor, syscall and application-workflow qualification
remain required before production adoption or safe UID reuse.

Native iteration established two compatibility requirements. The shipped
[libvirt 10 AppArmor driver](https://raw.githubusercontent.com/libvirt/libvirt/v10.0.0/src/security/security_apparmor.c)
selects its UUID-based profile; the confined Domain therefore supplies a stable
UUIDv8 derived from its case-sensitive protected ID and requires that exact live
label. Exclusive profile publication refuses collisions. The profile separately
admits the per-domain runtime master-key filename for reading and the readonly
system image for locking; neither admission grants file writing. The compiler
checks require the native-accepted hyphen-first character-class form. Global
libvirt templates and abstractions remain unchanged.

Hardlink mediation is now explicit: the per-domain profile denies all hardlinks
rather than relying on link permission being absent from inherited rules for
admitted files. Write complement rules separately deny writing outside the
allowlist; readonly-image lock admission remains unchanged. The native helper
attempts to link the guest-owned data inode into its same-filesystem temporary
directory and requires EACCES/EPERM. An unconfined positive control uses the same
UID/GID, empty groups and zero capabilities to create, verify and remove a link
to that inode, demonstrating that ownership and protected_hardlinks alone do not
explain the negative result. Linux cross-compilation, focused Domain race tests
and the [native run](https://github.com/pranavreddyg17/home-node/actions/runs/40112644747)
at `a403cdb` passed. The workflow now requires exactly two paired positive and
negative observations. [Retained evidence](evidence/confinement-hardlinks-a403cdb.json)
records profile/log hashes and controls. The negative tests exercise combined
write/link denials, without attributing refusal to only one rule. This does not qualify arbitrary inherited descriptors, alternate filesystems,
application guests or safe UID reuse.

Inherited-descriptor qualification now deliberately delegates a generated
root-owned 0600 regular file, opened read/write by the unconfined parent, to the
probe child. After mount/profile transition, complete credential drop and
parent-confirmed enforcing label, the child verifies the descriptor's metadata
and attempts a one-byte pwrite. It accepts only a successful write or explicit
EACCES/EPERM refusal. The parent checks actual bytes against the reported result.
The workflow requires two observations alongside existing confinement controls.
No user or workload files are used. Linux cross-compilation and the
[native run](https://github.com/pranavreddyg17/home-node/actions/runs/40113093732)
at `386111f` passed. Both intentionally delegated descriptor writes succeeded,
with changed bytes independently verified by the parent. Fresh temporary writes
and hardlinks remained denied. [Retained evidence](evidence/confinement-inherited-386111f.json)
records the result and source/log hash. This measures this particular delegated regular descriptor,
not libvirt's actual inherited descriptors or all file/socket/device classes.

The inherited-descriptor result requires an explicit launch-authority audit:
inventory actual QEMU writable regular files and other descriptor classes under
the admitted process identity, bind necessary inherited authority to owned
resources and lifecycle, and refuse unexplained persistent writable handles.
A rendered pathname allowlist alone cannot establish that boundary. The observed
deliberate delegation is not evidence that libvirt passes an unsafe descriptor.
Production adoption and UID reuse remain closed pending that qualification.

Actual QEMU descriptor inventory is now implemented for the disposable launch
fixture. A pidfd detects process exit, exact DAC identity is checked before and
after, and a pinned proc directory bounds enumeration to 256 descriptor slots.
Each slot's flags, target and inode identity are observed twice; drift refuses
capture. Metadata records include object type, access flags, owner, inode/link
identity and target without reading file contents or opening target devices for
I/O. Writable regular data descriptors must match the admitted named volume.
The initial collector tagged domain logs as candidates; the stricter metadata
check below now refuses logs and all unexplained writable regular targets. All descriptor classes remain
visible for subsequent review. The workflow requires two inventories.

The native inventory run at `f2257c4` passed. This is a bounded
live observation, not retained exclusion of descriptor creation/transfer or a
production descriptor-admission implementation. Socket peers, anonymous objects,
log ownership/lifecycle and complete application startup still need qualification.

The first actual inventory run at `f2257c4` passed and observed 23 descriptors on
each launch. Writable regular handles included the admitted data volume and a
root-owned, mode-0644 libvirt PID file held write-only. This is an observed extra
inherited capability, not proof of an unsafe libvirt delegation. Other recorded
classes included devices, pipes, sockets and anonymous kernel objects.

Writable regular metadata qualification now refuses unknown paths, mismatched
named/described inodes, owner/group/mode drift, extra links, deleted objects and
unexpected access modes. The data file requires its exact reserved identity,
0600 mode and read/write access. The PID file requires its exact domain path,
root:root ownership, 0644 mode, single link and write-only access, and remains
explicitly tagged as a candidate role. This checks metadata; it does not grant
lifetime authority or establish that the writable PID capability is safe. Linux
cross-compilation and local metadata race tests passed. Native execution of this
stricter check passed at `fbbd10b`. Production adoption and UID reuse remain closed.

The candidate PID handle now has a post-stop native check: after libvirt reports
the VM stopped, its exact PID-file path must be absent and bounded procfs
observation must find no task with the reserved UID. Both stop cycles must emit
the required marker. This checks an observed lifetime on the disposable host;
it does not authorize production UID release, defeat process-creation races or
prove that every descriptor in other host processes has closed. Linux
cross-compilation passed. The [native run](https://github.com/pranavreddyg17/home-node/actions/runs/41105923277) at `c1f317f` passed both cycles.
[Retained evidence](evidence/confinement-regular-lifetime-c1f317f.json) includes
the actual descriptor snapshots and two post-stop observations.

Read-only regular descriptors now also require an explicit metadata role: the
exact system-image path and named inode, root ownership, the admitted guest
group, mode 0440, one link and read-only access. Unknown regular handles refuse
the synthetic launch inventory regardless of access mode. This does not hash
image contents or replace the catalog verification path. Focused local race
tests and Linux cross-compilation passed. The [native run](https://github.com/pranavreddyg17/home-node/actions/runs/41107430503) at `09e5ad0` passed both launches, as did retirement regressions.
[Retained evidence](evidence/confinement-readonly-09e5ad0.json) records the image
identities and remaining unclassified descriptor counts.
Production descriptor admission and UID reuse remain closed.

The synthetic inventory now also checks character-device metadata. Only
/dev/null (read/write), /dev/urandom (read-only) and /dev/kvm (read/write) are
recognized, with exact device numbers, root ownership, expected group/mode,
single link and named inode equality. Device paths may match either the
observer root or the pinned QEMU process root; the observed scope is recorded.
No device is opened for I/O. Double observation now also compares link count
and rdev. Local metadata race tests and Linux cross-compilation passed; native
execution passed at `1e552c6` and again at `2b78a0a`. This does not qualify device ioctl
surfaces, seccomp, sockets, anonymous objects or descriptor lifetime exclusion.

Socket inspection is now a disposable-fixture diagnostic using pidfd_getfd.
It duplicates one actual QEMU socket at a time, requires matching inode,
owner/group/mode/link metadata, queries family/type/protocol/listener state,
Unix addresses and connected-peer credentials, then closes the duplicate.
Only AF_UNIX is accepted; a disconnected socket must be listening. No message
is received, no SCM_RIGHTS is adopted, and no shared flags, socket options or
connection state are changed. Source-slot identity is rechecked afterward.
Two completed socket observations are required by CI. Linux cross-compilation
passed. The [native run](https://github.com/pranavreddyg17/home-node/actions/runs/41107962921) at `2b78a0a` passed both launches.
[Retained evidence](evidence/confinement-sockets-2b78a0a.json) records connected
monitor and listening monitor/adapter sockets, root peer credentials, and device
namespace observations. Peer credentials are observations rather
than authorization, and do not identify every process holding the peer endpoint.
Production peer admission and descriptor-transfer exclusion remain unqualified.
The [pidfd_getfd manual](https://www.man7.org/linux/man-pages/man2/pidfd_getfd.2.html)
explains the shared open-file-description authority and ptrace permission check.

The candidate profile now explicitly denies network inet and inet6, overriding
broader distro abstraction permissions for fresh Internet socket creation.
The confined native task must receive EACCES/EPERM for IPv4/IPv6 stream and
datagram socket creation and must successfully create an AF_UNIX stream socket.
The unconfined task with the exact reserved UID/GID, empty groups and zero
capabilities must create all four Internet sockets successfully. No bind,
connect or packets are used. CI requires two paired observations. Focused
profile race tests and Linux cross-compilation passed; native execution of
these network controls passed on both launches in the [native run](https://github.com/pranavreddyg17/home-node/actions/runs/41108247763) at `b8a67b5`.
[Retained evidence](evidence/confinement-network-b8a67b5.json) records the paired
controls and retirement regression result. This does not cover inherited Internet
sockets, other address families, guest packet paths or a complete firewall test.
Syntax follows the [Ubuntu AppArmor profile manual](https://manpages.ubuntu.com/manpages/noble/man5/apparmor.d.5.html).

Socket metadata qualification now refuses unknown endpoint roles. A live
libvirt numeric domain ID is resolved by the protected domain name before and
after capture; its exact runtime directory and shortened domain name bind the
monitor path. Only the owned adapter listener, domain monitor listener and
connected domain monitor stream are recognized, with exact AF_UNIX stream
protocol/access, inode-owner/group/mode/link metadata, listener state, local
path and observed peer shape. Connected monitor credentials must report a
positive peer PID and root IDs. These remain candidate roles: root credentials
do not authenticate the peer executable, prevent PID reuse, or inventory every
holder of the endpoint. Focused metadata race tests and Linux cross-compilation passed; native execution
passed in the [native run](https://github.com/pranavreddyg17/home-node/actions/runs/41108557157) at `eb4dbd5`. The synthetic fixture observed exactly one of each
expected socket role on both launches. [Retained evidence](evidence/confinement-socket-roles-eb4dbd5.json)
records those snapshots and retirement regressions. Production admission remains off.

Reserved-QEMU process verification now additionally requires Seccomp mode 2
and a positive canonical Seccomp_filters count on every observed task, using
the existing bounded protected-procfs task walker. Missing, duplicate, disabled,
strict-only or malformed fields refuse adoption. This applies to production
reserved process verification rather than only the candidate profile fixture.
The native inventory separately requires two per-task observation markers.
Focused parser/metadata tests, the full local supervisor race suite (23.388s),
and Linux cross-compilation passed; native
execution passed at `e60932b` and `38de2cb`, with six observed tasks per launch
on both runs. [Retained evidence](evidence/confinement-seccomp-38de2cb.json)
records the runs and retirement regressions. Mode/count cannot establish filter contents,
libvirt's requested sandbox options, forbidden-syscall behavior, or exclusion of
threads created after observation. These qualifications remain open.
See [Linux status fields](https://man7.org/linux/man-pages/man5/proc_pid_status.5.html)
and [libvirt's QEMU security model](https://www.libvirt.org/kbase/qemu-passthrough-security.html).

The disposable launch inventory now additionally reads a bounded cmdline from
the pinned QEMU proc directory and checks the requested sandbox options without
logging other arguments. It requires one exact -sandbox argument with on and
obsolete=deny, elevateprivileges=deny, spawn=deny, resourcecontrol=deny; missing,
duplicate, aliased, malformed, unknown or permissive option forms refuse the
fixture. CI requires two observations. Focused parser race tests and Linux
cross-compilation passed. The [native run](https://github.com/pranavreddyg17/home-node/actions/runs/41109163684) at `38de2cb` observed the required sandbox options on both launches. Process argv is
mutable and is not attestation of filter content or syscall refusal. Production
seccomp state checks remain separate from this diagnostic argument check.

Anonymous kernel-object metadata now has explicit candidate roles for observed
eventfds, signalfd, KVM VM, vCPU and read-only vCPU-statistics handles. Unknown
targets refuse capture. Each role requires the observed root-owned mode-0600
anonymous inode shape, one link and exact access mode; vCPU indices must be
canonical and inside the admitted CPU count. Focused metadata race tests and
Linux cross-compilation passed; native execution is pending. Anonymous inode
names do not attest VM ownership, installed seccomp contents, ioctl safety or
absence of transferred handles. These remain candidate fixture roles rather
than production authorization. Pipe authority still needs separate treatment.
