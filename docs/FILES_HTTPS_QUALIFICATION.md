# Authenticated Files integration gate

Status: open. The HTTPS identity/frontend fixture currently runs without a
privileged workload backend. Its authenticated Files listing is metadata-only;
it is not an upload, guest execution or retained-data demonstration.

The separate native Files fixture already launches an actual unsigned Files
guest through the supervisor and transfer executables. The Files fixture now
creates fresh `homenode` and `homenode-transfer` system accounts with distinct
private groups and a shared `homenode-runtime` group on its explicitly disposable
Linux runner. It refuses existing names, uses the observed IDs in policy and
channel/socket ownership, and removes only accounts it created after teardown.
The synthetic firmware-only fixture retains its original numeric identities.
The helper clients now take identities
from that accepted policy rather than hardcoding them, including transfer
directory ownership, process credentials, runtime requests and negative-role
checks. The runtime client receives the expected controller UID explicitly and
still requires matching kernel credentials and disposable integration opt-in.
Linux compilation passes. Run 39305868215 passed the helper refactor at
`6af6bcc`: its log confirms actual KVM Files guest execution, the supervisor and
transfer executables, role refusals, direct 1 GiB object transfer, disk-full
handling and retained restart. The executable transfer exercise is separately
bounded; the 1 GiB result is not an HTTPS transfer demonstration. Named Files
roles remain pending; that later change is not covered by this run. See
`evidence/files-policy-roles-6af6bcc.json`.

The next integration must join these paths with fresh, distinct named service
identities and the real supervisor/transfer sockets. It must preserve catalog
signature verification, reserved guest identity admission, channel/volume
provenance, service peer checks and native cleanup. A memory backend, a seeded
session or an existing guest instance substituted into controller state cannot
satisfy this gate.

Required demonstration:

1. Start an empty controller state with the actual runtime clients, enroll
   through verified HTTPS, and approve the Files app start using a fresh signed
   ceremony. Let the controller allocate its own instance and operation IDs.
2. Upload through the public transfer API into the actual guest, interrupt a
   chunk response and resume from observed committed state without duplicate
   effects. Begin with a bounded payload, then qualify the required 1 GiB case.
3. Download through the gateway, verify content SHA-256, interrupt/resume with
   the supported Range/ETag contract, and exercise quota, Trash and ZIP export.
4. Revoke a paired Files device during an active transfer/download. Observe
   termination and retained-data integrity; event-stream evidence alone is
   insufficient for these guest data paths.
5. Stop/restart the real services and guest with retained storage, retry the
   original request identity, and verify bytes and operation outcomes. Record
   source, image digest, policy, role identities and bounded diagnostics.

The generated HTTPS identity client now has an explicit
`HOMENODE_NATIVE_HTTPS_FILES=1` mode for the joined fixture. It refuses a
controller without configured workload sockets or with existing file metadata.
Using the enrolled owner, it obtains fresh signed app approvals, starts Files,
polls the actual operation, creates a 32 KiB transfer with the same request key
twice, observes committed chunks, verifies downloaded bytes and Range/ETag,
exercises Trash/restore and completed ZIP contents, then stops/starts Files and
checks retained bytes before a final stop. It neither seeds state nor supplies a
memory workload backend. The current gateway fixture does not enable this mode:
it must first be joined to fresh native supervisor/transfer services. The mode
has compiled for Linux but has not executed against a guest. Its bounded smoke
payload does not satisfy the 1 GiB, network-loss, active-transfer revocation,
quota or complete service-restart requirements above.

Fixture-signed development catalogs are distinct from release publisher trust.
Passing this integration would still leave physical networking, browser/mobile
acceptance, signed installation and the remaining product release gates open.
