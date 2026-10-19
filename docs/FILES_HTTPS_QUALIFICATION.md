# Authenticated Files integration gate

Status: open. The HTTPS identity/frontend fixture currently runs without a
privileged workload backend. Its authenticated Files listing is metadata-only;
it is not an upload, guest execution or retained-data demonstration.

The separate native Files fixture already launches an actual unsigned Files
guest through the supervisor and transfer executables. Its current policy
uses controller UID 1 and transfer UID 2. The helper clients now take identities
from that accepted policy rather than hardcoding them, including transfer
directory ownership, process credentials, runtime requests and negative-role
checks. The runtime client receives the expected controller UID explicitly and
still requires matching kernel credentials and disposable integration opt-in.
Linux compilation passes; native regression of this helper refactor is pending.

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

Fixture-signed development catalogs are distinct from release publisher trust.
Passing this integration would still leave physical networking, browser/mobile
acceptance, signed installation and the remaining product release gates open.
