# Authenticated Files integration gate

Status: open. The 1 GiB joined HTTPS-to-KVM workflow passed at `3faf952` in
run 39368428314, including active-download revocation, retained owner integrity
and all four service restarts. See [retained component evidence](evidence/https-files-large-3faf952.json).
Installed-product, browser/mobile and physical-network requirements remain unverified.
The standalone HTTPS identity/frontend fixture still runs without a
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
roles passed separately at `5d86740` in run 39306090347. Its log records
controller UID 997, transfer UID 995 and runtime GID 985, actual executable
startup and the real manager lifecycle. That run predates workload client peer
pinning and the joined HTTPS workflow. See
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
memory workload backend. The standalone gateway fixture does not enable this
mode. The joined development guest fixture enables it only with fresh native
supervisor/transfer services. The mode has compiled for Linux but has not yet
executed against a guest. Its bounded smoke
payload does not satisfy the 1 GiB, network-loss, active-transfer revocation,
quota or complete service-restart requirements above.

The development guest workflow now includes a joined mode after the separate
manager regression. `HOMENODE_FILES_HTTPS_INTEGRATION=1` starts a fresh supervisor
journal and transfer executable, passes their protected sockets to the HTTPS
fixture, and enables the client mode. The controller receives only socket inode
bind mounts inside its source-confined unit. Its private temporary namespace
does not expose workload volume, image, channel or journal trees. The fixture
reuses only the explicitly handed-off controller/runtime identities, creates
and removes its own gateway/proxy identities, and refuses unqualified sockets.
The controller allocates the guest instance; native journal inspection must
observe it stopped afterward. Failed workflows retain storage when effects may
still be in flight, even after best-effort VM cleanup. This joined mode has
compiled but has not yet passed native CI; its gate remains open. The first
native run is 39362374540 at source `8f8574c`. Its log confirms the complete
32 KiB Files lifecycle reached its success marker through actual HTTPS and the
native guest. The enclosing identity test then failed on the replacement
credential's final login with HTTP 429 after the combined ceremonies exceeded
the unchanged production 30-authentication-requests-per-minute budget. The job
therefore failed and is not a qualified joined pass. The client now paces its
combined authentication sequence, preferably before opening a ceremony, without
changing the limiter or silently retrying signed mutations. Corrected run
39363486412 at `8b7657f` passed, including the actual joined Files workflow and
the enclosing identity/recovery sequence. This qualifies the bounded 32 KiB
component demonstration at that source. It does not cover the subsequent
active-download or partial-upload revocation changes, a complete service restart,
the 1 GiB mode or signed release installation. See
`evidence/https-files-8b7657f.json`.

The subsequent client revision supports an explicit `files_https_bytes` workflow
input: 32768 bytes by default or 1073741824 bytes for the required 1 GiB case.
It generates payload windows of at most 256 KiB from a small randomized pattern,
hashes downloaded and archived content as streams, and stores the ZIP in a
private temporary file with a payload-plus-1-MiB disk budget. This avoids holding
the object or archive in client memory. The fixture has separately bounded
timeouts for the large case. An upload acknowledgement body is deliberately
abandoned, the connection is closed, and committed state is recovered through a
fresh request. A download is interrupted after 16 KiB before resuming with
Range/If-Range and checking the suffix hash. These are explicit client-side
interruptions, not physical network-loss qualification. This revision has
compiled but has not executed natively; neither payload size is newly qualified
by the earlier source run, and active-device revocation remains open.

The next large-case client revision keeps Files running while it enrolls the
paired Files device. Immediately before the owner's signed revocation it opens
that device's verified 1 GiB download, reads a 16 KiB prefix and pauses. After
revocation it requires an incomplete HTTP body with unexpected EOF before a
30-second deadline, then re-downloads and hashes the whole retained file using
the owner's independent session before approving the final guest stop. The
large-mode harness requires both truncation and owner-integrity success markers
in addition to the enclosing test pass. The small case does not claim to prove
active data-path revocation. This extension has compiled but has not executed
natively; active upload revocation, service restart and the other requirements
remain open.

Files mutations now refresh the cookie's current session and Files capability
after JSON body reception and before creating transfers, sending chunks,
cancelling or changing files. Finalization also refreshes authority immediately
before starting. This closes the slow-body gap where route authentication could
precede device revocation, logout or a capability change. The refreshed session
replaces the handler actor. The regression changes authority on the first body
read, after route admission, and checks refusal with no guest call or transfer
metadata change. A bounded guest mutation already admitted before revocation
may finish; this check does not claim rollback of committed bytes or native
active-upload termination qualification.

The subsequent native client adds a partial-body upload check in both payload
modes. A paired device creates a separate 32 KiB transfer through HTTPS, receives
100 Continue, and sends half of a valid chunk body while withholding the rest.
The owner revokes the device with its signed approval before the body completes.
The client then releases the remaining body and requires a certificate-verified
401 `AUTH_REQUIRED`. After the complete identity flow, the root fixture opens
the actual controller database read-only and requires the same transfer ID,
zero committed offset, uploading state and a revoked device record. No session,
app or transfer state is seeded or edited for that inspection. The initial
controller-admission timing is covered separately by the body-reader regression;
100 Continue and prefix transmission establish the native gateway body path.
This case has compiled but remains pending native execution.

