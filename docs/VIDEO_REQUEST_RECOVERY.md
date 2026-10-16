# Video request recovery

Every conversion has a durable attempt and operation. Reusing its creation key
observes the same attempt. An intentional retry creates a new attempt and runtime
instance in the original retry group; it does not silently rerun uncertain work.

Cancellation reads current attempt authority in a metadata transaction. A queued,
preparing or cancelling attempt can become immediately cancelled only when
`start_requested` is false and no runtime cleanup journal exists. Its attempt,
operation result and event commit together, releasing the logical output quota
reservation. Failed writes roll everything back. This path works when runtime
services are unavailable and sends no start, stop or purge request. A stale
worker cannot subsequently acquire start authority for the cancelled attempt.

If runtime start was requested, cancellation remains pending until the runtime
stop path is confirmed. Missing runtime services do not mean a VM has stopped.
Contradictory cleanup metadata refuses the never-started shortcut and retains
the reservation. Started workload cleanup remains governed by its own persistent
stop/purge journal.

The Jobs screen offers Retry cancellation while an attempt remains cancelling.
This repeats cancellation for the same group and attempt; it does not submit a
new conversion. Retry as new attempt remains a separate explicit action after
a terminal failure, cancellation or interruption.

Succeeded, failed, cancelled and interrupted attempts are terminal. A late
completion cannot replace their result or emit another terminal event. Cleanup
may still be pending and visible separately; cancellation acknowledgment and
temporary resource cleanup are different observations.

Race tests use production orchestration and guest file stores with a simulated
video runtime and canned conversion output. They cover transaction failure,
quota retention/release, stale-worker refusal and terminal-result stability.
They do not qualify installed FFmpeg execution, guest isolation, physical power
loss or the complete authenticated conversion workflow.
