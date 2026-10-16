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

The app status response includes a `requiresAttention` flag derived from its
current operation. The browser retains this warning across reloads, disables
Start and avoids polling a transition that cannot complete itself. This exposes
the recovery requirement; it does not authorize clearing the operation or
provide a complete guided repair workflow.

The operation and app transition commit in one transaction. A failed metadata
write rolls back both. Superseding app intent and interrupted operation
authority are preserved rather than overwritten by an older completion.

`internal/workload/app_start_cleanup_test.go`, `app_shutdown_test.go` and
`maintenance_stop_test.go` exercise these rules with controlled runtime replies.
They do not qualify installed VM shutdown, physical power loss, or the complete
owner-facing recovery workflow. Those still need installed-system evidence.
