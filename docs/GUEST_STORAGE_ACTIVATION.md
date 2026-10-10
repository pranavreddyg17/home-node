# Storage migration activation exclusion

The recorded guest storage proposal is not runtime authority. The installer
does not publish a reserved guest pool while the following
transaction is unfinished. This document identifies the implementation boundary, not a passed
qualification gate.

## Manager semantics

Systemd v255 checks conditions when a queued start job executes, rather than
when it is queued. Conditions do not change the state of an already active
unit. Empty condition assignments reset the condition list, and triggering
conditions have different combination rules. These semantics are documented in
the versioned [unit manual source](https://raw.githubusercontent.com/systemd/systemd/v255/man/systemd.unit.xml),
under Conditions and Asserts.

Therefore the existing persistent `recovery-blocked` marker can participate in
activation exclusion only when the manager actually loaded the owned, ordinary
negated path condition. Merely finding that line in a file, observing an
inactive unit, or retaining a marker descriptor does not prove exclusion.
Already active services and guest processes must be stopped separately.

## Required migration transaction

1. Hold the private installer lock across the entire operation. Validate the
   installed journal, owned configuration and canonical saved storage intent;
   derive identities independently from live account and device observations.
   Reject inconsistent installation state before any ownership changes.
2. Durably create the activation marker, retain its descriptor and pathname
   identity, and preserve it after cancellation, failure or process crash.
   Verify the fixed service/socket unit conditions and the manager's loaded
   configuration, with no foreign drop-ins, transient substitutions or pending
   reload. Reject condition resets or triggering alternatives.
3. Stop the credential socket and fixed application services through the system
   manager using bounded synchronous calls. Reobserve their jobs, state and
   process IDs. Independently prove the guest subtree is empty; stopping the
   supervisor alone is insufficient because libvirt can retain QEMU processes.
   Resolve already-running guests through owned runtime teardown before
   admitting migration. Never kill a process based only on a reused PID.
4. Retain the marker and installation authority throughout descriptor-qualified
   storage inspection, ownership-intent commitment and ownership changes.
   Reverify marker, configuration, device policy and runtime exclusion around
   each mutation. Publish immutable runtime pool/group policy in coordinated
   journal order. Interrupted steps must reconcile from exact recorded inode,
   identity and bytes; conflicting or foreign storage must remain untouched.
5. Enable reserved preparation only after its volume/channel lifecycle consumes
   that policy and ownership intent. Qualify actual application launch,
   restart, backup admission, teardown and cross-guest refusal under shipped
   protections. Release activation only through a separately journaled accepted
   transition; no cleanup defer or failed-command path may remove the marker.

The trust boundary excludes arbitrary privileged host administration: root can
replace unit files, remove a marker or launch QEMU independently. Detectable
drift must refuse further migration or activation; no claim of protection from
a malicious host administrator follows from these controls.

## Current evidence and missing integration

The installer has create-only durable marker handling, owned configuration
checks, bounded fixed-unit dormancy observation, guest-cgroup emptiness
observation and saved storage intent validation. Its recovery quiescence API
explicitly provides only a snapshot and also checks restore-destination vacancy;
that API must not be reused as a general storage migration lease.

Loaded-condition admission and a bounded fixed-unit stop operation are now
implemented in the recovery quiescence path. `homenode recovery-quiesce` checks
loaded identity before stopping and checks dormancy, conditions, marker and guest
emptiness afterward; it still returns no publication authority. Its Linux wire
query and native activation fixture have compiled but remain unverified by
native execution at this revision.

Private migration scopes now retain the storage proposal, installation and
activation records and the shared account-allocation lock while rechecking live
eligibility. Image helpers record immutable pre-change inode/content provenance,
retain image and parent descriptors, check pathname and mount identity, and
perform descriptor-based group migration with provenance-bound retry. These
helpers have cross-compiled. The installed image-phase API now joins those
records with live account/KVM eligibility, independently trusted catalog
verification, retained image descriptors and exact parent journal publication.
It requires an existing activation block and vacant controller, supervisor and
volume destinations; populated runtime storage migration is still unfinished.
The current composition remains pending native qualification.

Linux CI run [37970946612](https://github.com/pranavreddyg17/home-node/actions/runs/37970946612)
completed successfully for `a6eee60ecff252be3d8c0081ac81fa44f0fec397`.
Its root installer suite includes the owned image-batch and parent-publication
interruption fixtures. The retained log reports package results rather than
individual fixture outcomes; future runs use uncached verbose execution to
retain those outcomes. These disposable-root fixtures do not invoke the full
installed image command against actual account, KVM and manager authority, and
do not establish runtime policy publication or activation qualification.

The subsequent uncached run [38004410342](https://github.com/pranavreddyg17/home-node/actions/runs/38004410342)
passed at `94650df538b8a7da6799ec6d5f5ed2a9ff4b3cc2` and explicitly reports
`TestRootGuestStorageImagesMigrationRecoversInterruptedBatch` and
`TestRootGuestStorageParentPublicationRecoversOwnershipInterruption` as passed.
This confirms execution of those interruption/retry fixtures rather than only
their compilation. Their synthetic runtime observers still do not qualify the
full installed image command against the actual system manager or account/KVM
authority.

The private parent publisher records and reconciles its exact ownership/journal
intermediate state, while preserving the activation block. Owned guest teardown,
other storage/configuration transitions, immutable policy publication and
activation release remain unfinished. Native tests must
exercise a queued start while the marker is
present, an already-running service, a live guest after supervisor stop, marker
replacement, cancellation and crash at every publication boundary. Run those
tests only on disposable Linux hosts, never on the developer's desktop.

An explicitly configured manager now joins reserved resource preparation and
launch publication under retained account-allocation exclusion. It requalifies
prepared ownership before starting and retains account authority through live
domain/socket verification and revision-bound publication. Failed launch cleanup
inside the consumer retains that scope. Direct Linux backend preparation still
refuses reserved domains: callers must enter through the manager lifecycle.
Native manager-launch execution is recorded below. The shipped
CLI now consumes an explicit reserved policy after live account/KVM qualification
and read-only parent ownership/mode/ACL observation. Reserved startup does not
create storage directories or repair ownership. Native installed CLI qualification,
immutable installer policy publication and installation activation integration
remain pending.

CI run [38020466571](https://github.com/pranavreddyg17/home-node/actions/runs/38020466571)
passed at `0dea5fb3baa8c27b8c4d53130abf5975181e10c9`, with an explicit pass for
`TestNativeReservedDACManagerLaunch`. The synthetic firmware guest completed
manager launch, audit, stop and identity-retaining restart using native KVM.
The same run explicitly passed the root configuration batch and interrupted
journal-publication fixtures. Their runtime guards are synthetic; these results
do not qualify the installed excluded publisher, signed application images,
boot-time channel-parent provisioning or activation release. The scope and
retained log digest are recorded in
[evidence/reserved-manager-0dea5fb.json](evidence/reserved-manager-0dea5fb.json).

The supervisor's systemd unit owns `/run/homenode` through `RuntimeDirectory`.
Reserved CLI startup requires an already qualified `/run/homenode/guests`
parent. The unit now sets `RuntimeDirectoryPreserve=yes` to retain the recorded
runtime/channel inodes across stops; native qualification of that unit change is
pending. Provisioning and recreation after reboot remain an installation/boot
integration requirement. Creating a matching
parent opportunistically in the reserved runtime loader would bypass the
recorded provisioning authority and is not an accepted completion of that step.

Linux CI run [38020955501](https://github.com/pranavreddyg17/home-node/actions/runs/38020955501)
at `01ff8e8cb5b3dc429cc0078c4eaf40b59e3a72a5` explicitly passed the retained
empty-volume parent intent and interrupted ownership/journal publication fixtures.
These use temporary root-owned directories and synthetic runtime guards. They
do not prove the later public migration command, populated-volume migration or
activation release. Exact scope and the retained log digest are recorded in
[evidence/empty-volume-parent-01ff8e8.json](evidence/empty-volume-parent-01ff8e8.json).

Configuration preparation now has an installed exclusion composition. It
requalifies the saved allocation against live accounts, requires all three
storage parents at their final ownership, verifies the independently trusted
signed catalog and actual image bytes, and captures both installed configuration
files against their journal hashes. Both source descriptors remain retained
while the immutable configuration intent is recorded. Staging runs under the
same runtime/account exclusion; a retry with an existing stage receipt must
retain its recorded source and replacement inodes. Unrecorded partial stages
remain evidence and are refused. This composition has compiled for Linux but
has not yet completed a native installed workflow; it does not provision a
missing channel parent, publish configuration or release activation.

Channel provisioning now has a create-only staging primitive under a retained
`/run/homenode` descriptor, mount and named-path guard. It creates
`.homenode-guests.stage` privately, verifies an empty directory without ACLs,
assigns `root:transfer` ownership and mode `0710`, and records both runtime and
candidate inodes in `guest-storage-channel-stage.json`. It preserves uncertain
partial candidates and refuses adoption by repeated staging. Publication,
recorded-stage recovery, installed-command composition and reboot integration
remain unfinished. The primitive and disposable root fixture compile for Linux;
native execution of this new fixture remains pending.

The recorded-stage consumer now retains the immutable receipt and both runtime
and candidate descriptors, admitting exactly one of the fixed staging/final
paths. The publisher uses `RENAME_NOREPLACE`, synchronizes the candidate and
runtime directory, and verifies the recorded final inode. Retry after publication
rechecks and synchronizes the same inode without rewriting the receipt. A fixture
interrupts acknowledgement after the rename and retries, then refuses a foreign
final directory with matching ownership. These additions compile for Linux;
native execution and installed/reboot composition remain pending.

The supervisor protection fixture now checks the source's preservation setting
with two completed native systemd services sharing a disposable runtime directory.
It verifies that the outer inode and a `root:transfer`-shaped channel inode and
permissions survive directory reuse. This checks stop preservation, not reboot
provisioning or actual installed supervisor restart. Systemd documents that
`yes` preserves runtime directories on stop, while `/run` directories are still
removed at reboot: [systemd execution documentation](https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml).

New channel receipts use version 2 and include the kernel boot UUID read from
the fixed procfs `boot_id` interface. Staging rechecks that identity and recorded
publication requires a matching current boot before admitting the consumer.
Inode reuse on a later boot therefore cannot qualify a stale receipt. Missing,
old-version or foreign-boot records are preserved and refused; authorized receipt
rollover and reboot provisioning remain unfinished. A root fixture checks that
a canonical receipt with a foreign boot UUID cannot reach publication authority.
Linux compilation has passed; native qualification of these additions is pending.
