# App action recovery

App commands persist their instance, revision, operation ID and shutdown mode
before runtime dispatch. Replaying the same command key observes the existing
operation without repeating its runtime effects.

A failed start attempts cleanup of only its intended instance and revision.
If cleanup cannot be confirmed, the operation requires attention. A fresh start
cannot replace that record; an explicit stop remains available.

An error or invalid response after stop dispatch does not establish that the
runtime stopped. The operation becomes `requires-action` with
`STOP_CONFIRMATION_REQUIRED`, and the app stays `stopping`. New start and stop
commands are refused until runtime reconciliation changes the observed state.
In particular, a refused cooperative shutdown cannot silently turn a second
Stop into forced termination. If reconciliation observes a running app, its
next explicit Stop again records cooperative shutdown. Maintenance admission
remains closed throughout an unconfirmed stop.

Startup reconciliation accepts an inspection only when its instance and
workload match, its revision is positive and within the protocol bound, and its
state is running, stopped or interrupted. An unavailable or invalid observation
leaves an existing stopping fence intact. The management revision never moves
backward, including when a refused command did not advance the runtime revision.
Publishing an observation also compares the original instance, revision and
state. If another command changes the app during inspection, the stale result
cannot overwrite that newer intent.

When a validated inspection reports `stopped`, reconciliation atomically marks
the app's unresolved start/stop operation `interrupted`, retaining its original
payload. This settles current inactivity without claiming the original command
succeeded or dispatching another runtime effect. Running, interrupted and
invalid runtime observations do not settle the operation. A database failure
rolls back both the app observation and operation settlement. Replaying the
original key still returns the same operation and its recovered state.

An explicit stop records its immediately preceding unresolved app operation in
the durable stop intent. A confirmed stop or validated stopped inspection
settles its linked unresolved ancestors as interrupted in the same transaction;
a failed stop does not. Every link must name the same instance and workload and
have a strictly lower revision. The controller validates the full chain before
dispatch and before settlement. Histories exceeding 256 links, malformed
payloads, cycles and unrelated links require operator repair instead of
unbounded processing or unrelated operation settlement. The global maintenance
check continues to reject unresolved operations elsewhere.

The app status response includes a `requiresAttention` flag derived from its
current operation. The browser retains this warning across reloads, disables
Start and avoids polling a transition that cannot complete itself. This exposes
the recovery requirement. For an uncertain failed start, an administrator can
choose Clean up app, confirm interruption of any remaining instance and approve
the existing stop action with a passkey. Cancelling confirmation dispatches no
command. The browser cannot clear an operation directly. Uncertain shutdowns,
invalid histories and unavailable runtime inspection still need host-side
reconciliation; the complete guided repair workflow remains unfinished.

A new command cannot increment a negative revision or a revision already at
the protocol ceiling (`2^53`). Existing idempotency keys still return their
original operation despite damaged current metadata. Rejected new commands
roll back their operation insertion.

The supervisor currently admits a fresh cooperative shutdown only from its
running state. A runtime already shutting down cannot be given a new shutdown
owner through this browser control. Exposing a retry button for that state
would not implement recovery; resolving the original shutdown receipt and
runtime state remains necessary.

If the guest exits within its admitted shutdown deadline but the completion
reply is lost, the supervisor audit can now complete the original shutdown
journal after observing domain exit. It checks the original owner receipt,
instance, revision, desired state, deadline and pending operation, then records
completion and deletes the receipts atomically. This dispatches no additional
shutdown or forced stop. A foreign owner or failed journal write leaves the
runtime transition unresolved. Controller startup can subsequently observe the
stopped runtime and settle its own uncertain operation history. A running
controller now also observes apps requiring attention through a serialized
background worker, with a ten-second inspection deadline per app. This worker
never marks executing commands uncertain, never retries commands and skips
settled apps. Failed observations preserve the last known app state. Validated
stopped observations use the same atomic settlement and stale-snapshot guards
as startup. The browser's Refresh status retrieves the recovered state.
Physical guest shutdown and disk consistency remain unqualified.

`app_runtime_recovery_test.go` connects the production controller workload
service and supervisor Manager with separate SQLite journals, fixture immutable
image bytes, a simulated runtime backend and the actual guest file store. It
checks that lost completion retains the controller fence before audit, audit
finishes the original native receipt, live observation settles the controller
operation, original-key replay remains stable and an explicit restart can read
the retained file. It asserts one shutdown and zero forced stops. This is
cross-layer logic evidence; it does not execute QEMU, prove guest filesystem
flush behavior, exercise installed services, or use private HTTPS/passkeys.

The opt-in Linux `TestNativeReservedFilesManagerLaunch` fixture now injects a
completion-persistence failure after the actual Files guest's first shutdown.
It retains the production Linux backend, reserved identities and socket
provenance checks, then requires audit recovery and stable replay before the
second launch and retained-file transfer checks. Local Linux compilation alone does not qualify this native gate. The fixture must run
only on the disposable Linux runner, never the owner's workstation.

[Retained native recovery evidence](evidence/files-shutdown-recovery-8f15d42.json)
records that this test passed at source `8f15d42` in run 38844381680. The log
confirms the injected completion failure, audit recovery, stable replay and
subsequent restart/transfer checks. The test step succeeded; the full job was
still uploading its development disk artifact when this log snapshot was
captured. This qualifies the direct Manager recovery path within that fixture,
not installed controller/UI recovery or a production release.

The operation and app transition commit in one transaction. A failed metadata
write rolls back both. Superseding app intent and interrupted operation
authority are preserved rather than overwritten by an older completion.

`internal/workload/app_start_cleanup_test.go`, `app_shutdown_test.go` and
`maintenance_stop_test.go` exercise these rules with controlled runtime replies.
They do not qualify installed VM shutdown, physical power loss, or the complete
owner-facing recovery workflow. Those still need installed-system evidence.

The full development guest-image workflow subsequently completed successfully
for both Files and video. The evidence record retains the original snapshot
boundary and adds the final job/workflow conclusions and final log digest.
