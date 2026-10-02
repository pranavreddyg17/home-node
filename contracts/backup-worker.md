# Installed backup worker protocol

This contract describes the implemented private Linux worker channel. It is not
an HTTP API, a remote compute interface, or evidence of physical-host validation.

The controller connects once to the trusted root-created, socket-activated
`unixpacket` listener. The sender checks the kernel peer UID; the receiver checks
the configured non-root controller UID. Path ownership/group/mode validation in
the installed dispatcher is required before connecting. A packet carries bounded
UTF-8 JSON plus exactly one sealed anonymous read-only close-on-exec password
file descriptor. Password bytes are validated and cleared; they are not JSON,
command arguments, environment variables, durable settings or reply contents.

## Requests

JSON is at most 1024 bytes. Each field listed below is required, with exact case.
Unknown fields, duplicate decoded keys (including escaped duplicates), aliases,
null, invalid UTF-8, invalid IDs, oversized packets and trailing documents fail
closed. Installed workers accept exactly one of these disjoint schemas:

| Domain | Version | Fields |
| --- | --- | --- |
| Preliminary launch | 2 | version, jobId, deviceId, managementToken, release, catalogVersion |
| Stopped runtime cleanup | 3 | version, jobId, deviceId, managementToken, runtimeToken |

Launch carries no runtime token, destination, executable or arbitrary command.
Installed release/catalog metadata and registered repository configuration are
validated independently. The worker authenticates and retains the exact mounted
repository before attempting runtime acquisition. Cleanup independently qualifies
owned stopped/restoring state before releasing backup-owned runtime authority.
The legacy acquired-dispatch version 1 is not accepted by this installed entry
point. There is no schema downgrade or permissive fallback.

## Replies

Replies are exact UTF-8 packet bytes, with one literal space before the same
job ID and no terminator, trailing data, credential or error detail:

| Callback outcome | Reply prefix | Controller meaning |
| --- | --- | --- |
| Launch successful | homenode-backup-launch-complete-v2 | Callback stopped successfully; independently inspect durable publication/root evidence |
| Repository refused before runtime acquisition | homenode-backup-launch-repository-refused-v2 | Callback stopped after pre-acquisition repository admission failure; independently qualify owned refusal checkpoint |
| Cleanup successful | homenode-backup-cleanup-complete-v3 | Callback stopped successfully; independently verify recorded runtime release |

The server joins callback execution and closes the received credential before a
reply. It holds admission through reply and slot release so immediate next work
cannot race the previous slot. Only a typed pre-acquisition repository admission
failure can generate refusal. Cleanup, configuration, credential, acquisition,
cancellation and arbitrary failures cannot be relabeled as refusal. Reply-send
failure stops and joins the service. No failure details are serialized.

Only the exact authenticated launch refusal reply maps to
`ErrLaunchRepositoryRefusedStopped`; the worker-local
`ErrLaunchRepositoryAdmission` is insufficient. Receipt must be uncancelled and
match the launch domain/job. A successful transport write, empty runtime token,
process restart, timeout, EOF, wrong job or old-domain reply is not stop evidence.

## Lifecycle and recovery

The server admits one received/running operation. Request/credential transfer is
bounded by five seconds; launch completion by two hours; cleanup completion by
three minutes; replies by five seconds. Caller cancellation can shorten these
limits. Cancellation stops intake, cancels work and joins it. There is no automatic
replay after handoff or a lost reply.

Before delivery, the controller durably records `uncertain:<job>` while its owned
job is freezing with no attached root. Uncertain jobs remain blocked and eligible
for explicit reconciliation; generic transitions cannot leave freezing. Successful
launch completion requires independently matching published outcome and owned
root/publishing phase before recording `complete:<job>`.

A stopped refusal requires an atomic dedicated transition: owned freezing job,
active owner, empty root, drained inventory, exact uncertain marker and no
publication for this job. It records `refused:<job>` and restoring together.
Valid earlier published history is retained. Contradictory or unresolved history
fails closed. This checkpoint does not claim a new snapshot or release a root.

Restoration runs with a bounded recovery context. Admission reopens only after
workload restoration and durable completion checks. A backup refusal remains a
backup failure even when restoration succeeds. Restoration failure retains
requires-action/refused state. Explicit retry consumes a fresh owner passkey
approval bound to job, empty JSON body, request key, session, epoch and policy.
Recovery authority is requalified before workload effects. It cannot replay
publication, acquire runtime authority or guess uncertain cleanup succeeded.

Public status distinguishes none, uncertain, complete and refused. Recovery job
selection comes from the owned current maintenance journal; an earlier snapshot
must not become the refused recovery target. Public responses contain no private
maintenance/runtime tokens or repository passwords.

## Evidence and remaining acceptance

Parser/domain/cancellation tests, durable rollback/restart/history/contradiction
fixtures, approval/HTTP recovery tests and virtual-passkey browser fixtures cover
these boundaries. Linux socket/server/coordinator fixtures require native Linux
execution. Empty-workload HTTP fixtures and mocked browser transport do not prove
live VM restoration, external-drive correctness, installed systemd activation or
power-loss recovery. Those remain separate release acceptance requirements.