Native partial-upload run 39364512000 is testing `acea852`. Independently, main's
32 KiB joined test passed at `1b1b4e0`, with the Files body-authority fix included.
Run 39365139607 now requests `files_https_bytes=1073741824` against that same
tested main revision. It covers the large transfer and active-download revocation
extension but predates the partial-upload test; the two runs must not be
combined into a claim that one source passed every requirement. Both results
must be inspected before their respective gates can change.

Partial-upload run 39364512000 passed at `acea852`, including the explicit 401
and read-only journal check for zero committed offset. The initial 1 GiB run
39365139607 failed at finalization with HTTP 409 after its chunk loop. It does
not qualify large transfers. Bounded transfer diagnostics now distinguish
unavailable/timed-out guest responses from guest refusals without logging object
IDs, names, contents or raw guest error text; the diagnostic rerun remains
pending. The session activity-write optimization passed local identity/control
race suites and native gateway run 39365437176 at `e431532`.

The next fixture revision adds all-four-service restart. After a signed guest
stop, the live protocol client pauses on a pipe, retaining its actual session
cookie and generated passkey only in memory. The root orchestrator stops the
controller and gateway, invokes a fixture-only root-peer-protected Unix endpoint
to replace the supervisor and transfer, then recreates the confined controller
and gateway with fresh socket mounts. The supervisor requires a stopped journal
and observed guest exit before restarting; transfer peer PIDs and controller/
gateway executable identities are observed again. The client resumes, verifies
the same enrolled device, replays the original transfer creation key/body and
requires the same ready object, then signs a fresh guest start and checks bytes.
Run 39367007350 passed this workflow at `50f8acd`, including observed new
supervisor/transfer PIDs and the retained session/request/data checks. This is a
32 KiB component demonstration. Only controller/gateway use
source-confined transient units here; canonical installed-unit lifecycle and
signed release qualification remain separate requirements. Failed or ambiguous
restarts preserve fixture storage rather than declaring cleanup successful.

Manual development-image runs can select `profile=files` for focused Files
qualification or `profile=video` for video. The default remains both; push runs
always select both profiles. A focused run qualifies only the selected
component and cannot substitute for the complete product release matrix.

Diagnostic run 39366902074 at `f98c5e5` confirmed the large finalization failure:
the public request returned 409 after 20.003 seconds and the transfer executable
reported `operation=finalize`, `timedOut=true`, `elapsedMs=20000`. This matches an
operation budget shorter than a fully cold 1 GiB read at the configured data
disk ceiling of 50 MiB/s. Whole-object `finalize` and `stat` calls now receive a
bounded 85-second budget under a 90-second service write deadline. Other calls
retain a 20-second budget; caller deadlines and cancellation still take
precedence. The budget includes body decoding, queueing, inspection and guest
I/O. The corrected 1 GiB case passed in run 39368428314 at `3faf952`, including
Range resume, ZIP integrity, all four service restarts and active-download
revocation with retained owner access. Exact truncation counts and timing are
not printed by the enclosing fixture; no precise latency claim is made.

The public finalization path now checks the live session, device, identity epoch,
expiry/idle bounds and Files capability inside both its verification-admission
and publication transactions. A guest may finish an already admitted hash or
rename, but withdrawn authority cannot publish the result as a ready file.
Denied publication retains the verified guest object, verifying journal state
and quota reservation. Local regressions withdraw authority after the real guest
byte store finishes verification, confirm no file metadata is published, and
prove that a new session for the same device can reconcile once without another
upload. These are component regressions; native withdrawal during whole-object
verification remains a separate execution case. All three main workflows passed
at `f293054`: HomeNode checks 39370028910, Gateway security boundary 39370028703,
and Development guest images 39370028924. This includes the publication
regressions and bounded native workflow, not native late-verification revocation.

Fixture-signed development catalogs are distinct from release publisher trust.
Passing this integration would still leave physical networking, browser/mobile
acceptance, signed installation and the remaining product release gates open.

The large native case now also stages a separate 128 MiB paired-device upload,
restarts the guest through signed owner actions, starts finalization and observes
`verifying` through the public transfer API before owner-approved revocation.
It requires explicit HTTP 401 and read-only journal inspection showing the full
retained offset, a revoked device, verifying state and no published file record.
This observes verification admission, not the exact instant hashing starts;
already admitted guest work may finish. Native execution of this added case is
pending. Only bounded selected non-secret size, truncation and owner-integrity
result lines are emitted from the captured client for future evidence retention.

Large run 39459032258 at `2283f16` failed before the new verification-admission
revocation case: the initial cooperative stop returned `requires-action` before
`NATIVE-RESTART-READY`. Storage and the uncertain ownership fence were retained.
This is not evidence for or against the later publication assertion. Main's
bounded workflow passed at the same revision. Root shutdown failure logs now
report only elapsed time and deadline/cancellation classification, without raw
backend errors, object IDs, paths or payloads; the next large run must diagnose
or pass this earlier stop before qualifying the added case.
