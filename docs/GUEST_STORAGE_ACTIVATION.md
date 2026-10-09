# Storage migration activation exclusion

The recorded guest storage proposal is not runtime authority. Production
reserved-DAC preparation remains refused while the following transaction is
unfinished. This document identifies the implementation boundary, not a passed
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

The private parent publisher records and reconciles its exact ownership/journal
intermediate state, while preserving the activation block. Owned guest teardown,
other storage/configuration transitions, immutable policy publication and
activation release remain unfinished. Native tests must
exercise a queued start while the marker is
present, an already-running service, a live guest after supervisor stop, marker
replacement, cancellation and crash at every publication boundary. Run those
tests only on disposable Linux hosts, never on the developer's desktop.
