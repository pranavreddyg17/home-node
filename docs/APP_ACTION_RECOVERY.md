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
the durable stop intent. A confirmed stop settles that predecessor as
interrupted in the same transaction as its own completion; a failed stop does
not. This covers direct cleanup of an uncertain start. Recovery of longer
chains after repeated cleanup failures still needs implementation and tests;
the global maintenance check continues to reject any unresolved ancestors.

The app status response includes a `requiresAttention` flag derived from its
current operation. The browser retains this warning across reloads, disables
Start and avoids polling a transition that cannot complete itself. This exposes
the recovery requirement; the browser cannot clear the operation and does not
yet provide a complete guided repair workflow.

The operation and app transition commit in one transaction. A failed metadata
write rolls back both. Superseding app intent and interrupted operation
authority are preserved rather than overwritten by an older completion.

`internal/workload/app_start_cleanup_test.go`, `app_shutdown_test.go` and
`maintenance_stop_test.go` exercise these rules with controlled runtime replies.
They do not qualify installed VM shutdown, physical power loss, or the complete
owner-facing recovery workflow. Those still need installed-system evidence.
