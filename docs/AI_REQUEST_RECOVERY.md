# Chat request recovery

The [chat OpenAPI contract](../contracts/chat.openapi.json) describes the current
conversation, history/export, generation/cancellation and approved deletion
operations. It shares passkey ceremony schemas with the existing device
approval contract. This is a chat slice, not the full product API freeze.

Conversation creation accepts a device-bound idempotency key. The creation
operation and conversation commit together. Matching retries return the same
conversation before maintenance admission or quota checks; changed intent
conflicts, and replay never recreates a deleted conversation. Requests without
a key retain the legacy API behavior.

The browser saves the generated title and key before creation dispatch and
retains them after an unknown result, including across reloads. It removes that
intent only after acknowledgment. This marker contains no messages. A later
intentional creation gets a fresh key. Message generation separately retains
its device-bound retry key without storing prompt content locally.

An unacknowledged creation is labelled Retry conversation creation after reload.
If it cannot be resolved, the owner may confirm Forget pending request after
loading the conversation list. That clears only the matching local marker; it
does not delete a server conversation or dispatch another creation. Cancelling
keeps the marker. A marker changed by another window is preserved and requires
reopening AI before retrying the local action.

Conversation-list reads share one in-flight request and abort when leaving AI.
Read failures are separate from action errors. An acknowledged creation remains
known even if its subsequent list refresh fails. History distinguishes loading,
unavailable and empty states, supports retry, and aborts obsolete reads on
conversation switch. Generation polling is serialized, aborts on exit, and
clears its read error after recovery.

Deletion uses a fresh passkey-approved action. An acknowledged pending deletion
marks the conversation unavailable for new messages. The browser observes its
durable deletion status through the conversation list, without keeping a
separate two-minute operation polling loop alive after navigation. Leaving AI
cancels local list reads, not server cleanup. A lost action response triggers a
list refresh to discover the server's current state. Data is reported deleted
only after confirmed completion or removal from the list.

Backend race tests cover concurrent creation replay, maintenance admission and
deleted-chat refusal. The browser identity workflow commits creation through
the real controller, loses its response, reloads, and checks one conversation
with stable key/title/ID. Network fixtures exercise read failures, recovery and
cancellation. These checks do not qualify installed model inference, guest data
erasure, physical devices or private-network deployment.
