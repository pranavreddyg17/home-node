# Implementation record

HomeNode's end-to-end implementation is in progress. A completed code path is not a passed release gate.

## Delivered foundation

- Host prerequisite report, loopback diagnostics API, responsive dashboard.
- Go tests, vet, Linux cross-build and browser smoke checks passed for the initial slice.
- Persistent SQLite identity; WebAuthn enrollment/login; capability-scoped pairing, sessions, revocation, recovery and redacted diagnostics.
- Browser end-to-end test passed with a Chromium virtual authenticator: setup, pairing, denied admin access, revoke, sign out/in, recovery invalidation and mobile width.
- Production requires private-network binding and TLS; explicit development mode is loopback-only.

## Runtime and file implementation

- Added an independently authorized Unix-socket supervisor, protected operation journal, signed immutable catalog checks, resource admission, no-NIC libvirt XML, confinement checks, timeout/audit enforcement and restart reconciliation.
- Added a separate transfer service and bounded, correlated guest protocol. Guest content handling includes durable checksummed chunks, replay checks, immutable finalization, and path confinement.
- Added persistent app operations and Files API/UI with storage reservation, resume, download, rename, trash and restore. The controller never mounts a guest filesystem.
- Unit/integration tests exercise actual guest byte storage, control authorization, response headers, supervisor admission/replay/failure policy and catalog verification. These tests do not prove VM isolation.
- Video and CPU inference guest adapters exist; their controller workflow, image packaging and validation are not complete.

## Current work

1. Identity follow-ups: action-bound sensitive approvals, broader browser/device coverage, recovery/export tooling.
2. Typed supervisor protocol, independently enforced signed catalog/resource policy, confined libvirt runtime, guest adapters.
3. Resumable transfers, isolated files, durable video jobs, CPU AI, functional UI.
4. Private TLS/network setup, packaging, verified updates, encrypted backup/restore, export/removal.
5. Abuse/failure tests, release tooling and complete documentation.

## Release evidence outstanding

- Physical Ubuntu 24.04 x86-64 host with KVM/AppArmor: isolation, overload, network scans, power-loss and restore drills.
- Tailnet account and two real devices: certificate provisioning and pairing.
- Measured CPU model profile, licensed artifact inventory and release signing infrastructure.
- Independent security review, nondeveloper usability test and 72-hour soak.

Development is on macOS; none of these hardware or external review gates may be marked passed by unit tests or simulated execution.

## Video and AI control workflows

- Video jobs now have separate job/attempt identities, bounded queuing and output reservations, durable transitions, cancellation, explicit retry and uncertain-work reconciliation. Results are copied back into Private Files and checked before success is committed.
- Per-instance lifecycle revisions prevent delayed start/stop messages from reversing newer intent. Stop intent is recorded even before a VM exists; temporary video volumes can be removed only after the instance is stopped.
- AI conversations use bounded guest inference, status polling, cancellation, history export and deletion. Prompts and answers remain on the AI guest disk; management state contains opaque references and operation metadata.
- Linux CI is configured to test the real FFmpeg adapter, kernel Unix peer identity and distribution libvirt XML schema. These are separate from the still-outstanding physical-host isolation test.
- Local Go race tests, vet, Linux cross-build, web production build and browser identity/navigation regression passed for this slice. Inference tests use controlled stream fixtures, not a qualified CPU model benchmark.

## Upload expiration

- A bounded background worker removes expired partial uploads through the Files guest. Reservations remain held while the guest is unavailable or deletion fails; successful deletion is synced before metadata releases quota.
- Cleanup takes the same per-upload lock as upload/finalize, rechecks state, and retries idempotently after interrupted acknowledgment. Tests cover unavailable guest, actual byte removal, rejected expired finalization and cleanup replay.
- Paused or failed uploads can be explicitly cancelled in Files. Cancellation is device-scoped, blocks further upload/finalization immediately, and holds quota until durable guest cleanup. API tests exercise capability and cross-origin denial; workflow tests cover offline cleanup and replay.

## Development packaging

- Added a Linux amd64 Debian package build containing the controller, supervisor, transfer service, guest-image executable, web assets, payload checksums and explicit path ownership documentation.
- The package has no automatic provisioning or removal hooks and does not include private configuration or data. It is unsigned development tooling, not a production distribution or completed installer.
- CI now builds/extracts the actual package and checks its entire payload against its checksum inventory. Local shell syntax checks pass; Debian packaging is tested on the Linux runner because this Mac has no `dpkg-deb`.
- Upload expiration and cancellation slices passed Linux CI before this packaging change.
- The package build, extracted payload checksum verification and artifact retention passed Linux CI on commit `0886f3b`. Systemd provisioning templates were added afterward and have separate checks pending.

## Service provisioning groundwork

- Added inactive systemd templates for separate controller, transfer and supervisor identities, protected paths, empty unprivileged capability sets, required private HTTPS configuration, and a bounded workload slice.
- Templates ship as setup inputs, not enabled services. Account creation, policy generation, measured limits, certificate provisioning and resumable activation are still pending.
- CI installs the development package on its disposable Ubuntu runner and verifies unit syntax against real installed executable/dependency paths. Runtime confinement and service lifecycle still require supported-host validation.

## Supervisor shutdown

- Supervisor shutdown now waits for HTTP drain and attempts to stop every recorded active guest before exiting. Start admission closes permanently on the retiring manager.
- A stop failure no longer skips remaining guests or records false success. Failed instances remain visible for the next retry/reconciliation; errors propagate to the service exit status.
- Tests cover two workloads, a failed stop, continued cleanup, rejected starts and successful retry. Forced runtime stop is not a clean application backup; the maintenance protocol remains outstanding.

## Recovery snapshot groundwork

- Added consistent management snapshots using the SQLite driver's online backup API, private staging, integrity checking and file/directory syncing.
- Recovery copies remove sessions, credentials, pairing/setup challenges and recovery-code trust; revoke old device capabilities, advance identity epoch and require new enrollment. Active workload intents are interrupted/stopped and unfinished uploads require cleanup. The source database is untouched.
- Removed trust records are cleared and the recovery copy compacted. Tests check standalone WAL data recovery, byte-level absence of seeded secrets, private permissions, cancellation cleanup, refused overwrite and revoked trust.
- Service templates, package installation/integrity checks and supervisor shutdown passed Linux CI on `1494acb`. This snapshot primitive is not a complete backup: registered external drive, encrypted restic orchestration, clean guest stop, manifest/restore compatibility and UI are still pending.

## Backup destination admission

- Added Linux mountinfo parsing and registered UUID/device checks: exact writable filesystem mount, host-root rejection, bind-subdirectory rejection, filesystem allowlist, bounded input and strict escaping.
- Admitted destinations retain an open directory descriptor and verify the opened filesystem identity/read-only state. Backup code must use that pinned handle rather than resolving the mount path again after admission.
- Tests cover mismatched/missing mounts, read-only and bind mounts, wrong devices, duplicate mounts, malformed input and parser fuzz seeds. Linux CI exercises rejection of a nonexistent registered drive even when its configured host directory exists.
- Registration still needs physical-drive ancestry/removability checks; a different partition alone is not proof of an external drive. Authenticated restic repository identity, encryption, drive registration UI and backup/restore orchestration remain unfinished. This primitive does not enable backup writes.

## Restic authentication

- Added authenticated encrypted-repository config checks against the registered repository ID. Restic receives a fixed local repository path through an inherited directory descriptor, with no network backend, shell, inherited restic configuration or local cache.
- Passwords are passed through an anonymous sealed memory descriptor, never arguments/environment/disk files; command output is bounded and errors discard raw output. Password bytes are still present in the initiating process while in use.
- Linux CI initializes a disposable real restic repository and tests correct credentials, wrong credentials and wrong repository identity, plus sealed password descriptor behavior. Physical-drive backup/restore and orchestration remain outstanding.
- Implementation follows the [restic repository guide](https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html) and [scripting guidance](https://github.com/restic/restic/blob/master/doc/075_scripting.rst); no custom encryption format is introduced.
- Authenticated repository sessions now retain the exact repository descriptor through a bounded full encrypted-pack check. Closing the session closes its password descriptor and prevents further operations. Linux integration coverage replaces the visible directory after authentication to test that checks remain pinned to the original repository.
- Repository checks detect storage corruption; application recoverability still requires a restore drill. Automatic unlock, prune and unattended credential retention are not implemented.
- Full Linux CI passed the authenticated session, descriptor-replacement and real restic encrypted-pack checks on `dd92455`.

## Backup manifest validation

- Added a bounded versioned manifest with platform/schema requirements, catalog security floor, approved immutable image references, fixed payload filenames, sizes and SHA-256 digests. Unknown/duplicate JSON fields, trailing documents, incompatible versions and arbitrary host paths are rejected.
- Payload verification is confined beneath an opened staging root, refuses symlinks/nonregular files, checks exact lengths and hashes with cancellation between bounded reads.
- Tests cover corruption, symlinks, traversal, missing/incompatible metadata, security-floor rejection and unapproved image references, plus parser fuzz seeds. This validates metadata and bytes, not database semantics or complete app recoverability; restore must also reconcile the snapshot's app inventory and rebuild host policy before activation.
- Manifest compatibility and payload verification passed full Linux CI on `84dd063`.

## Recovery database admission

- Added read-only snapshot admission against the release's compiled SQLite schema, integrity/foreign-key checks, removed identity trust, stopped/interrupted intents, and bounded app inventory. Restored views/triggers or schema additions cannot substitute fake recovery state.
- Complete recovery-set validation now requires private staging, verified payloads and an exact correspondence between recorded persistent apps and declared disk files. Missing app disks are rejected even when all included files have valid checksums.
- Tests use actual sanitized SQLite snapshots and reject recomputed/checksummed snapshots with claimed identity, running apps or an injected view. Guest disk fixtures prove byte/inventory checks only, not filesystem or VM recoverability.
- Disk installation, host policy reconstruction, external-drive registration, clean backup orchestration and end-to-end replacement-host restore remain unfinished.
- Full Linux CI passed recovery database authority/schema rejection and app disk inventory checks on `bb4b61f`.

## Encrypted snapshot primitive

- Authenticated repository sessions can now create encrypted restic snapshots from a validated private recovery set, including the canonical manifest and every declared disk. All source paths are fixed names beneath an inherited staging descriptor; command output is bounded and only a final valid snapshot ID reports success.
- Staging integrity is checked before and after the command. Callers must hold an exclusive maintenance lease and keep source disks stopped/immutable throughout; this primitive does not establish guest consistency by itself.
- Linux CI now tests real encrypted snapshot creation, full pack verification, byte-for-byte database recovery and refusal of corrupt staged payloads. The raw disk fixture verifies transport/inventory, not a recoverable guest filesystem.
- Backup service orchestration, clean stop/restart, operation journal/UI and replacement-host installation remain pending.
- Real encrypted snapshot creation, pack verification and byte-for-byte database recovery passed full Linux CI on `59faf59`.

## Encrypted restore staging

- Added typed restic recovery-set retrieval into empty private staging. Only the fixed manifest/database/disk names are read; declared sizes bound each output stream, hashes and schema/authority/inventory checks gate completion, and existing destinations are never overwritten.
- Failed retrieval removes only the files created by that operation and syncs cleanup. Restored data is not activated or given host trust by this primitive.
- Linux integration coverage now restores the complete encrypted set, refuses incompatible manifests/nonempty destinations, and exercises cleanup after a missing disk fails following successful database retrieval. Separate tests check oversized stream rejection.
- Host replacement wizard, clean backup maintenance, registered-drive provisioning, restore disk installation and UI/journaling remain outstanding.
- The first restore integration run rejected its test staging directory because Go's test child directory permissions were broader than the required 0700. The fixture now explicitly tests public-directory denial and creates private staging; production validation remains strict. The corrected full restore run is pending.
- Corrected full Linux CI passed encrypted-set restore, incompatible/private-stage denial and partial retrieval cleanup on `e314767`.

## Trash retention

- Seven-day trash expiry now removes guest objects durably before releasing quota. A minimal irreversible metadata tombstone preserves job/transfer references without making deleted content visible or restorable.
- Cleanup and file actions share a lock; expired restore is denied, and active job inputs delay deletion until the attempt is terminal. The Files interface discloses retention, shows expiry dates and disables expired restore.
- Tests cover offline deletion/quota retention, actual guest object removal, denied resurrection, replay and queued-job input protection. This is logical deletion, not a claim of forensic disk erasure or removal from older backups.
- Full Linux CI passed trash retention and active-input protection on `32e6e6a`.

## Durable temporary runtime cleanup

- Video attempts persist cleanup intent atomically before start reaches the supervisor. Stop/purge failures remain pending, use stable retry operation IDs and are retried by a bounded worker after terminal attempts.
- Successful result files remain available while Jobs reports pending temporary cleanup; the warning clears only after acknowledged removal. Controller reconciliation seeds cleanup for earlier recorded starts.
- Recovery snapshots clear ephemeral cleanup intents/start flags, and restore admission rejects them so replacement-host recovery cannot issue old temporary runtime actions.
- Fault tests cover purge failure, persisted intent across service recreation, stable retries, retained results and cleared status after success. Tests use runtime fixtures; physical VM/volume cleanup remains a supported-host gate. Maintenance backup orchestration is still outstanding.

## Failed result-copy cleanup

- Unpublished video output objects now retain a conservative 4 GiB reservation until the files guest durably acknowledges deletion. Upload and job admission include these reservations.
- A bounded worker shares the output-copy lock and rechecks publication before deletion. Successful copies remove their orphan record atomically with result publication; interrupted copies remain recoverable cleanup intent across service recreation.
- Race tests cover unavailable guest retention, actual partial-object deletion, quota release after acknowledgement and refusal to delete published files. Supported-host VM and storage fault qualification remains outstanding.

## Private HTTPS identity validation

- Full Linux CI passed durable runtime and orphan-output cleanup on `5a95cf1`.
- Production startup now requires the running Tailscale node’s DNS/IP identity, an assigned local tailnet IPv4 address, matching unprivileged listener/origin port, stable DNS origin and a currently trusted server certificate/key. This runs before management state/listener creation. Read-only `network-check` exposes the same validation without key content.
- Root-owned protected TLS files reject final symlinks, special files, unsafe permissions and oversized input. New TLS handshakes observe certificate replacement with bounded revalidation and fail closed on invalid replacements/expiry.
- Tests validate trusted identity, malformed/mismatched origins, nonlocal/LAN/public binds, certificate validity/trust, special-file rejection and a real TLS 1.3 HTTPS request followed by rejected bad replacement. This is local/fake-PKI coverage, not phone/private-network qualification.
- Network setup documentation separates identity checks from tailnet authorization and remote-device reachability. Installer integration, automated issuance/renewal and supported-host policy/phone verification remain outstanding.

## Resumable configuration ownership

- Full Linux CI passed private HTTPS validation and certificate reload on `1f2b858`.
- Added the installer’s owned-configuration transaction engine: restricted paths/permissions, private journal, cross-process lock, planned ownership/content hashes, atomic no-overwrite file publication and synced transitions.
- Resume reconciles incomplete staging or published intent. Rollback retains borrowed directories, refuses changed files and preserves occupied owner-data directories without recursive deletion.
- Race tests include child-process exit at staging/publication/removal boundaries, replay, concurrent installer refusal, cancellation, foreign configuration, path escapes and journal corruption. Linux CI also runs a root-only temporary fixture for distinct controller ownership.
- This is the filesystem phase, not the complete installer. The trusted orchestrator, journal bootstrap, account creation, verified images/releases, activation, update/uninstall integration and physical power-loss acceptance remain outstanding; see `docs/INSTALLATION_JOURNAL.md`.

## Concrete installation configuration

- Full Linux CI passed the journal engine and root ownership fixture on `42f475a`.
- Added deterministic configuration generation from typed network/account/policy values, independent publisher trust/catalog floor and host capacity. It verifies all three catalog profiles, service/group separation, Files/video execution capacity and initial disk headroom/reserve.
- Generated service units come from embedded reviewed sources; the resource slice now matches policy exactly. Canonical network values are shared with production HTTPS validation and cannot inject systemd environment syntax.
- Tests cover deterministic policy/unit/catalog binding, malformed identities, signed catalog tampering/rollback, incomplete profiles, resource/storage rejection and a root-only generated-plan install/rollback fixture. Debian packaging retains templates/docs without packaging their new Go source file.
- Configuration generation does not prove current account memberships, allocate/verify guest images, validate tailnet authorization, activate services or complete onboarding. Those gates remain explicit in the preview and installer integration work.

## Local service identity validation

- Full Linux CI passed configuration generation, packaging and generated-plan root install/rollback on `e148ddf`.
- Added read-only `accounts-check`: protected bounded local account/NSS snapshots plus deadline-bound system resolution confirm distinct locked nologin users, system ID ranges, no UID/group aliases, exact private/runtime membership and local-only identity sources.
- The configuration planner uses the same numeric identity range. Account/password content is excluded from results and diagnostics; no account tools run in this CLI.
- Race tests cover account/credential/group/NSS failures. A separate opt-in disposable Linux CI fixture creates real identities, checks the installed CLI, denies privileged supplementary membership and tests repair/cleanup; that new integration result is pending.
- Account creation journaling, full installer/activation and physical-host acceptance remain outstanding. This closes identity admission, not the complete provisioning lifecycle.

## Account creation intent and native cleanup

- `1b35e78` passed Linux Go/root fixtures, vet/build, web build and package inspection. Its native account/installed-CLI checks and privileged-membership repair completed; the run failed when cleanup tried to delete private groups already removed by userdel. Browser/artifact steps did not run.
- Fixed native cleanup to distinguish definite group absence from lookup failures; initial fixture vacancy checks now use the same fail-closed distinction. No account-admission rule was weakened.
- Added deterministic account creation planning with reserved unused system IDs, occupied-name refusal, stale-membership/dangling-GID protection and fixed useradd/groupadd vectors. Tests cover collisions, namespace exhaustion, ownership-marker syntax and dependency identity.
- Planning changes no accounts. Durable account intent/execution/reconciliation and production rollback remain unfinished before full installer integration.

## Journaled service account creation

- Full Linux CI passed native account/CLI inspection, repair and cleanup on `a1a8fa7`.
- Added root-only `accounts-provision` over the existing private installer journal. Fixed command templates reserve identities, persist intent before effects, check live vacancy and verify observed attributes before committing progress. It starts no services and removes no identities.
- Matching user/group effects survive lost command results and engine recreation; foreign names, live ID collisions, disappeared completed identities and changed ownership/membership are conflicts rather than implicit repair or reuse.
- Tests cover lost user-creation acknowledgement, durable resume/replay and collision/ownership refusal. Native CI now exercises actual journaled creation/replay before installed CLI inspection; its new result is pending.
- Journal bootstrap, full fresh-host installation/activation, account/data-aware removal, malformed OS database recovery and supported-host power-loss/VM gates remain outstanding.

## Fresh installation journal bootstrap

- Full Linux CI on `c929126` passed actual journaled account creation/replay, installed CLI inspection, browser workflows and package retention.
- Default account provisioning now securely creates the fixed private journal directory after validating and pinning its system parents. Existing unsafe directories and symlinks are rejected without permission repair; the shared installer lock still prevents concurrent execution.
- Added bootstrap replay, lock contention and unsafe-parent/journal fixtures. These run locally and in the existing Linux/root fixture suite. Physical power-loss durability remains a host acceptance gate.
- Full installer orchestration, verified guest image installation, private-network onboarding, service activation and account/data-aware removal remain unfinished.

## Bound configuration phase

- Connected configuration generation and owned publication behind completed account ownership admission and native live marker/membership checks.
- Native configuration uses supported-host preflight and observed memory/disk/CPU capacity, overriding supplied capacity/account fields and binding runtime peer IDs. Generation and apply share the installation lock.
- Added denial fixtures proving missing/incomplete/mismatched account intent produces no configuration journal, and a root-only temporary fixture for observed-value binding and replay.
- Targeted race tests, vet and command builds passed locally. Linux root fixture evidence for this change is pending; live host preflight, images, network identity, activation and user-facing installation orchestration remain outstanding.

## Verified immutable image placement

- Full Linux CI passed account-bound configuration and its root temporary replay fixture on `117de41`.
- Added journaled local import of all three signed catalog images, bound to unchanged committed configuration and independent publisher trust/version floor. Input names are content digests; symlinks/special files, unsigned or mismatched bytes, occupied foreign destinations and changed completed images fail closed.
- Copying uses bounded streams, cancellation and SHA256 verification, private staging, synced mode 0440 root/QEMU ownership, no-overwrite publication and synced progress. Replay checks all image bytes and can resume after a lost publication acknowledgement.
- Portable tests cover stream integrity/cancellation/symlink rejection; root-only temporary fixtures cover publication acknowledgement loss, replay and ownership conflicts. Targeted local race/vet/build checks passed; new Linux root fixture results are pending.
- This adds image placement, not bootable guest release artifacts, downloads, update/removal, activation or VM/hardware qualification. Full installation remains incomplete.

## Installation preparation command

- Full Linux CI on `783c768` passed streamed image import and root-owned publication/replay/conflict fixtures, plus the existing native account/package/browser checks.
- Added `install-prepare` to connect account provisioning, account/capacity-bound configuration and independently authenticated local image placement. Release/settings/source shape and host prerequisites are checked before journal bootstrap; signals and a deadline bound execution.
- Publisher key and catalog floor are explicit independent trust inputs. Peer identity/capacity are not input flags. Success reports prepared configuration and remaining gates with services inactive; replay uses the existing phase journals.
- Race tests cover parsing/trust, catalog rollback/expiry, source absence, invalid policy/network, unknown identity flags, symlink and oversized catalog input. Local targeted race/vet checks passed; new Linux CI is pending. Actual CLI preparation on a supported host, bootable release artifacts, HTTPS/ACL qualification, activation and phone onboarding remain incomplete.

## Image-aware installation replay capacity

- Full Linux CI on `08c67c2` passed the preparation parser/CLI build, image/root fixtures, native account checks, package and browser workflows.
- Fixed configuration replay double-counting already-published image storage. Credit requires matching owned configuration/catalog and freshly verified image attributes/content; missing completed or changed images fail, and foreign files receive no credit.
- Owned partial staging is removed and synced under the installer lock before capacity is observed again. The preview keeps actual capacity and reports original/remaining allocation plus verified image bytes separately.
- Preparation defers only the provisional doctor disk floor to catalog-derived storage admission, retaining measurable-disk and non-storage host prerequisites. Root replay fixtures cover low-space publication replay, staging cleanup/remeasurement and foreign/changed refusal; new Linux results are pending.
- Local targeted race tests, vet and command builds passed. Full installer activation, maintained bootable releases, network onboarding and physical resource/power-loss acceptance remain incomplete.

## Independent catalog floor at supervisor startup

- Full Linux CI on `c071e9f` passed image-aware capacity replay, staging cleanup/remeasurement and foreign/changed artifact refusal root fixtures.
- Closed the initial supervisor trust-floor gap: preparation now commits a protected independent catalog floor, and supervisor startup requires it before opening runtime state. Previously recorded catalog versions can raise this floor but cannot lower it; invalid floors fail closed.
- Hardened supervisor configuration reads with no-follow/nonblocking descriptors, file identity/ownership/mode revalidation, bounded contents and policy trailing-data refusal.
- Local race/vet/build checks passed, including canonical floor parsing and maximum-floor selection. Linux CI now explicitly runs the supervisor protected-file fixture as root alongside installer fixtures; that new result is pending.
- Existing development configuration journals require a reviewed migration for the new owned file. Full update/migration, activation, maintained guest releases, network onboarding and physical acceptance remain incomplete.

## Transactional catalog acceptance

- Full Linux CI on `f800e1d` passed protected independent floor generation, supervisor root configuration-file fixtures, package and browser checks.
- Moved supervisor catalog floor read, signature/version admission and durable highest-version advancement into one SQLite transaction. A competing writer conflict fails closed instead of allowing an independently read stale floor to overwrite newer progress.
- Tests use actual private SQLite stores, including two independent connections starting concurrently; rejected rollback or corrupted trust does not advance the record, and the recorded version matches the highest successful commit.
- Local supervisor race tests, vet and command builds passed after fixing private test-directory permissions. New Linux CI is pending. Full activation, updater/migration, bootable guest releases and hardware/network acceptance remain incomplete.

## Inspection from committed installation state

- Full Linux CI on `7368dc8` passed transactional catalog admission, concurrent independent SQLite startup, root protected configuration fixtures, package and browser checks.
- Added `install-check` and native installation inspection deriving network, runtime policy, publisher/floor and image inventory from unchanged owned configuration. Fully published images are rehashed; live account markers/membership and protected local Tailscale/TLS identity are required.
- TLS inspection also enforces the controller-only private group for root-owned 0640 keys. It starts no services and makes no journal progress; peer-policy, VM-overhead/enforcement, activation and phone gates remain explicit.
- Root temporary tests cover valid read-only inspection, incomplete/changed/expired state, identity binding, missing floor, changed image and shared-runtime key access refusal. Local targeted race/vet/build checks passed; new Linux root results are pending. Actual host TLS/ACL/activation and maintained bootable guest release workflows remain incomplete.

## AI guest model readiness

- Full Linux CI on `11095fa` passed committed installation inspection and TLS controller-group root fixtures, plus package/native account/browser checks.
- Fixed AI guest health reporting ready before model loading finished. A separate deadline-bound no-proxy/no-redirect loopback health client now requires HTTP 200 and the compatible model-ready contract; loading/errors/invalid bounded responses report starting.
- Probe parsing rejects duplicate/trailing/unexpected fields and oversized responses without forwarding server error details. Separate connections prevent generation streams from consuming probe capacity; agent close releases both clients' idle connections.
- Guest/workload race tests, vet and command builds passed locally; contract/readiness/redirect tests exercise model loading and failures. New Linux CI is pending. Maintained bootable guest builders/releases, real licensed CPU model qualification and host/VM boot/isolation acceptance remain unfinished; see `docs/GUEST_STARTUP.md`.

## Guest shutdown and durable interruption

- Full Linux CI on `8321be1` passed AI readiness and redirect-denial probes, plus installer/root/package/browser checks.
- Fixed guest shutdown closing its confined data root before cancelled workers completed. Shutdown now rejects new work, joins workers, preserves interrupted task journals, reports retained journal-write errors and closes the root once.
- Guest process signals interrupt active channel reads and reconnect waits before agent closure. A channel opened during cancellation is closed before serving; late AI readiness cannot claim ready after closure.
- Local guest/workload/guest-command race tests, vet and command builds passed. Tests cover actual durable interruption/reopen, worker final writes, idempotent close, blocked read/retry cancellation and late readiness. New Linux CI is pending.
- This does not implement clean VM power-off, guest filesystem unmount, maintenance backup leases or bootable image qualification. Those end-to-end/hardware acceptance requirements remain unfinished.

## Bounded guest task recovery

- Full Linux CI on `f8bfbcc` passed joined-worker shutdown, durable interruption/reopen and guest channel cancellation, plus existing root/package/browser checks.
- Replaced whole-directory/whole-file startup task reads with bounded enumeration and descriptor-based private regular-file admission. Up to 1,000 records are decoded with canonical unique fields, strict state/profile/identity/output validation and no trailing JSON.
- All task records are validated before interrupted intent is rewritten. Symlink/special/oversized/malformed records fail without rewriting a valid running record. A 256 KiB journal bound supports existing 32 KiB AI text after worst-case JSON escaping; the writer enforces it too.
- Local guest/workload/guest-command race tests, vet and builds passed; fixtures cover invalid recovery and maximal escaped text round-trip. New Linux CI is pending. Bootable guest builders/releases, mounts and actual VM/host acceptance remain incomplete.

## Guest image service inputs

- Full Linux CI on `11409e3` passed bounded/private task recovery, malformed-state preservation and maximal escaped AI text recovery, plus root/package/browser checks.
- Added guest-only service/profile/virtio-access sources for the image builder. The template requires a mounted data volume and fixed device, runs as a dedicated guest user without capabilities, scopes writes/devices and uses the signal/joined-worker shutdown path.
- Profile quotas match current minimum image data capacities. Sources are not installed/enabled on the host; account creation, safe data-disk initialization/mounting, model service, complete bootable image builder/release artifacts and booted-device access qualification remain unfinished.
- Linux CI now validates the guest unit source after installing the development binary; its result is pending. Static validation does not prove a booted guest or actual mount/udev/confinement enforcement.

## Guest adapter overlay staging

- Full Linux CI on `adc67dd` passed guest unit static validation alongside native/root/package/browser checks.
- Added clean-checkout, pinned-toolchain Linux staging for each profile: static amd64 adapter, service/device/account inputs, exact payload inventory and source/hash metadata. Output publication does not overwrite occupied paths; metadata explicitly says the overlay is not bootable.
- Streaming verification rejects unexpected files/directories, symlinks/special entries, mode/hash/size mismatches, invalid ELF/profile/source metadata and duplicate fields. Portable integrity mutation tests and shell/Python syntax checks passed locally; actual Linux three-profile staging is a new pending CI step.
- The command safety review rejected recursive cleanup in the initial command; the implementation uses empty-directory cleanup and preserves failed staging instead.
- This is a required image pipeline component, not a qualified image/release. Kernel/rootfs/bootloader, account collision admission, data initialization, CPU model/runtime, signing/provenance and real VM boot/isolation gates remain incomplete.

## Deterministic guest overlay assembly archives

- Full Linux CI on `1344123` passed actual static Linux/amd64 staging/verification for all three overlays, guest unit validation and existing native/root/browser/package checks.
- Added deterministic fixed-inventory USTAR packaging with source timestamp, root ownership metadata, fixed permissions and canonical metadata. Every archived file stream is checked against its verified size/hash; source drift and occupied output paths fail before publication.
- Local archive tests passed for repeat byte identity, expected ownership/timestamps/paths, readonly output and occupied-output refusal alongside existing mutation tests. CI now packages each real overlay twice, compares bytes and retains the three unsigned archives after successful checks; new results are pending.
- Repeat packaging is not compiler/image reproducibility or signed release provenance. Safe assembler/extraction, kernel/rootfs/bootloader/data initialization, model packaging, actual boot/VM tests and production activation remain unfinished.

## Digest-pinned overlay import for assembly

- Full Linux CI on `824b652` passed actual three-profile archive repeatability and retained unsigned guest overlay artifacts, alongside existing root/native/package/browser checks.
- Added bounded private archive snapshotting with expected digest/profile/source revision, exact fixed-member admission and exclusive file creation into new protected staging. Generic archive extraction is not used; links/special/duplicate/extra/escaping members fail before staging.
- Extracted bytes, modes, source identity, ELF/profile quota and metadata are reverified. Failed staging stays private; successful files/directories and parent publication are synced. Existing output paths are refused.
- Portable tests passed for round-trip, mismatch/source refusal, private failed staging, occupied outputs, invalid headers and archive bounds. CI now imports and verifies each real compiled archive; new Linux results are pending.
- This connects verified artifact import to assembly without declaring a bootable release. Rootfs/kernel/bootloader, guest account/mount initialization, model/runtime packages, release signing, activation and physical VM acceptance remain incomplete.

## Guest image account admission

- Full Linux CI on `3d967bc` passed real three-profile digest-pinned imports/reverification and retained unsigned archives, plus native/root/package/browser checks.
- Added read-only image-root identity admission with bounded no-follow/nonblocking descriptor reads, protected ownership/modes, exact fixed locked nologin UID/GID, alias/primary/supplementary group refusal and local-only NSS.
- Portable tests passed for identity drift, collisions, privilege/password/NSS failures and protected shadow/symlink inputs. Added a separate opt-in root Linux fixture applying systemd-sysusers only inside temporary image roots, including requested-ID collision refusal; that native result is pending.
- Requested sysusers IDs are not proof of actual allocation. Complete rootfs/kernel/bootloader assembly, safe data initialization/ownership, model/runtime packages, signing and actual VM boot/activation qualification remain unfinished.

## Guest named-device startup

- Replaced the ordering-only global udev settle edge with a fixed named-device dependency and lifetime binding. The matching guest udev rule now exports the systemd tag/alias while retaining private account ownership and mode.
- Portable overlay and account tests passed. Linux CI checks canonical systemd path escaping and unit syntax; actual delayed virtio arrival/removal, boot startup and guest confinement remain unverified.
- Identity admission commit `62e6079` remains under Linux CI validation at the time of this change. Neither static dependency wiring nor account fixtures establish bootable guest releases.

## Protect guest account admission directories

- Guest identity reads now admit only four fixed inputs and validate ownership and no group/other writes on the opened image root and every traversed account directory, in addition to the protected file checks.
- Portable tests passed for world/group-writable roots and account directories, intermediate symlinks and path traversal/unknown input refusal; the Linux sysusers fixture remains opt-in. Assembly staging still requires exclusive trusted writers and release/image verification.
- CI `36667603116` was cancelled by the newer push; its cancellation is not a validation result. CI `36667770808` is the authoritative pending Linux run for the identity fixture and named-device unit changes.

## Native guest group declaration repair

- Linux run `36667770808` failed the positive native sysusers identity fixture; Go/root tests, builds, package inspection and real overlay compilation passed, while later unit/browser checks were skipped.
- Upstream systemd v255 sysusers implementation requires an explicitly supplied numeric primary GID to exist. The guest source now declares its fixed private group before the user. Exact observed identity and collision admission remain mandatory; native validation of the corrected source is pending.
- Directory-protection commit `bbf1414` and this repair are shipped together for the next CI run. This is account assembly repair, not completed guest image boot qualification.

## Guest disk role serials

- The supervisor now emits distinct fixed system/data serials on the existing isolated raw disks. Structured XML tests bind each role to its correct source, target and read-only/writable mode.
- Local supervisor tests passed. Installed-libvirt schema validation is supplied by Linux CI; physical/kernel serial discovery, filesystem identity, safe initialization and actual mounts remain pending.
- Corrected native account/unit CI `36668044046` is still running. The disk-role change is committed locally while that validation completes, to avoid cancelling its native fixture again.

## Guest named data mount source

- Full Linux run `36668044046` passed corrected native systemd-sysusers account/collision admission, protected directories, canonical device escaping/unit validation, real three-profile overlays, native/root/package and browser workflows. This is not booted guest evidence.
- Added the guest-only ext4 `data.mount` using the fixed data serial/by-id path and nodev/nosuid/noexec. The existing adapter mount dependency selects it; no host installation, formatting or filesystem repair is performed.
- Overlay inventory/staging, deterministic archive packaging and fixed-member import now carry the mount source. Portable overlay/import/account tests and shell syntax passed. Linux validation for disk-role XML and mount syntax is pending.
- Safe blank-disk initialization, block/filesystem admission, private object ownership, full kernel/rootfs image assembly and actual boot/power-loss/isolation qualification remain incomplete.

## Guest private object root admission

- Agent startup now rejects a nonprivate, foreign-owned or final-symlink object root and checks the opened root identity. Missing parents are not implicitly created; existing permissions/data are never silently repaired.
- Added rejection tests preserving existing bytes/modes and refusing missing ancestry and symlinks. Existing guest fixtures now explicitly create private roots. Local guest/workload/controller tests passed.
- Mount/serial CI `36668333488` remains running. This change is committed locally pending its completion; full boot initializer, mount provenance and booted filesystem/hardware qualification remain incomplete.

## Native guest object ownership fixture

- Linux CI `36668333488` passed real overlay packaging/import, installed-libvirt disk XML schema, mount/unit syntax, native account checks, root/package and browser workflows. This does not prove a booted disk mount or guest filesystem initialization.
- Pushed private-root admission commit `ece64d4`. Added a separate opt-in root Linux fixture assigning a temporary directory to foreign UID/GID 900 and asserting rejection preserves its owner, mode and existing bytes. Only this disposable fixture changes ownership; agent admission never does.
- Local guest tests passed with the native fixture skipped on macOS. The next Linux CI run must establish native ownership refusal. Guest-side blank-volume initialization and rootfs/kernel assembly remain unfinished.

## Binary guest mount admission

- Production guest entrypoint now checks fixed data path, named block device/serial, kernel writeability and a single whole ext4 mount of the same major/minor at `/data`, with nodev/nosuid/noexec and rw filesystem options. Kernel evidence reads are bounded; checks do not open raw disks for I/O or change mounts/data.
- Mount parser and guest command tests passed; Linux cross compilation passed. Real mount/device metadata and blank-disk initialization remain booted-image qualification gaps.
- Private-root CI `36668596200` remains running; the foreign-owner fixture and binary mount admission are committed locally pending its result. Full guest image assembly and end-to-end activation remain unfinished.

## Select the guest-visible data mount by kernel ID

- Mount admission now opens `/data` without following a final symlink, verifies the directory's filesystem device, reads bounded kernel fdinfo and selects mountinfo using the opened mount ID. Hidden stacked mounts cannot authorize an unsafe visible mount; repeated selected IDs fail.
- Parser/guest command tests and Linux cross compilation passed, including safe and unsafe visible mounts stacked above hidden entries. Actual systemd mount namespace and booted-device validation remain pending.
- CI `36668972192` passed the disposable foreign-owner preservation fixture and root tests so far; the full run is still active. This change stays committed locally until that run finishes. Full disk initialization and maintained guest image assembly remain unfinished.

## Native descriptor mount evidence fixture

- Full Linux run `36668972192` passed foreign-owner preservation, root/native account checks, package/browser workflows and real guest overlays. It predates the mount-ID selection changes and is not evidence of booted guest mount admission.
- Extracted the production bounded descriptor mount-ID reader and added a Linux-only test opening an actual temporary directory, selecting its current kernel mount record and comparing the descriptor filesystem device to mountinfo. Invalid descriptors fail. No mount, format or host disk modification occurs.
- Local parser/guest command tests passed and the Linux test binary cross-compiled. The actual native descriptor test must run in the next Linux CI build. Both this fixture and `25f42da` are now submitted together; safe data initialization and full bootable image assembly remain unfinished.

## Guest object startup initialization

- Full CI `36669263454` passed native descriptor mount evidence, root/foreign-owner/account checks, real overlays, package and browser workflows. It does not prove actual guest boot admission.
- Added a separate guest-only root initializer and oneshot dependency. After mount admission, it creates only a private object leaf under a protected root:root mount root, assigns fixed guest UID/GID 900 and syncs directories. Existing foreign, nonprivate, symlink or incomplete roots are preserved and refused.
- Overlay staging/archive/import inventory carries the helper/unit and checks both executables are x86-64 ELF. CI adds disposable native creation/reopen/rejection fixtures and installs the helper only in the CI runner for static unit validation. Host packaging does not ship the initializer.
- Local unprivileged initializer refusal, guest mount tests, overlay/import tests and Linux cross compilation passed. New native fixture results are pending. Blank-disk formatting and durable crash-between-create/chown recovery remain incomplete, alongside full rootfs/kernel image assembly and physical acceptance.

## Resume owned guest object creation

- Added a root-owned protected intent, synced with the mount parent before object leaf creation. Reads are bounded/no-follow/nonblocking and require exact bytes, mode/owner and one link. An admitted intent permits completing only an empty root-owned private leaf; unmarked, nonempty or corrupt states remain preserved and refused.
- The root helper now includes CAP_DAC_READ_SEARCH to inspect previously initialized mode 0700 guest-owned directories; it still has no raw device access. CAP_CHOWN remains restricted to its creation workflow.
- Added native fixtures for interrupted creation replay, nonempty incomplete-directory preservation and corrupt intent refusal. Local unprivileged refusal tests passed and Linux tests cross-compiled; native and actual unit capability results remain pending. Partial intent publication and physical power-loss qualification are not complete.

## Restricted initializer capability fixture

- Full Linux run `36669688520` passed initial directory creation/reopen/rejection fixtures, both guest ELF overlays, unit syntax, package/browser and root/native checks. It predates durable-intent recovery and restricted-capability execution.
- CI now runs the native guest initializer fixtures with only CHOWN and DAC_READ_SEARCH in effective/permitted/bounding sets, cleared inheritable/ambient capabilities and no-new-privileges. The test checks actual `/proc/self/status` evidence before exercising the workflow. Fixture cleanup restores only its own disposable directories after preservation assertions.
- Local unprivileged tests passed. Native durable-intent and restricted-capability results remain pending; actual systemd namespace/device and power-loss acceptance are not established by this fixture.

## Guest intent admission and capability fixture correction

- Linux `36670164196` passed durable initializer replay with unrestricted root but failed restricted fixture setup: the test tried chmod after chown, requiring a capability intentionally absent from the service. Fixture setup now applies mode before transferring ownership. Service limits are unchanged; corrected native evidence is pending.
- Intent creation now checks its newly created descriptor's root ownership/link count/mode and short writes. Admission also rejects special permission bits. Added native preservation/refusal cases for symlink, hardlink, FIFO, public, foreign and special-mode intent inputs.
- Local unprivileged tests passed and Linux tests cross-compiled. Full native/capability/unit validation remains pending, with partial intent publication, physical power-loss and complete guest image boot still incomplete.

## Publish complete guest initialization intent

- Full Linux run `36670377488` passed corrected restricted-capability initialization, unsafe intent refusal, durable directory replay, native/root/package/browser checks and guest overlays.
- Intent creation now writes/fsyncs a new private staging file, uses Linux RENAME_NOREPLACE publication and syncs its parent before object creation. Existing active intent cannot be overwritten; incomplete staging is preserved and does not prevent a fresh retry within bounded limits. Enumeration is batched and limited to 1,024 root entries and fewer than 64 retained staging records.
- Added native interrupted-staging retry/preservation, occupied publication refusal and staging bound fixtures. Local unprivileged tests and Linux test cross compilation passed; new native results are pending. Metadata cleanup/diagnostics, physical power-loss and blank-disk/image boot qualification remain incomplete.

## Initializer source service protection fixture

- Full Linux `36670656479` passed complete-intent publication/retry/refusal, restricted capability evidence, native/root/package/browser workflows and real guest overlays. This is not guest boot or physical crash evidence.
- Added an opt-in systemd fixture taking reviewed service protection properties directly from the initializer unit. It executes compiled native directory tests with their data in a fresh ordinary `/data` fixture, checks capabilities and PrivateTmp isolation, refuses any pre-existing `/data` and removes only its owned empty fixture directory. ExecStart/RemainAfterExit are adapted for the test harness; mount dependencies are deliberately outside this test's scope.
- Local initializer tests and Python compilation passed. Native source-protection results are pending; full rootfs/kernel image assembly, formatting, actual data/device mounts and physical power-loss qualification remain incomplete.

## Observe initializer system-path write protection

- Extended the source-unit fixture to require observed EROFS on exclusive creation in a root-owned system directory while its object initialization tests write under `/data`. Permission-only errors do not pass; an unexpectedly created owned probe is removed before failure.
- Local initializer tests passed and the Linux test binary cross-compiled. Systemd run `36671154395` remains active and predates this additional assertion. Actual guest disk/device boot and full image assembly remain unfinished.

## Source-unit special-mode assertion correction

- Linux `36671154395` passed ordinary/root and restricted-capability fixtures but the source-unit fixture failed while creating a setuid test input: RestrictSUIDSGID correctly returned EPERM. The source-unit variant now requires that kernel refusal and unchanged mode; separate native variants still create the unsafe input and test initializer admission refusal. Service protections remain unchanged.
- The new EROFS write-boundary assertion is submitted with this correction. Local tests and Linux cross compilation passed; full native source-protection results remain pending. Actual guest boot/device and physical acceptance are not established.

## Observe initializer address-family restriction

- Added source-unit runtime assertions that AF_UNIX creation succeeds while AF_INET/AF_INET6 stream sockets are denied, accepting only explicit unsupported-family/permission refusal. No sockets are bound and no network connections occur.
- Local initializer tests passed and Linux tests cross-compiled. Systemd CI `36671362054` remains active and predates this assertion. Guest adapter/model network policy and actual VM no-NIC isolation are not established by initializer socket tests.

## Initialize fresh supervisor data volumes

- Full Linux run `36671362054` passed source-unit protections including readonly paths and special-mode refusal, restricted capabilities, native/root/package/browser checks and real guest overlays. The socket-family assertion commit is now submitted with this functional change.
- Linux volume preparation now exclusively creates private staging, preallocates its descriptor, formats only that new inode with fixed bounded mkfs.ext4 execution, syncs and publishes without replacement. Existing files are never formatted, mounted or parsed as filesystems; owner/mode/link/size admission is strengthened. Reserve policy is passed from the manager and checked before fresh allocation. Failed staging is retained.
- Added explicit e2fsprogs dependency and an opt-in native fixture for fresh ext4 magic/metadata, existing-byte preservation, symlink/hardlink refusal and cancellation. Local supervisor tests and Linux test cross compilation passed; native formatting results remain pending.
- Failed preparation reclaim, complete volume provenance and restore activation, legacy blank-data migration, production-size formatting performance, maintained rootfs/kernel assembly and actual guest boot/mount/isolation qualification remain incomplete.

## Metadata-only volume admission and reserve refusal

- Full Linux `36672293927` passed actual small-file ext4 initialization and preservation checks, source-unit address-family restrictions, protected guest setup, native/root/package/browser workflows and real overlays. This does not qualify production-size formatting or guest boot/mount behavior.
- Existing volumes now use O_PATH/no-follow metadata admission instead of opening contents/devices. Special permission bits are rejected and cancelled preparation returns its cancellation error.
- Added native FIFO/directory/mode/size preservation and unavailable-reserve-before-staging cases. Local supervisor tests and Linux test cross compilation passed; new native evidence is pending. Recovery/reclaim/provenance, full bootable guest artifacts and hardware acceptance remain incomplete.

## Reclaim abandoned volume preparation under root authorization

- Full Linux `36672568648` passed metadata-only volume admission, native special-file/mode/size and reserve refusal, formatting, source-unit/root/package/browser checks and real guest overlays.
- Added bounded complete-set staging cleanup for root-admitted preparation/retry and stopped-video purge. Only exact scoped nonce names, private root-owned single-link regular files of zero or expected size qualify; published volumes and other resources are excluded. Invalid metadata/names stop cleanup before deletion, and the parent is synced before success.
- Root purge now leaves its durable operation pending and avoids final-volume deletion if staging cleanup fails. Added portable failure/replay evidence and native scoped preservation, complete-set refusal and abandoned-preparation-to-fresh-format fixtures.
- All local Go packages passed; Linux supervisor tests cross-compiled. New native cleanup results remain pending. Complete volume provenance/restore activation, maintained bootable guest artifacts, production-size measurements and physical boot/isolation/power-loss acceptance remain incomplete.

## Bind volume publication to the prepared inode

- Added final metadata re-admission and same-inode checking between the formatter descriptor and staged pathname before non-replacing publication. No existing data content is read for this check.
- Added native occupied-destination preservation and swapped-staging refusal fixtures. Local supervisor tests and Linux test cross compilation passed; native publication evidence is pending.
- Run `36673378456` passed native staging cleanup/retry/formatting and source-unit protections so far, while the full run remains active. This publication change stays committed locally until that run completes. Full volume provenance, bootable guest artifacts and hardware acceptance remain unfinished.

## Supervisor source-unit volume and secret-path fixture

- Full Linux `36673645975` passed same-inode publication, occupied destination preservation, staging swap refusal, formatter/recovery, guest source-unit, root/package/browser and real-overlay checks.
- Added opt-in native supervisor source-unit execution of volume formatting/cleanup/publication under its actual reviewed protection settings and write paths. Synthetic controller/TLS markers test InaccessiblePaths; runtime assertions cover capability sets, no-new-privileges, private tmp, readonly paths, socket families and cgroup memory/pid bounds. Existing fixture leaf paths are refused; only generated markers/owned empty directories are removed.
- Local supervisor tests, Python source compilation and Linux test cross compilation passed; actual supervisor source-protection results are pending. Runtime/state management and restart/dependency/entrypoint lifecycle are adapted or excluded, and filesystem marker hiding does not prove total process-memory isolation. Maintained bootable images, production performance and physical acceptance remain incomplete.

## Bounded guest operating-system writable paths

- Full Linux CI `36674546120` passed native volume formatting/publication/reclaim under the supervisor source service protections, guest source protections, package/browser workflows and overlay pipelines. It does not execute production supervisor RPC/VM lifecycle or prove memory isolation of controller secrets.
- Added guest-only `/tmp` and `/var` tmpfs mount inputs with separate 64 MiB/8192-inode ceilings, root ownership and nodev/nosuid/noexec. Adapter startup requires both units; all three overlays carry them through manifests, packaging and strict imports. CI includes mount-unit syntax validation.
- Bootable rootfs/kernel/bootloader assembly, ephemeral directory population, generator reconciliation, actual mount/boot/resource-pressure behavior and physical acceptance remain incomplete. These sources do not change mounts on the owner's host.

## Read-only root mountpoint inventory

- Added empty mode-0755 `/data`, `/tmp` and `/var` directories to each guest overlay, so mounting does not require creating paths on a read-only system disk. Directory inventory now comes from one shared function used by verifier, packager and strict archive importer.
- Local tests passed for packaging/import round trips and refusal of missing, symlinked, writable or populated mountpoints. These tests use fixture binaries; Linux CI remains responsible for real compiled artifacts and unit syntax. Run `36675536359` is still active for the preceding mount-unit change.
- This provides assembly inputs only. Maintained base-root merging, distribution ephemeral state initialization, kernel/bootloader assembly and boot/hardware qualification remain unfinished.
- Subsequently, full Linux CI `36675536359` succeeded for `e5ba790`, including real three-profile overlay packaging/import and tmpfs unit syntax checks. It predates the mountpoint inventory change and is not boot or memory-pressure evidence.

## Guest volatile state and startup ordering

- Added guest-only tmpfiles rules for private-temp prerequisites and minimal `/var/log` and `/var/lib` directories. Both initializer and adapter require the distribution's tmpfiles setup before startup; all overlays include the rules and volatile journal configuration.
- Guest journald configuration selects runtime-only storage, retention/rotation targets, rate limiting and disables forwarding. These are journal targets, not hard byte guarantees; `/run` capacity and actual daemon behavior still need booted-image validation.
- Local overlay/import tests passed; native directory integration is skipped on macOS. Added explicit disposable Linux/root CI test running real systemd-tmpfiles in a fresh fixture root twice, checking exact numeric ownership/modes and preservation of temporary/workload bytes. Unit dependencies, journal daemon behavior, maintained base-root assembly and actual boot/resource qualification remain pending.
- Full Linux CI `36675800911` succeeded for `f51a6bf`, validating real overlay mountpoint inventories, archive round trips and source-unit syntax alongside root/package/browser workflows. It predates the volatile-state sources and does not prove a bootable image.

## Adapter admission of bounded OS temporary state

- Full Linux CI `36676096884` passed volatile tmpfiles creation/replay/preservation, real overlays, source-unit syntax and native/root/package/browser workflows for `44b2176`. It does not execute the journal daemon or prove guest boot.
- Production adapter startup now checks the actual opened `/tmp` and `/var` filesystem metadata, protection flags, root ownership/modes, separate tmpfs devices and bounded byte/inode counts. Mount IDs bind visible paths to kernel mountinfo; private-temp binds are supported and read-only `/var` remains compatible with the adapter's strict system sandbox.
- Local parser/command tests passed and Linux test binaries cross-compiled. Added a disposable private-mount-namespace CI fixture using actual source mounts and checker refusal of unsafe modes, overlarge limits, missing flags and read-only `/tmp`, plus private-temp/read-only-var views. Native results remain pending; actual guest boot/source-unit startup, maintained rootfs/kernel assembly and physical resource acceptance remain unfinished.

## Merged guest root admission and profile enablement

- Full Linux CI `36676721653` succeeded for `c73ea56`, including actual private-namespace tmpfs mounts, unsafe remount refusal and private-temp/read-only-var admission. This is not guest boot or execution under the complete adapter source unit.
- Added root-finalization phase admitting a separately verified overlay's merged bytes, protected ownership/modes/parents, mountpoint directories and local guest identity before creating known mount/profile enablement links. Foreign links and another profile's normal multi-user enablement are preserved and refused; matching links replay.
- Local tests passed for all three profile/replay paths and preservation on tampered payload/account, conflicting profile/link and symlinked parent. CI now runs these assembly-phase tests. Fixtures contain synthetic ELF headers and account files; they do not execute a real distro/kernel/bootloader.
- Inspected pinned upstream mkosi v25 (`52448a27f6f869c108352ed82fcfb9be633703fb`) BIOS GRUB support against the current q35 BIOS domain contract. A complete maintained image recipe, independently authenticated base packages, current tool vulnerability coverage, complete startup inventory, actual image builds/boot and hardware qualification remain unfinished.

## Development BIOS guest disk assembly pipeline

- Full Linux CI `36725365653` succeeded for `bb9e007`, including merged-root fixture admission/replay and existing native/package/browser workflows. These fixture roots are not distribution images.
- Added explicit disposable Linux/root development build wrapper: independently digest-checked overlay import, unchanged root-owned pinned mkosi source admission, exclusive staging, reviewed helper/recipe copy, bounded build timeout and output SHA256/size/source metadata. Failed staging is retained; no physical disks or host services are changed.
- Added Ubuntu noble BIOS/EFI GRUB/kernel/initramfs recipe, read-only 2–4 GiB ext4 root, ESP/BIOS partitions, locked root/no autologin and validated profile finalization. Files/video development builds run in a separate disposable CI workflow; AI remains refused pending its actual reviewed model/runtime pipeline.
- Local assembly tests and syntax checks passed. Actual Linux recipe parsing/build results are pending; mkosi cannot run on the current macOS host (`unshare` is unavailable). Maintained repository signature verification/recorded packages are not a pinned reproducible package closure. Outputs explicitly remain unsigned, unboot-validated and release-unqualified; boot/runtime/license/source promotion, tool security coverage and hardware gates remain unfinished.

## Development guest boot and object fixture

- Full `HomeNode checks` run `36726533869` passed for `aefba9d`. Both image-build attempts of `36726533904` failed before image assembly, with terminal logs ending during setup/staging and only generic exit-code annotations. No concrete build cause or image artifact is established. Split staging and packaging into distinct steps for clearer next-run diagnosis; do not treat these failures as boot evidence.
- Added explicit unprivileged Linux TCG/q35 VM fixture with read-only system input descriptor, newly formatted small data file, virtio RNG/serial and no NIC/monitor/display. It binds a bounded image digest record and expects actual adapter health plus upload/finalize/download/content verification/delete acknowledgement, with bounded console retention and separate success evidence.
- Local framing tests passed, including fragmented responses and refusal of wrong identity/version, duplicate/unknown fields, guest errors and oversized frame headers. They do not boot or emulate the adapter. Actual image build/boot results remain pending; AI/model assembly, package closure/provenance/licenses, production lifecycle/security/performance and physical hardware gates remain unfinished.

## Development initramfs repository admission

- Direct terminal job-log retrieval established the actual failure in files job `109931851788` of run `36728594248`: mkosi's default initramfs package installation could not locate `erofs-utils` with only main enabled. Pinned upstream Ubuntu installer source confirmed additional components come from `Repositories=`.
- Added universe to the development recipe with repository signature checks unchanged. FFmpeg also needs that component. This is a development assembly correction, not release maintenance approval; complete system/initramfs package support, security response and license/pinned-closure gates remain unfinished.
- Direct logs also revealed the same concrete `erofs-utils` failure in earlier retry job `109926123534` of `36726533904`, correcting the preceding provisional interpretation based on incomplete high-level logs. The unavailable package—not a proven runner setup defect—was the observed build failure there. Actual disk assembly/boot results remain pending after this correction.

## Image formatter prerequisites and explicit profile selection

- Actual files job `109936960666` in `36729972078` passed account/payload finalization, ext4 population and initramfs creation, then failed ESP formatting because `mcopy` was absent. Added `mtools` to the disposable runner's build dependencies.
- Actual video job `109936961250` reached finalization and was correctly refused: distribution presets enabled the files instance from the shared template's `DefaultInstance=files`. Removed that default, preserving the gate and reserving profile selection for explicit image finalization.
- Logs also showed duplicate conventional finalize invocations/partition definition paths. Removed their redundant explicit recipe entries so mkosi's named-file discovery supplies them once.
- Local overlay/finalization/protocol tests passed. Complete disk/boot results after these corrections remain pending; package support/provenance/licenses, AI runtime/model assembly, production VM lifecycle and physical qualification remain unfinished.
- Pinned mkosi source also requires host `grub-mkimage`/`grub-bios-setup` for BIOS assembly. Declared `grub-pc-bin` explicitly in runner dependencies rather than relying on incidental hosted-image tools; this is source-derived prerequisite coverage, not a newly observed GRUB failure.

## EFI partition capacity correction

- Video job `109941532816` of `36731363492` passed the selected-profile/root/initramfs stages and reached vfat population with `mcopy` available, then failed with `Disk full` at the original 256 MiB ESP. No boot or published disk is established.
- Changed the development ESP to 512 MiB, matching the inspected pinned upstream default partition definition. The overall candidate still stays within the existing 8 GiB system-image bound; package/kernel size closure remains a release qualification gate.
- Local overlay/finalization/protocol tests passed and partition-source whitespace checks passed. Actual formatting/BIOS installation, booted transfer, production confinement/lifecycle, AI runtime/model and physical acceptance remain unfinished.

## Verified development guest boot and pending conversion

- Full image run `36732215628` and main checks `36732216243` succeeded for `0400e37`. Downloaded and inspected both small boot-evidence artifacts: files SHA256 `ef6d79e38b461978cae8e06c0376a5e2251c498b7f7a37be7a0341f7f0f5455f`; video SHA256 `ca385aa0923d4c246c3e8da58351f50959c2f11a074930165ac56c1b4c566680`. Each proves TCG boot, actual adapter readiness and a small object upload/finalize/download/delete-ack round trip with releaseQualified false.
- Extended the video boot fixture to generate test-only source media, execute both approved presets inside the guest, wait for durable results, verify bounded transferred output bytes/digests and H.264/dimensions using a resource-limited disposable unprivileged CI probe. Production host roles do not parse video payloads. CI now declares the host fixture's FFmpeg tool explicitly.
- Local protocol tests passed, including strict numeric/text response fields. New booted conversion results are pending; audio/codec breadth, cancellation/reboot, quality/performance, controller/supervisor/UI lifecycle, AI model/runtime, release provenance/maintenance/licenses and physical security/availability gates remain unfinished.

## Booted video preset evidence and multi-chunk transfer gate

- Full image run `36735256003` and main checks `36735255614` succeeded for `0f368fc`. Inspected small boot-evidence artifacts `11107272222` (video) and `11106768174` (files), rather than relying solely on workflow status. Files image SHA256 is `264c247c3631250131b78e6046b8a7dd6ffd7e0f71a9c423b38d1279193918f7`; video is `4910f94544f194ddd63b2b4135401b618b531178b23309b9bbe11dba4caac159`.
- Actual video adapter conversion produced H.264 1280×720 output (1748 bytes) and 1920×1080 output (1955 bytes). This uses generated one-frame black media and verifies transferred digests, codec and dimensions. It does not establish audio/codec breadth, sustained performance, cancellation, production supervisor confinement or controller/UI lifecycle.
- Extended both boot profiles' object fixture to transfer 1,048,579 bytes in five chunks, replay an acknowledged chunk, verify each download offset/digest plus complete content, then request deletion. Added portable tests rejecting wrong replay progress, finalization digest, empty chunks, wrong offsets and wrong chunk digests. All five protocol/transfer tests pass locally; actual multi-chunk boot results remain pending.
- AI runtime/model packaging, production networking/install/update/backup/restore lifecycles, independently reviewed release trust and physical reference-host acceptance remain required for the complete application.

## Cancellation retains guest execution and deletion admission

- Inspection found that cancellation changes durable task state before its worker finishes teardown. Admission previously checked only `running`, allowing another task to overlap or deletion of a still-used input/task while the cancelled worker remained alive.
- Added an in-memory active-worker flag, held through teardown and the worker's final locked cleanup. Both execution admission and input/task deletion now consider active workers even when their visible state is cancelled. The flag is not serialized: restart recovery does not resurrect execution.
- A deterministic regression test blocks a cancelled inference transport during teardown, checks input/task deletion and overlapping execution refusal, then releases/joins it and verifies cancellation remains visible, deletion succeeds and execution admission reopens. `go test -race ./internal/guest` and `go test ./...` passed locally. This is adapter-level lifecycle coverage; actual model-server cancellation, video subprocess teardown, booted cancellation and controller/supervisor release gates still need verification.

## Development boot cancellation fixture

- Extended the video boot fixture to run/cancel a conversion through virtio, require cancelled state, wait up to 30 seconds for task deletion with only explicit `OBJECT_BUSY` tolerated, require subsequent `NOT_FOUND`, and verify adapter health. Both preset conversions follow cancellation, exercising reopened execution admission with the preserved input.
- Generated source media now contains 30 black frames, still within the fixed 256 KiB input bound. Cancellation is requested immediately; this does not prove FFmpeg was already executing, sustained cancellation performance or controller/supervisor/VM-stop behavior.
- Portable tests require the cancellation/deletion/health sequence, reject incorrect terminal states, failed disappearance and teardown timeout, and reject unapproved error codes. Local guest packaging/protocol tests passed (20 tests, two native-only skips). Actual booted cancellation and multi-chunk results remain pending. Superseded runs `36736474547` and `36736474567` were cancelled, not validated.

## Pinned development CPU inference compilation

- Retrieved official llama.cpp v0.5.0 source and inspected its CMake/server/UI options. The tag peels to `7fe450e19305b828c199d602c23a8337aaa1f03b`, different from release API `target_commitish`; the build pins the actual checked-out commit rather than assuming that API field identifies tag bytes.
- Added an explicit unprivileged disposable Linux x86-64 compiler wrapper and dedicated workflow. It refuses dirty/wrong-revision source, uses exclusive protected output, fixes build options/environment, limits two compile workers and build time, checks ELF/size/ownership, executes version reporting and retains binary SHA256/options plus upstream license. No model acquisition, host service installation or release qualification is performed.
- Source-reviewed options disable native/AVX tuning, GPU/RPC/dynamic ggml backends, subprocess support, OpenSSL and downloaded UI assets. They are candidate build settings, not proof of complete endpoint removal, guest confinement, dependency support or efficient inference. Local Python AST parsing passed; local YAML parser is unavailable, and actual Linux compilation is pending.
- Model integrity/license/profile admission, model-server sandbox/arguments and image integration, third-party notices/SBOM/security maintenance, booted inference/cancellation and physical memory/CPU benchmarks remain required.

## Compiler configuration admission

- Added bounded, no-follow, single-link/protected-owner CMake cache admission before compilation and an unchanged-cache check afterward. Every required setting must exist once with its recognized BOOL/STRING type and exact value; an unrecognized `UNINITIALIZED` option now fails instead of silently supporting a configuration claim. The runtime record includes the admitted cache digest.
- Portable tests cover ignored/re-enabled/missing/duplicate options, downloaded-UI re-enablement, NUL/oversize input, symlink/hard-link and writable configuration refusal. Local packaging/protocol tests passed (23 tests, two native-only skips). Actual pinned compiler run `36738027369` remains live in its compile step; no executable or AI inference is yet established.
- Main checks `36738027489` succeeded for `1b7b1a3`. Superseded image run `36737346762` was cancelled; cancellation and multi-chunk boot results remain pending in the current image workflow.

## Inference binary evidence and runtime dependency gate

- AI runtime run `36738027369` succeeded for `1b7b1a3`. Downloaded artifact `11109156250` (5,540,502 compressed bytes), inspected its development record/ELF metadata and independently hashed the 14,530,880-byte executable: SHA256 `1f8b8183270b7ea94a223a7f1c8e1e1f0026e2072f843770848944e9309d1fa9`. This establishes pinned Linux compilation/version execution, not model inference, the subsequently added CMake cache gate or guest execution.
- Added readelf dependency admission: expected x86-64 glibc loader; allowlisted distro C/C++/math/OpenMP/GCC libraries; refusal of foreign/duplicate/malformed dependencies, RPATH/RUNPATH and loader audit/filter additions. The development manifest records actual library names and interpreter. OpenMP/fetch/KleidiAI choices are now explicit cache-checked settings. Distro package/version closure and symbol compatibility still need image integration and release review.
- Local packaging/protocol tests passed (25 tests, two native-only skips), including loader/dependency denial cases. Actual new dependency-gated compilation is pending.
- Changed long AI/image workflow concurrency to preserve active runs while queuing the newest revision. Repeated pushes had cancelled preceding boot runs, including `36738027493`, before usable evidence. This retains bounded workflow timeouts; no old run is treated as proof of a newer commit.

## Pinned development instruct model acquisition

- Inspected the quantizer's published model card and Hugging Face repository/LFS metadata for `bartowski/SmolLM2-135M-Instruct-GGUF`. Added an explicit development-only Q8_0 candidate pinned to revision `09816acd5d99df7be770d85ea30822623dab342c`, 144,811,360 bytes and SHA256 `5a1395716f7913741cc51d98581b9b1228d80987a9f7d3664106742eb06bba83`. The card identifies the base model and Apache-2.0; this is publisher metadata, not completed redistribution/license/source-provenance review or a production SKU decision.
- Added explicit unprivileged disposable Linux acquisition with private exclusive staging, full preallocation, HTTPS-only bounded redirects to HF domains, disabled environment proxies, identity encoding, bounded streamed size/digest checks and private publication only after integrity succeeds. Failed partials remain unpublished. No weights are parsed or executed by the host downloader, and no ordinary workload gains internet access.
- Dedicated runtime CI now fetches/retains this separate unqualified candidate after compilation. Portable tests reject short/long/digest-mismatched transfer, timeout and unsafe redirects; local suite passed (28 tests, two native-only skips). Actual download, offline runtime inference and guest boot integration are pending. Model notices/license review, upstream conversion authenticity, model memory/quality/security benchmarks and production controlled-download integration remain required.

## Explicit glibc loader dependency admission

- Configuration-gated runtime run `36738673273` succeeded for `dfc830d`. New dependency-gated run `36739123329` compiled completely but failed its dependency gate; no model fetch or inference is established by that run.
- Independently inspected the earlier downloaded candidate's ELF dynamic table and found the five expected runtime libraries plus a direct `ld-linux-x86-64.so.2` dependency. Added that exact glibc loader basename to the allowlist; the interpreter still must be exactly `/lib64/ld-linux-x86-64.so.2`, and foreign loader names/paths remain refused. This corrects the incomplete dependency inventory without allowing arbitrary libraries.
- Portable tests include foreign loader dependency refusal. Actual dependency-gated compile/download results after this correction remain pending, along with offline inference and guest image integration.

## Isolated development streaming inference fixture

- Added a disposable Linux fixture admitting pinned runtime/model descriptors, then launching the actual compiled server under address-space/CPU/process/file/core limits with fixed loopback/offline/CPU-only arguments. It uses a 512-token context and requests 16 output tokens solely for compatibility testing; these are not a qualified production profile.
- CI creates a separate network namespace, enables only its loopback device and drops UID/GID/groups/capabilities before the fixture runs. The fixture independently checks namespace separation, loopback-only device inventory and zero effective capabilities. Root setup operates only in that disposable namespace, with no owner-host network change.
- The fixture requires actual health, bounded chat SSE data with text, an approved finish reason and DONE marker, then records only digests/counts and unqualified success evidence. The process is joined/killed in cleanup; generated text is not an artifact. Local tests reject partial/empty/oversized/duplicate/invalid streams; complete portable suite passed (30 tests, two native-only skips).
- Actual model download and isolated inference results remain pending in Linux CI; actual guest service/image integration, adapter-level inference/cancellation, system budgets/benchmarks and release/license/provenance gates remain unfinished. Run `36739704797` predates the glibc loader allowlist correction and failed the same dependency gate.

## Verified model acquisition, files transfer and notice retention

- Runtime run `36740320461` succeeded for `be55de6`. Inspected direct job `109973415925` logs: cache/dependency-gated binary SHA256 remains `1f8b8183270b7ea94a223a7f1c8e1e1f0026e2072f843770848944e9309d1fa9`, with the six admitted loader/libraries; actual model acquisition passed exact size/SHA verification and emitted its pinned development record. This predates isolated inference and is not evidence of model execution.
- Files job `109970370761` in image run `36739123581` succeeded for `99a8138`. Downloaded its bounded boot record: system SHA256 `18cbe70d505ac9fff0e07fa6bd8a9c99a703372c33d0ee1bbf1332b7b0c23a1f`; actual adapter transfer of 1,048,579 bytes in 262,144-byte chunks plus acknowledged replay succeeded. The video job remains live; its completion is not inferred from the files result.
- Added exact-byte retention and hashed inventory of the pinned source's main license plus available JSON, HTTP and hashing-library notice files. Inputs are bounded protected single-link regular files; missing/symlinked notices fail candidate publication. Tested staging all six actual files from the admitted local source checkout as well as portable refusal/preservation fixtures. Full local suite passed (32 tests, two native-only skips).
- The inventory is source-notice retention, not a complete dependency/license review: embedded source notices, distro runtime notices, model notice/source chain, SBOM and redistribution/security-maintenance approval remain required. The runtime record explicitly marks licenseReviewComplete false. New notice-bearing compile and isolated inference results remain pending.

## Development model service and admitted assembly input

- Added a separate guest-only development model service using a dynamic identity, fixed loopback/offline/CPU arguments, private devices/temp, read-only system protection, empty capabilities, inaccessible app data/adapter port and bounded memory/tasks/files. Added an AI-specific adapter dependency binding to model-service lifecycle. Neither source is installed on the owner host or enabled in existing files/video disks.
- Added explicit root/disposable-Linux assembly of four known AI payload files with exclusive staging and exact runtime/model size/digest admission. The binary is pinned to the observed `1f8b818...` candidate rather than accepting an arbitrary self-declared artifact digest. The receipt is written only after all inputs copy/hash/sync successfully; mismatches leave unqualified diagnostic staging without a valid receipt. Units use the current reviewed repository bytes. These inputs still require independent source-root finalization, boot/unit/identity and complete image verification before use.
- Local tests cover exact copy/mode preservation, overwrite refusal, digest/size mismatch, symlink/hard-link and writable-input denial. Full local suite passed (34 tests, two native-only skips). Actual assembly remains behind the inference CI gate, and its service limits/context are development values awaiting measured qualification.
- Isolated run `36740956547` for `5c6eb1c` failed with ValueError before readiness; the available log does not establish a concrete cause. Added fixed policy-error detail and a kernel-file-size-limited 64 KiB startup diagnostic artifact, retained on failure as well as success. This fixture uses only generated test prompts. No inference success, guest image execution or production isolation claim is made; the next actual run must establish the cause and result.

## Revalidation of staged AI assembly inputs

- Added separate verification of the AI input tree and receipt before successful staging returns: exact known file/directory inventories, owned protected directories, no links/special entries, single-link exact file modes/sizes, independent runtime/model/unit digests and strict receipt identity/types. Later image integration can revalidate these inputs rather than relying only on the original copy operation.
- Portable fixture tests cover positive admission plus foreign entries, modified bytes, writable files/directories, altered inventory, boolean/numeric flag confusion, floating-point modes and symlink refusal. These fixtures patch binary/model sizes/digests and do not execute real model code or prove guest image admission. Local suite passed (35 tests, two native-only skips).
- Actual runtime run `36742589542` remains live; inference startup diagnosis, full root merge/finalization, native model-service startup and booted inference/cancellation remain unfinished. No result is inferred from elapsed time or the verifier alone.

## Initial assistant stream event compatibility

- Terminal diagnostic run `36742589542` failed with `invalid inference continuation`, after health and streaming request startup. Its bounded server diagnostic contains initialization/CPU-only GPU warnings, not a model-startup error. The fixture's stream parser refused null content; pinned upstream `server_task_result_cmpl_partial::to_json_oaicompat_chat` explicitly emits an initial assistant-role event with null content.
- Accepted that exact initial role/null event at most once, before text and completion. Tool roles, role-less null content, repeated initial events and existing malformed/partial/oversized stream cases remain refused. Added the same initial event to the Go adapter's bounded/persisted chat regression test; adapter compatibility passes without changing production parsing.
- Full portable suite passed (36 tests, two native-only skips); `go test -race ./internal/guest` passed. Actual end-to-end isolated inference after this compatibility correction is pending. Prior partial readiness/stream startup is not promoted to completed inference or guest boot qualification.

## Verified isolated inference and AI disk integration

- Runtime run `36743572716` succeeded for `3ebac10`. Downloaded/inspected its bounded inference record: runtime SHA256 `1f8b8183270b7ea94a223a7f1c8e1e1f0026e2072f843770848944e9309d1fa9`, model SHA256 `5a1395716f7913741cc51d98581b9b1228d80987a9f7d3664106742eb06bba83`, completed streaming response of 70 UTF-8 bytes under the 512-context/16-requested-token disposable profile, releaseQualified false. This establishes isolated loopback runtime compatibility, not quality/performance, the model systemd sandbox or VM/adapter inference.
- Development disk assembly now accepts AI only with a separately verified pinned AI tree. It revalidates the copied tree, declares explicit distro runtime libraries, supplies reviewed helper/unit sources and merges the additional tree through mkosi. Root finalization verifies exact merged model/runtime/unit/receipt bytes and enables both AI/model units only after admission. Bare AI adapter roots and model inputs for another profile are refused.
- Added synthetic merged-root fixtures for AI enablement/replay and tampered merged model refusal, retaining existing files/video/conflict tests. These patch binary/model digests and are not real images. Local suite passed (41 tests, two native-only skips), including bounded AI generation/terminal-text/delete checks.
- Dedicated CI now builds/retains an unsigned AI disk after successful isolated inference and payload admission, then boots a no-NIC q35/TCG VM and exercises actual adapter health, multi-chunk objects and bounded generation/deletion. The AI fixture requests 1536 MiB/two virtual CPUs and waits for explicit starting-to-ready health; these are development test resources, not a supported host/model envelope. Actual build/source-unit startup/booted inference remain pending. Physical KVM/AppArmor/resource/performance, cancellation/reboot, signed release/package/model provenance/licenses, networking/install and full product lifecycle gates remain unfinished.

## Interrupted task metadata deletion

- Inspection found that typed guest deletion omitted the `.task.tmp` file used by atomic journal writes. Interrupted/failed writes could therefore leave object-associated metadata after deletion acknowledgement, including generated AI text if present in that journal.
- Deletion now removes that known temporary suffix alongside partial/object/committed-journal/working files before directory sync and acknowledgement. Existing active-worker deletion protection still applies, and object ID validation scopes all suffixes.
- Added a filesystem regression verifying removal of every known associated file, idempotent repeat deletion and preservation of another object's temporary journal. `go test -race ./internal/guest` and `go test ./...` passed locally. This establishes logical unlink/ack behavior, not physical byte erasure or an end-to-end controller/backup retention claim.
- AI image run `36745173882` remains live for `d90a10e`; no booted AI result is yet established. The full application goal, production lifecycle and hardware/security qualification remain unfinished.

## Boot verification before large artifact upload

- Direct terminal logs for video job `109970371238` in run `36739123581` established that it ended with cancellation at its 35-minute limit. Dependency installation consumed about 31 minutes; actual disk assembly reached finalize/build, then cancellation occurred during the multi-gigabyte artifact upload before boot testing. The stale high-level in-progress view was not evidence that this job was still running.
- Moved files/video and AI boot checks plus bounded diagnostic retention before large disk upload, preserving the existing unsigned disk/package artifact step after successful verification. Increased files/video workflow timeout to 60 minutes to accommodate the observed slow dependency phase while retaining a finite limit. No assembly, boot, conversion or inference checks were removed.
- Both changed workflow files parse successfully with the available local Ruby YAML parser, and whitespace checks pass. Actual latest boot/build results remain pending; AI run `36745173882` was revalidated live in disk assembly. Runtime/source-unit confinement, controller lifecycle and physical release gates remain unfinished.

## Verified development AI guest and video cancellation

- AI run `36745173882` succeeded for `d90a10e`. Downloaded and inspected its bounded boot artifact: system SHA256 `547b5c716bc79bf6331877857a21520b0ff2107089766f98642b577dada02a28`, actual no-NIC q35/TCG boot, 1,048,579-byte multi-chunk object transfer with acknowledged replay, adapter generation of 152 UTF-8 bytes and task deletion acknowledgement. This exercises the assembled guest model/adapter startup and actual inference; it does not establish production sandbox escape resistance, KVM performance, AI cancellation/reboot, model quality or release provenance. The record explicitly remains releaseQualified false.
- Guest-image run `36745813187` succeeded for `c6aa240`. Downloaded and inspected its video boot artifact: system SHA256 `19a018b382570aae373c2d9e924ba983227024b15421496588d30559c2b5af85`, multi-chunk transfer/replay, cancelled task and deletion acknowledgement, then actual H.264 720p (1280x720, 3,248 bytes) and 1080p (1920x1080, 4,364 bytes) conversions of the generated 30-frame fixture. This proves the fixture lifecycle, not sustained owner video workloads or controller/UI cancellation.
- Expanded AI workflow triggers to all guest packaging sources plus go.mod/go.sum. Previously edits to shared staging, overlay admission, service/mount sources or Go dependencies could leave AI disk/boot CI unexecuted despite changing its inputs. Files/video already use this broader dependency scope. Both workflow YAML files parse locally; whitespace checks pass.
- HEAD `dee7b97` main checks passed; its AI job `109993746075` was revalidated live in disposable dependency installation. Older successful revision evidence is kept distinct from the newest run. The complete product goal remains active: action-bound approvals, signed maintained releases, real installation/network workflows, backup/restore lifecycle and physical security/resource validation remain unfinished.

## AI guest cancellation and recovery fixture

- Extended the actual AI boot fixture to start a generated count prompt, require cancellation acknowledgement and persistent cancelled state, wait at most 30 seconds for worker teardown/deletion admission, require NOT_FOUND after deletion and ready adapter health, then complete and delete a separate generation. The emitted AI evidence includes cancellation only after that sequence succeeds. A completed generation cannot masquerade as cancellation.
- Shared bounded teardown checks with the existing video fixture; no production adapter behavior was weakened. Portable regressions cover busy-until-joined deletion, same-task identifiers, completion refusal and prevention of recovery generation when cancellation fails. Local suite passed (44 tests, two native-only skips); actual AI VM cancellation/recovery remains pending CI and is not inferred from the earlier generation-only record. This immediate cancellation fixture does not establish cancellation after sustained token streaming or complete controller/UI/reboot behavior.

## Retain task identity when guest deletion fails

- Guest deletion previously removed the in-memory task even when unlinking an associated path or syncing the directory failed. It now discards the task only after all known suffix cleanup and directory sync succeed. Failure still returns OPERATION_FAILED, and the retained terminal task allows callers to observe the unacknowledged deletion and retry. This is not an atomic rollback of partially removed disk files.
- Added a deterministic nonempty-directory unlink fault fixture, verifying failure is not acknowledged, cancelled task state remains visible, clearing the fault permits deletion retry and successful deletion returns NOT_FOUND. This injected directory is not an admitted journal. Guest race tests and the complete Go suite passed locally. Real power-loss/sync-failure recovery and controller retention workflows remain separate unfinished gates.

## Fail closed on future or expired verification timestamps

- Session freshness previously checked only now-minus-verification less than five minutes, admitting future timestamps after clock rollback and relying on callers to enforce expiry. Fresh now requires a positive nonfuture verification timestamp within five minutes and a session expiry strictly after now. Clock rollback therefore requires renewed verification instead of extending the sensitive-action window.
- Added current/recent, five-minute boundary, old, future, missing verification, expired, expiry-boundary and missing-expiry regressions. Identity/server/control race tests and the complete Go suite passed locally. This is freshness hardening, not implementation of specification section 95 action/resource/body/policy/epoch-bound single-use approvals, which remains required.

## Transactional identity authority snapshot admission

- Identity mutation admission now reads current verification time, expiry, recovery epoch and device capabilities inside the same transaction and requires them to match the authenticated actor snapshot. Previously it checked only that an unrevoked current-epoch session existed; changed capability/verification state or an incorrect supplied epoch could therefore remain unexamined when creating pairing invitations or revoking devices. Missing authority returns ErrDenied; database errors remain distinguishable internally.
- Added pairing refusal regressions for changed persisted verification/expiry/capabilities and mismatched actor epoch, including absence of persisted invitations after denial. Identity/control/server race tests and the complete Go suite passed locally. This snapshot check covers these identity transactions, not all control-plane mutations, and does not replace the required action-bound passkey approvals.

## Require complete AI streams before success

- Production guest inference now requires both an approved stop/length finish reason and a subsequent DONE marker. Previously any non-null finish reason caused immediate success before checking stream completion. Additional data events after finish are refused. Truncated streams, tool/unknown finish reasons and marker-only completion cannot become successful task results.
- Regression streams cover approved stop/length, missing marker, missing finish, tool/unknown finish and content after completion. Existing pinned-server initial assistant/null event compatibility remains intact. Guest race tests and the complete Go suite passed locally; actual booted pinned model compatibility for this revision remains pending CI. This does not yet implement full strict output-event shape validation or action-bound approvals.

## Refuse nontext model deltas

- Production inference now admits only content and the pinned runtime's exact initial assistant-role/null-content event once before text/completion. Tool/function/other delta fields, foreign/repeated/late roles, roleless null content and null deltas fail generation instead of being silently ignored. Empty terminal delta remains compatible with approved finish events. No tool execution authority is added.
- Added tool-delta, foreign-role, roleless-null and null-delta refusal streams alongside existing initial-event and complete-stream regressions. Guest race tests and the full Go suite passed locally; actual pinned-runtime VM compatibility remains pending. Duplicate JSON keys and full outer-event validation still need separate review; these checks are not a complete model-output trust boundary audit.

## Reject ambiguous model event JSON

- Before typed SSE event decoding, production inference now validates UTF-8, recursively rejects duplicate decoded object keys (including escaped equivalents), requires exactly one complete JSON value and limits nesting to 32. The existing scanner bounds individual event bytes and total stream input. This prevents Go's default last-key-wins and invalid-UTF-8 replacement behavior from masking malformed model events.
- Regressions cover normal events, outer/nested/escaped duplicate keys, trailing values, truncation, invalid UTF-8 and excessive nesting. Guest race tests and the complete Go suite passed locally. Full outer-event schema qualification, booted runtime verification, action-bound approvals and production lifecycle/security gates remain unfinished.

## Revalidated AI boot ordering and current verification handle

- Run `36746394847` completed successfully for `dee7b97`. Downloaded and inspected its bounded AI boot evidence: image SHA256 `826d1cd6cfb759a23567161a8652709b071d0a7cb828a26470d2c3341a8fe743`, no-NIC TCG boot, 1,048,579-byte object transfer with replay, generation of 174 UTF-8 bytes and task deletion acknowledgement. The revised boot-before-large-upload workflow completes; its development record remains releaseQualified false. It predates cancellation and stricter model-event parsing, so neither is inferred from this success.
- Run `36747768087` for `d023676` was revalidated with concrete live job `110000070280`, in disposable dependency installation. This revision includes AI cancellation/recovery and failed-deletion task retention, but predates later parser changes. No restart was issued merely because newer runs were pending or observations were slow. Pending runs are not proof of live execution or completed validation.
- Current control/identity/frontend inspection confirms that sensitive operations still rely on a global fresh session and manually repeated login. Specification section 95 requires action/resource/body/policy/expiry/epoch bindings and single-use authorization; the current snapshot and freshness defenses do not satisfy that requirement. This remains an implementation gap, alongside the maintenance/backup/restore, independently trusted release and real installation/network/hardware gates.

## Action approval binding foundation

- Added a bounded canonical approval binding containing action, copied/sorted resource set, exact request-body SHA256, policy generation, recovery epoch, two-minute expiry, device ID and session hash. A domain/schema-separated digest binds every field. Validation refuses unknown actions, duplicate/unsafe/oversized resource sets, invalid identities/digests, nonpositive generations/epochs and expired/overlong authority. Raw body bytes are hashed, so equivalent but changed request bytes require renewed approval.
- Identity race tests passed, including all authority-field digest changes, canonical resource ordering, caller-resource mutation isolation and invalid/expired binding refusal. This internal foundation is explicitly not a grant and is not exposed as an authorization endpoint. Passkey ceremony persistence/verification, single-use transactional grant consumption, all sensitive-route integration, OpenAPI and browser workflow verification remain required before section 95 is satisfied. Existing global freshness remains in place until that full integration is implemented.

## Persist device-scoped approval ceremonies

- Added BeginApproval to create a UV-required passkey ceremony with the exact server-derived action binding. Current actor authority and claimed recovery epoch are revalidated in the insertion transaction; credentials are restricted to the acting device, unlike general owner login. Ceremony and database expiry match the two-minute binding expiry. Expired challenges are pruned, with at most 20 live approval ceremonies per device. The returned token alone is not a grant.
- Identity race tests passed. A synthetic credential fixture verifies the single acting-device allowlist, required UV, exact persisted binding/epoch/expiry and refusal above the per-device cap; it does not sign or verify a real assertion. No HTTP approval endpoint is exposed yet. Finish verification, single-use grants, transactional mutation integration, request-derived resource/body/policy contracts and browser workflows remain unfinished.

## Verify approval assertions before opaque grant issuance

- Added FinishApproval: burns the challenge on every attempt, validates binding/session/device/epoch/policy/expiry and capability, runs the existing UV-required WebAuthn assertion verifier, refuses clone warnings and transactionally revalidates actor plus returned credential ownership. Successful verification updates the credential counter and persists an opaque hashed grant with the original binding expiry; live grants are capped at 20 per device. Ceremonies/grants include issuer identity for existing revocation cleanup. No approval endpoint is exposed and a grant does not yet authorize a route.
- Identity/control race tests and the complete Go suite passed locally. Negative fixtures prove malformed assertions, another session, changed policy, expired bindings and wrong challenge kind cannot issue a grant and cannot reuse the ceremony. These are refusal fixtures, not a valid signed-assertion/browser success test. Positive real assertion verification, single-use transactional grant consumption, mutation/HTTP/UI integration and complete requirement qualification remain unfinished.

## Transactional single-use approval consumption

- Added ConsumeApproval, which revalidates the actor, loads only an opaque approval-grant token, compares all binding fields with authority derived from the actual request/current policy, checks epoch/expiry/capability, and deletes the grant in the same transaction as the supplied durable mutation. Successful commits consume the grant once; failed mutations roll back both state changes and consumption. Callbacks must use the provided transaction and persist intent rather than perform external effects.
- Synthetic persisted-grant regression proves changed body/policy do not invoke the mutation, injected mutation failure rolls back identity state and allows retry, successful retry commits and subsequent replay is denied. Identity race tests and the full Go suite passed. This tests consumption semantics, not cryptographic grant issuance; positive signed assertion/browser verification and actual sensitive-route/OpenAPI/UI integration remain unfinished.

## Cryptographic approval assertion verification

- Added a positive assertion fixture using a freshly generated P-256 private key, COSE public key, actual RP-ID hash/user-presence/user-verification flags/counter, canonical client data with the persisted challenge and a real ES256 ASN.1 signature. The production WebAuthn verifier accepts it and issues a grant preserving the exact approval binding, while persisting the updated signature counter. No verifier mock or preapproved credential result is used.
- The same cryptographic fixture rejects signed assertions from a foreign origin, missing user verification and a corrupted signature, with no grant persisted. Identity race tests and the complete Go suite passed locally. This proves server-side assertion checks for these cases; virtual-browser passkey workflows, physical authenticator behavior, additional replay/cross-device races and sensitive-route/transaction/UI integration remain unfinished.

## Approval replay boundaries and actual AI cancellation evidence

- Added an eight-way concurrent grant-consumption race fixture: exactly one mutation executes and commits. Added refusal cases for changed resources/device/session/epoch, expired authority and actor revocation; none invoke the mutation. Identity race tests passed. These use synthetic grants to isolate transaction semantics and do not prove HTTP/browser integration.
- Run `36747768087` succeeded for `d023676`; downloaded and inspected its bounded AI boot artifact. System SHA256 `fed98f1f839f859e798312c36aca5d416783e61161ccfe0dc8e96110eda35f0c`, no-NIC TCG object transfer/replay, immediate generation cancellation with deletion acknowledgement, then separate generation of 127 UTF-8 bytes and deletion acknowledgement. This establishes actual guest cancellation/recovery for that revision, not sustained-stream cancellation, newer parser compatibility, controller/UI behavior or physical KVM/resource qualification. ReleaseQualified remains false.
- Action approval route/UI integration, production maintenance/backup/restore, independently trusted release artifacts and real host/network/security gates remain unfinished.

## Approved pairing durable mutation adapter

- Extracted shared invitation creation into a transaction helper and added PairApproved. It decodes a bounded exact request body with unknown/trailing field refusal, validates name/capabilities and creates the invitation/event in the same transaction that consumes device.pair approval. No separate caller-supplied name/capability set can diverge from approved bytes. Existing Pair remains for the current HTTP flow until API/frontend integration is ready.
- Added synthetic-grant pairing regression: changing capabilities denies issuance, the exact approved body creates the expected issuer/name/capabilities invitation, and reuse cannot issue another invitation. Identity/control race tests and the full Go suite passed locally. Public approval routes/browser prompts, revocation/workload/AI mutation adapters and full end-to-end policy qualification remain unfinished.

## Approved device revocation transaction adapter

- Extracted shared revocation into a transaction helper and added RevokeApproved with exact target-ID and bounded empty-object request-body binding. Grant consumption, last-admin protection, revocation, session/credential/invitation/issuer-challenge cleanup and audit event now share one transaction in the approved path. Existing Revoke remains used by current HTTP callers pending complete API/UI integration.
- Synthetic-grant regression proves target substitution refusal, successful revocation/approval consumption, revoked-session denial and replay refusal. Last-admin refusal rolls back grant consumption as well as mutation. Identity/control race tests and the full Go suite passed locally. Public approval endpoints, browser action prompts, workload/AI approved mutation adapters and complete product/hardware security qualification remain unfinished.

## Exact identity approval request interpretation

- Added a bounded UTF-8/exact-key object decoder for identity approval requests. Duplicate decoded keys (including escaped equivalents), case variants, unknown fields, nonobjects and trailing values are refused. Pairing validates the exact name/capability values; revocation accepts only an empty object. Both approved mutation adapters now use this parser.
- Added BeginPairApproval and BeginRevokeApproval to derive fixed action/resource authority from these validated request bytes rather than accepting client-proposed bindings. Request bytes remain hashed exactly, including whitespace. Identity/control race tests and the full Go suite passed, including ambiguous-body refusals and valid reordered-field admission. HTTP endpoints/UI integration and the remaining sensitive workloads are still unfinished.

## Authenticated identity approval HTTP ceremonies

- Added admin-authenticated pairing/revocation approval-begin endpoints and authenticated assertion finish under the existing authentication rate limiter. Begin bodies are capped at 4 KiB and derive authority through the strict action-specific identity methods; options/challenge token/expiry are returned in no-store JSON. Finish accepts the opaque challenge token only in X-Approval-Challenge, verifies the actual assertion body and returns an opaque grant. All endpoints inherit existing HTTPS/origin/content-type/connection/body protections; begin endpoints also use the finite authentication limiter.
- HTTP fixtures prove unauthenticated denial, ambiguous/oversized request refusal, a UV-required begin response, noncacheable token response and missing-challenge finish denial. Identity/control race tests and the full Go suite passed locally. The synthetic begin credential does not prove signed HTTP/browser completion. Current mutations still use the existing freshness flow until frontend/grant-required route integration is complete; section 95 remains unsatisfied.

## Device approval browser and mutation integration

- Pairing/revocation HTTP mutations now require X-Action-Approval and call their transactionally approved adapters; fresh sessions alone cannot bypass approval. The Devices UI snapshots exact JSON bytes, begins the specific action ceremony, invokes browser passkey authentication, verifies it with the challenge header and sends the same bytes with the opaque grant. Tokens stay in memory/body/headers, not URLs/storage. Existing origin/TLS/no-store boundaries remain.
- Added HTTP refusal for pairing/revocation without an action grant despite a fresh administrator session. Identity/control race tests, full Go suite and production frontend build passed. The existing full enrollment/pair-limited-device/revoke/sign-in/recovery browser test passed with Chrome's virtual UV authenticator, exercising the new approval paths. The initial Playwright attempt could not launch its missing downloaded browser; explicit installed Google Chrome succeeded. This is desktop virtual-authenticator evidence, not physical phone/passkey testing.
- OpenAPI approval contracts, additional browser denial/replay cases, workload/AI action approval integration and production maintenance/release/network/hardware qualification remain unfinished.

## Device approval OpenAPI contract slice

- Added contracts/device-approvals.openapi.json (OpenAPI 3.1) for the five implemented device approval/mutation endpoints. Records production session-cookie authentication, required origin and designated token headers, bounded request schemas, two-minute binding/ceremony semantics, exact-byte mutation matching, transactional consumption/rollback, no-store responses and common error responses. Explicitly identifies development cookie differences and remaining API scope.
- Reviewed paths/status/header/body shapes against current control/identity code and successful virtual-passkey browser flow. JSON parsing, all local reference resolution and unique operation identifiers passed a structural check; this is not a complete OpenAPI specification-validator or generated-client conformance result. Other endpoint contracts and richer WebAuthn/error schemas remain required alongside workload approvals and full product/release/hardware gates.

## App intent transaction boundary for approvals

- Extracted AppActionInTransaction, retaining existing workload/action/backend admission, idempotency and revision checks. It persists only the pending operation/app intent through the caller's transaction; runtime effects remain in the operation worker. Existing AppAction wraps the same helper, preserving current behavior until approval route integration. This permits grant consumption and lifecycle intent to commit atomically without nested transactions or external VM effects inside the transaction.
- Added caller-transaction rollback/commit regression: injected failure leaves no apps/operations and performs no backend starts; commit leaves one pending operation and still performs no backend effect. Workload race tests and the complete Go suite passed locally. App approval endpoints/UI, resource-instance binding/replay contracts and AI deletion approvals remain unfinished, alongside full product and physical qualification gates.

## App instance and revision approval resource admission

- Added transaction-scoped app approval resource derivation covering logical workload, current instance ID when present and explicit revision (zero for never-created apps). Added an intent adapter that rechecks that exact resource snapshot in the mutation transaction before persisting lifecycle intent. Changed instance/revision snapshots return conflict instead of applying stale authority; no runtime effects happen in these methods.
- Regression verifies initial resource shape, refusal after another intent advances revision/instance, current snapshot admission and absence of backend effects. Workload race tests and the full Go suite passed locally. This is the resource-admission component; app approval HTTP/UI integration, idempotency-replay approval handling, AI deletion approvals and full product/security/hardware gates remain unfinished.

## App approval begin API and idempotency-key binding

- Added strict preset-only app action parsing and an administrator-authenticated app approval-begin endpoint. It refuses unconfigured runtimes, derives current workload/instance/revision resources and adds a SHA256 binding of the validated-length idempotency key before beginning the UV-required ceremony. Exact body bytes and current policy/epoch/session remain bound; client-proposed resource bindings are not accepted. The endpoint inherits approval authentication rate/body/origin/TLS limits.
- Identity/control race tests and the full Go suite passed, including unknown/duplicate/case-variant/action-field refusals and valid start/stop parsing. App mutation consumption and browser reuse of the exact bytes/idempotency key remain unfinished, so existing app mutation freshness behavior is not claimed as action-bound authorization. Complete lifecycle/release/network/hardware gates remain open.

## App lifecycle approval mutation and UI integration

- App start/stop HTTP mutations now require a specific grant, bind the exact body plus idempotency-key digest/current instance/revision resources and consume approval in the same transaction as lifecycle intent. A changed snapshot is rechecked before intent insertion. Runtime effects remain outside the transaction. Moved the browser approval flow into a shared helper; app controls preserve one key and exact body across begin and mutation, while device controls reuse the same helper.
- Added HTTP synthetic-grant regression proving changed idempotency key denial, exact request intent creation and consumed-grant replay denial with one pending operation. Control/identity/workload race tests, full Go suite, frontend production build and the existing virtual-passkey device enrollment/pair/revoke/recovery browser flow passed. The device browser flow verifies shared-helper compatibility; it does not execute app approval against a configured physical runtime.
- App-specific browser tests, lost-response idempotency recovery behavior, expanded OpenAPI, AI deletion approvals and full installation/network/maintenance/release/hardware qualification remain unfinished.

## Durable conversation deletion intent foundation

- Existing conversation deletion performs guest side effects before metadata commit, so it cannot safely run inside approval consumption. Added a transaction-scoped deletion-intent method: validates target existence and absence of active generation states, records a deduplicated pending operation and exact conversation reference, and leaves conversation/guest data untouched. It is not called by public routes until processing/admission/UI integration is complete.
- Regression proves rollback removes intent, same-key replay returns the same operation and intent insertion remains pending without deleting conversation metadata. Workload race tests and the full Go suite passed after correcting a fixture call to the actual CreateConversation signature. Durable cleanup worker, prevention of new generation while deletion is pending, restart/retry behavior, approval API/UI and backup/recovery interaction remain required.

## Close generation admission during pending deletion

- Generation admission now checks for pending/running conversation deletion inside the same transaction as generation intent, refusing new work before staging metadata or guest uploads. Deletion intent insertion refuses a second operation for the same conversation while preserving same-key replay; its duplicate check excludes its own newly inserted operation.
- Regression verifies target-scoped pending admission, unrelated conversation independence, duplicate intent rollback and CreateGeneration refusal without persisted generation rows. Initial fixtures exposed self-counting of the freshly inserted deletion operation; corrected that query and reran workload race tests and the full Go suite successfully. Cleanup processing, restart-safe completion, approval/UI integration and backup/recovery interaction remain unfinished.

## Conversation deletion intent processing

- Added a bounded 30-second processing attempt for pending deletion operations, validated conversation references and integrated it with the existing service workers. Shared deletion cleanup retains its per-conversation lock and guest acknowledgement requirements. Successful metadata cleanup and operation success now commit together; errors retain pending intent for retry, avoiding a crash gap between deletion metadata and completion.
- Regressions prove empty-conversation completion/idle replay and unavailable-guest failure retaining generations/pending intent, then retry through a newly constructed service after guest availability returns. Workload race tests passed including the restart retry; the full Go suite passed before adding that final fixture. Mocked model output/in-process guest storage fixtures do not establish VM or physical power-loss behavior.
- Public approval/UI integration, per-object deletion progress/backoff/fairness, lost-response recovery and backup/recovery treatment of pending deletion authority remain unfinished. Existing public synchronous deletion remains until that integration is ready.

## Approved asynchronous conversation deletion API/UI

- Added AI-capability-authenticated deletion approval begin, deriving conversation ID plus idempotency-key digest from the actual route/header and strict empty-object bytes. Current target existence and active generations are checked before ceremony and again at durable intent admission. Deletion mutation now requires its action grant and commits consumption with pending deletion intent, returning 202/operation rather than premature deleted=true.
- The UI uses the shared passkey flow, exact bytes/key and polls the returned operation; it removes the conversation only after succeeded. A two-minute observation limit reports pending cleanup without cancelling server intent. No active session freshness bypass remains on the public conversation deletion route.
- Added HTTP refusal/no-intent-persistence for deletion without a grant. Identity/control/workload race tests, full Go suite and frontend build passed; the virtual-passkey browser journey passed including approved empty-conversation deletion and subsequent device pairing/revocation/recovery. This does not prove guest-content deletion in a real VM, recovery after page reload, per-object progress/backoff or backup/recovery authority treatment. Expanded API contracts and production lifecycle/release/network/hardware gates remain unfinished.

## Persist deletion retry timing and avoid starvation

- Failed conversation cleanup now persists a bounded attempt count and next eligibility timestamp, using exponential delays capped at 64 seconds. Eligible pending operations are selected by update/creation order rather than repeatedly choosing an unavailable oldest conversation. Generation admission remains closed during the delay; success still requires acknowledged guest cleanup plus metadata/completion commit. Retry payload size/fields/count/time are admitted explicitly.
- Extended unavailable-guest regression to inspect persisted scheduling, complete a second eligible empty-conversation deletion while the first is deferred, then simulate time eligibility and reconstruct the service for successful retry. Workload race tests and the full Go suite passed. This establishes scheduling/restart behavior in fixtures, not per-object progress, malicious payload recovery, real VM power loss or physical durability. Backup/recovery authority handling, reload/lost-response UI recovery, full API contracts and production lifecycle qualification remain unfinished.

## Recovery snapshot excludes approval and deletion authority

- Inspected existing snapshot sanitization and recovery validation: pending/executing/requires-action operations are interrupted with empty results, challenges/grants are removed and devices are revoked under a new recovery epoch. Added a specific regression with pending conversation deletion retry metadata plus approval ceremony/grant markers. No production sanitizer change was necessary.
- The actual SQLite backup/VACUUM fixture verifies the standalone admitted recovery copy retains conversation metadata, replaces destructive deletion intent with interrupted/empty authority, excludes approval markers from snapshot bytes, and leaves live source deletion retry state and approval rows unchanged. State/backup race tests and the full Go suite passed locally. This is management snapshot evidence, not clean app-disk backup, replacement-host activation, physical byte erasure or complete restic restore qualification. Maintenance/quiescing, per-object deletion progress, reload/lost-response UI recovery, release/network/hardware gates remain unfinished.

## Pending conversation deletion visibility after reload

- Conversation listings now report whether a pending/running deletion operation targets each conversation. The AI UI labels pending cleanup, hides generation controls, disables duplicate deletion and polls listings while pending cleanup exists, clearing a selected conversation when it disappears after completion. It does not depend on a page-local operation token to rediscover pending state.
- Backend regression verifies pending intent appears in listing state. Added an explicitly mocked listing browser reload fixture verifying pending status and disabled message/deletion controls, then removed the interception and completed the real approved deletion/device identity journey. Workload/control race tests, full Go suite, frontend production build and virtual-passkey browser workflow passed. The listing fixture establishes UI behavior, not actual VM deletion across reload; per-object progress, lost mutation-response recovery and complete production backup/release/network/hardware qualification remain unfinished.

## Checkpoint acknowledged conversation object cleanup

- Durable deletion processes generations in stable ID order and validates task/prompt object IDs before use. After both guest deletion acknowledgements, it commits removal of only that generation's metadata, conditioned on the matching pending deletion operation. Remaining generation rows serve as restart checkpoints; no guessed cursor or unacknowledged metadata is removed. Conversation removal/operation success still commit together after all remaining objects finish. The legacy synchronous helper retains its prior final-only metadata behavior.
- Added two-generation fault regression with actual in-process guest prompt storage and mocked inference output: failure on the second task leaves exactly its metadata, then service reconstruction/eligible retry completes without reissuing cleanup of the first task. Workload race tests and the full Go suite passed. This establishes checkpoint ordering/retry semantics, not real VM power-loss durability or physical erasure. Pending UI/error/retry contracts, full backup/quiescing/release/network/hardware qualification remain unfinished.

## Quarantine invalid durable deletion records

- Deletion intent parsing now rejects duplicate decoded keys (including escaped aliases), unknown fields, explicit nulls, invalid UTF-8, oversized payloads, incorrect scalar types, trailing values and out-of-range retry fields. Invalid records move from pending to requires-action without guest calls or metadata deletion, retaining the original record for diagnosis. Eligibility selection and conversation lookup guard SQLite JSON extraction so malformed JSON cannot poison the whole queue/listing.
- Valid records identifying a conversation in requires-action continue blocking new deletion/generation admission. Listings expose the attention state and the UI distinguishes it from automatic retry. Records too malformed to identify a conversation cannot establish that association; they remain visible through their operation, and no guest authority is inferred from them. A guided local recovery/repair flow remains unfinished.
- Fault fixtures cover malformed JSON, literal/escaped duplicate identifiers, null retry values, string scheduling fields and unknown fields: invalid operations retain conversation metadata, are quarantined, and do not starve the subsequent valid deletion. Fixtures also verify attention/listing/admission behavior for unambiguous identifiers. Full Go suite, workload/control race suites, frontend production build and the existing Chrome virtual-passkey end-to-end journey passed. These are local database/controller and browser checks, not physical corruption/power-loss tests or full application qualification; backup, release, network, hardware and recovery usability gates remain open.

## Durable maintenance admission barrier

- Added a database-backed exclusive maintenance token. Acquisition serializes with workload admission transactions; overlapping maintenance is refused. The barrier deliberately survives controller restart without automatic expiry, and only its exact token can release it. This is a consistency prerequisite, not evidence that running work has drained or guest disks are safe to copy.
- Wired admission checks into new file transfers, video jobs, AI generations/conversations and app action intent. Matching job/generation/app intent replay can return existing work without new admission. Already admitted execution and cancellation remain available to the future drain coordinator. HTTP workload errors report maintenance as POLICY_DENIED with an explanation. Other configuration/data mutation families still need maintenance integration before backup can claim a stable snapshot.
- Recovery snapshots strip source maintenance ownership and reject recovery sets containing it; snapshot preparation leaves the live source barrier intact. Added concurrent acquisition/admission, reopen persistence, wrong/stale token, workload rejection/no-new-intent, existing app intent replay and recovery copy/source regression checks. Full Go suite and state/workload/backup/control race checks passed locally. Maintenance is an internal orchestration primitive; no public backup workflow or automated VM drain/stop/copy/restart coordinator is claimed. Those workflows, recovery usability and physical network/release/hardware qualification remain unfinished.

## Maintenance blocks new data mutation intent

- File rename/trash/restore now check maintenance in the same transaction as their metadata/event changes. New durable conversation deletion intent checks the barrier before commit; existing matching intent replay can still be observed and previously admitted cleanup can drain. No guest deletion authority is created by rejected requests.
- Added a real in-process guest file upload/finalize fixture, then acquired maintenance and verified all three metadata actions preserve the original file, new conversation deletion rolls back its operation, and token release re-enables rename. Workload race tests and the full Go suite passed locally. This extends the internal barrier; existing transfer processing, retention workers, identity/configuration mutations and VM stop/copy/restart coordination still require integration before a complete backup consistency claim.
- Inspected external checks: HomeNode checks run 36757618157 at 4203858 completed successfully. Run 36758501685 at 4cf1955 was still in progress when inspected, with Go/Linux adapters, disposable root ownership/capability fixtures, package build and source inspections successful; browser installation/workflow remained unfinished. That partial run is not recorded as a completed qualification result.

## Register trash cleanup activity before guest side effects

- Added durable activity registration serialized with maintenance admission. Maintenance can query whether registered activity has drained and cannot release its barrier while such records remain. Activity markers do not expire on restart; interrupted markers require explicit reconciliation. The drain query covers registered activity only and is not evidence that jobs, transfers or VMs are quiescent.
- Trash retention selects work, registers before guest deletion, and releases its marker in a bounded independent cleanup context after the attempt exits. It pauses when maintenance already owns admission and avoids marker writes on idle ticks. Already registered attempts remain observable to a future coordinator. Recovery snapshots remove old-host activity markers without changing live-source ownership; recovery validation rejects retained markers.
- State tests cover active/drained observations, refusal of new activity and barrier release, persistence across reopen, wrong tokens and completion. In-process Files guest fixture verifies maintenance retains expired-file metadata, then release allows acknowledged expiry and leaves no activity marker. Snapshot fixtures verify source/copy separation. Full Go suite and state/workload race suites passed locally. This does not yet register other background cleanup/transfer/VM activity or reconcile interrupted markers; the complete drain/clean-stop/stable-copy/restart backup coordinator and hardware qualification remain unfinished.

## Inspect durable maintenance blockers consistently

- Added token-scoped maintenance inventory using one management transaction: registered activity, unfinished transfers/jobs/generations/operations, apps not recorded stopped, unresolved job cleanup and orphan objects. Unknown workload/operation states remain blockers; malformed cleanup JSON is counted without poisoning the query. Ownership is checked before returning inventory, and errors return no partial inventory.
- Added existing AI admission fixture to verify pending generation/operation and running app counts, refusal of a foreign token, fail-closed unknown generation state, malformed cleanup visibility, and separate generation-versus-operation completion. Full Go suite and state/workload race suites passed locally. The inventory reports metadata blockers only: zero counts are not authoritative proof of supervisor process exit, disk unmount, absence of in-flight byte calls, or stable app disks. A complete coordinator must combine this inventory with those independent checks and still needs implementation and hardware validation.

## Isolate malformed video cleanup journals

- Video cleanup selection guards JSON extraction and requires a nonnegative integer retry timestamp. Malformed JSON or an invalid timestamp cannot poison selection or reach stop/purge through that worker. Job status treats malformed or unresolved cleanup as pending rather than letting invalid JSON break listings, and maintenance inventory retains the blocker.
- Regression starts a real in-process job with injected purge failure, then adds a second interrupted cleanup-journal scheduler fixture (the retained first volume correctly prevents new VM admission). Corrupting the first record with malformed JSON or a string timestamp leaves it unresolved while the valid second journal issues exactly its stop/purge requests and completes. Runtime calls for the invalid record are absent; status queries remain usable; maintenance reports one cleanup blocker. Workload race suite and full Go suite passed. This is journal scheduling/controller evidence, not physical volume purge or power-loss recovery. Strict complete journal-schema admission, a guided local repair flow, activity registration for all cleanup paths and full backup coordination remain unfinished.

## Validate video cleanup journal before runtime authority

- Resource release now parses a bounded UTF-8 object containing exactly state and lastAttempt, rejecting decoded duplicate/escaped keys, unknown fields, nulls, incorrect types, trailing values and negative timestamps. Only pending/done states are accepted. Eligible invalid records are atomically wrapped as requires-action with their original value preserved; no stop/purge is sent. Already done journals return without repeat cleanup.
- Retry timestamp update compares the exact original journal, and successful cleanup completion compares the canonical pending journal written before runtime calls. A concurrent record change is a conflict rather than being silently overwritten. Stable supervisor cleanup IDs remain unchanged; this is not an exactly-once claim for concurrent runtime calls.
- Extended fault scheduling fixtures with duplicate state, escaped duplicate retry key and unknown-field records. Deterministic ordering verifies eligible invalid journals are quarantined, original values retained, no runtime requests issued for them, later valid cleanup completes and maintenance still reports unresolved cleanup. Workload race tests and the full Go suite passed locally. Invalid records not eligible for worker selection remain visible blockers; guided repair, complete cleanup activity/freeze coordination, clean VM backup/restart and release/network/hardware qualification remain unfinished.

## Bounded cooperative domain shutdown primitive

- Inspected existing Linux Stop behavior: it deliberately uses virsh destroy for forced teardown. Added a separate LinuxBackend Shutdown primitive requesting fixed-command ACPI shutdown, then polling domain-running state under a 90-second overall deadline. It validates the instance ID, returns probe/request failures, honors cancellation and never escalates to destroy. Existing forced teardown behavior and public supervisor actions remain unchanged.
- Libvirt documents shutdown as asynchronous and supports explicit ACPI mode: https://www.libvirt.org/manpages/virsh.html . The domain XML already enables ACPI. Tests with injected lifecycle probes cover delayed observed exit, already-stopped domains, request/probe errors, cancellation and invalid identifiers; full Go suite and supervisor race suite passed. These are lifecycle control-flow tests, not an actual virsh/guest shutdown qualification.
- Observed domain exit does not attest a clean guest unmount and may coincide with a crash or independent supervisor teardown. This primitive is not yet exposed as a maintenance action: integration must prevent ordinary stopping-state audit from racing cooperative shutdown, retain independent emergency enforcement, establish guest shutdown/unmount evidence and separately verify disk-copy safety. Actual assembled-image ACPI handling, supervisor/controller coordination, guaranteed app restart and end-to-end backup/hardware qualification remain unfinished.

## Prepare guest images for cooperative ACPI handling

- Inspected the minimal guest package recipe and overlay/finalizer. Added the system D-Bus dependency, explicit systemd-logind enablement and a guest-only logind drop-in selecting poweroff for the power key, ignoring suspend/hibernate/lid/idle actions and setting the high-level power-key inhibitor policy. The drop-in is inventoried/staged inside the guest overlay only; it does not modify host power policy or grant the adapter host authority.
- Systemd documents the settings in its maintained source manual: https://github.com/systemd/systemd/blob/main/man/logind.conf.xml . This policy does not establish that all low-level inhibition is impossible. Added enablement expectations for files/video/AI finalization; existing overlay/import/tamper/replay tests cover the expanded inventory. Guest Python suite passed 44 tests with two platform/tool-dependent skips on macOS; shell staging syntax passed. No Linux image assembly or ACPI boot result is claimed for this revision.
- Added dependency/service closure requires fresh image/SBOM/vulnerability qualification. Actual guest ACPI dispatch, service termination, data unmount and domain exit must be demonstrated on assembled images and supported hardware. Supervisor maintenance/audit coordination, stable disk copying, guaranteed restart, complete backup/recovery workflows and broader release gates remain unfinished.

## Require guest shutdown evidence in assembled boot fixture

- Added a fixture-private QMP socket and bounded protocol exchange: capability negotiation, system_powerdown acknowledgement and guest-initiated SHUTDOWN with reason guest-shutdown. Host signal/crash reasons are rejected. Messages/events and elapsed time are bounded. The fixture then requires zero QEMU exit and successful read-only e2fsck of the disposable data disk before writing boot success evidence. Failure-only terminate/kill cleanup cannot satisfy this gate. QMP semantics follow https://www.qemu.org/docs/master/interop/qemu-qmp-ref.html .
- New socket-pair tests exercise successful guest shutdown plus host teardown, panic, incorrect guest flag and command refusal. Direct boot protocol tests passed 15 cases; guest Python suite passed 46 with two platform/tool-dependent skips under ResourceWarning-as-error on macOS. No real QEMU/ACPI/filesystem result is claimed from these mocked protocol tests. The Linux CI image boot fixture now enforces the additional gate when run.
- Inspected live image run 36777477052 at 8791e50; it was in progress and predates this fixture gate. Its eventual old-fixture success cannot qualify the new shutdown check. Current-revision assembled-image results still need inspection. TCG shutdown/readonly filesystem checks, even once passed, do not establish KVM/AppArmor behavior, clean unmount attestation for production volumes, complete backup/restore consistency or physical durability. Supervisor audit/maintenance coordination and guaranteed restart remain unfinished.

## Verify shutdown edge cases and retained power-policy image builds

- Added QMP tests rejecting command acknowledgement without a guest shutdown event and accepting an event arriving before acknowledgement. Added supervisor lifecycle test for a guest that keeps running until the caller deadline; it returns deadline exceeded without repeat shutdown requests. Direct boot protocol suite passed 17 tests, guest Python suite passed 48 with two platform/tool-dependent skips under ResourceWarning-as-error, and supervisor race tests passed locally. These protocol/lifecycle mocks do not establish actual guest shutdown.
- Inspected completed successful Development guest images run 36777477052 at 8791e50, jobs 110098917709 (files) and 110098917844 (video). Downloaded retained boot evidence: files image SHA256 90393ff674af2be105dbdc2af1ca6b9084a395a0b2c5a67e6912b012721c0674; video image SHA256 f0f82afc6c6a975354fb1f277a19283b64c2c7a68538353d2828fba940e16c48. Both show no-NIC TCG boot, 1048579-byte/262144-byte-chunk transfer and acknowledged replay. Video additionally reports tiny H.264 720p/1080p fixtures and immediate cancellation/delete acknowledgement. Both remain releaseQualified=false and predate the cooperative shutdown evidence gate.
- New-gate image run 36777913854 at c460be5 is confirmed live: jobs 110101034778 (video) and 110101034983 (files) were building unsigned development disks when inspected. No result for its ACPI/QMP/read-only filesystem check is claimed yet. Preserve that live build rather than restarting on observation timeout. Complete supervisor/controller backup coordination, real shutdown evidence, app-data recovery, release/network/hardware qualification remain unfinished.

## Account for bounded cooperative shutdown in supervisor audit

- Added recognition of a shutting-down runtime state to restart reconciliation, active resource accounting, same-instance restart refusal and purge refusal. Audit observes that state under a protected per-instance deadline bounded to at most 90 seconds: missing/malformed/expired/excessive deadlines or host/isolation policy failures trigger ordinary independent forced teardown. Valid waits remain subject to host/image policy; observed domain exit leaves completion to the owning shutdown operation rather than inventing successful backup readiness.
- Tests seed the private runtime state after normal fixture start and verify healthy waiting, expired/missing deadline enforcement, host/isolation failure enforcement and interrupted restart reconciliation. Full Go suite and supervisor race suite passed locally. Public shutdown action/journaling, atomic deadline creation, owner completion checks, controller maintenance integration and guest/disk consistency attestation are not yet wired; this is audit/state accounting groundwork for that operation, not an end-to-end backup result.
- Revalidated live image run 36777913854: files/video jobs were still building unsigned development disks when inspected. No shutdown boot result is claimed. Complete backup/restore, release/network/hardware qualification remain unfinished.

## Journal cooperative supervisor shutdown intent and completion

- Added the typed shutdown action to the independently authorized supervisor. Only backends implementing cooperative shutdown can accept it, and only existing running files/AI instances with a newer revision are admitted. Stop revision, shutting-down state, desired state, operation owner and bounded deadline commit together before backend calls. Pending replay is refused rather than extending authority or repeating work; completed matching replay can observe the same stopped revision without another request.
- Success requires a stopped domain and an atomic conditional commit matching shutting-down state, revision, exact owner and exact unexpired deadline plus pending operation. An intervening stop, audit interruption or changed deadline prevents stale completion. Resource accounting also prevents a second instance of the same workload while the first is still in an active shutdown state. Ordinary forced teardown remains available independently.
- Backend fixtures verify authority precedes the call, successful completion/replay, refusal to report success after shutdown failure, later higher-revision stop or deadline change. Full Go suite and supervisor race suite passed locally. This journals an observed domain-stop operation, not an attestation of clean guest unmount; external crash/teardown and already-absent domains still require separate consistency checks before backup. Controller backup orchestration, maintenance approvals, full drain/freeze/copy/restart, interrupted-operation repair and actual image/network/hardware qualification remain unfinished.

## Inspect actual files/video shutdown evidence and deny invalid operations

- Development guest images run 36777913854 at c460be5 completed successfully, files job 110101034983 and video job 110101034778. Downloaded retained evidence: files SHA256 a0841a84582502aa5bd035cc8b5de857c0b4dd8dd0fa5d013dfae4a641dfd545; video SHA256 bd43660089374e7a0a0db13d38215bdd4552396b31a9a42358d2d9d0393d9cc2. Both now explicitly report guestInitiated=true/reason guest-shutdown, qemuExitCode=0 and readOnlyFilesystemCheck=true. Both retain releaseQualified=false. These are actual assembled no-NIC TCG fixtures with 128 MiB disposable data disks, not production KVM/physical-volume tests.
- Inspected retained console logs as corroborating evidence: files data.mount unmounted at line 729 and power down at line 762; video data.mount unmounted at line 737 and power down at line 770. Existing multichunk object transfer/replay, tiny video conversion presets and immediate video cancellation/delete acknowledgement also passed. AI, sustained-workload shutdown, actual supervisor/libvirt/controller chain and complete backup/restore consistency remain unverified.
- Added shutdown admission regressions for unsupported backend, mismatched workload, video instance, stale revision and wrong policy: no runtime call, no new operation/owner/deadline record and no instance state/revision change. Added audit-failure-during-shutdown regression confirming independent teardown prevents stale successful completion. Supervisor race suite passed locally. Controller maintenance/drain/freeze/copy/restart orchestration, recovery usability and broad release/network/hardware qualification remain unfinished.

## Use cooperative shutdown for running-app Stop intent

- Running files/AI app Stop now persists cooperativeStop=true in its server-derived durable app intent. The existing user action/approval binding stays app.stop; its worker sends the same operation/revision to the supervisor's shutdown action. A non-stopped reply is refused, and backend refusal marks the operation failed without silently falling back to forced stop. Stop intent for apps not recorded running, and previously persisted intents without the new flag, retain their prior teardown semantics.
- Updated in-process lifecycle fixtures to recognize the typed shutdown action. Added tests verifying the mode is persisted before effects, exactly one matching shutdown request is sent, success/failure status is correct and matching idempotent replay does not repeat runtime calls. Workload race suite and full Go suite passed locally. Existing supervisor Unix request/server bounds accommodate the 90-second operation; no frontend code changed.
- This connects ordinary approved app Stop to the journaled runtime shutdown path, not a complete backup coordinator or a clean-unmount certificate. Actual controller/libvirt/browser stop-chain qualification, maintenance-owned shutdown after drain, restart after failure, AI shutdown under load, disk copy/restore and release/network/hardware gates remain unfinished.

## Bind app success to matching runtime replies and durable ownership

- App workers now reject mismatched instance ID, revision, final state and workload replies. Start and cooperative shutdown require the named workload; legacy stop may legitimately receive an empty workload for an absent-instance tombstone. In-process fixtures now return the actual requested revision instead of omitting it.
- Successful app state, operation result and audit event now commit together. The transaction requires the current app operation/revision and the same device-owned executing operation; any lost ownership or interrupted operation rolls back the app update instead of leaving a partial success checkpoint. Existing failure/reconciliation paths still need a broader ownership/atomicity audit.
- Added runtime-reply fixtures for wrong ID/revision/state/workload, omitted cooperative workload, superseding app ownership and an operation switched to requires-action during execution. No invalid/stale reply reports success; interrupted completion preserves the pre-completion app state after transaction rollback. Workload race suite and full Go suite passed locally. These are controller transaction/reply checks, not real controller/libvirt/browser shutdown or complete backup/restart/recovery qualification. Full maintenance orchestration and release/network/hardware gates remain unfinished.

## Commit app failure without overwriting newer authority

- Replaced app-worker failure checkpoints with a single transaction that conditions the operation update on its device and pending/executing phase. Matching app operation/revision receives failed metadata in that same transaction and event. Superseded intent completes only the old operation as SUPERSEDED; newer app state is retained. An operation changed to requires-action/interrupted cannot be overwritten by the old worker's error path. Authorization expiry and pre-execution supersession use the same helper rather than ignoring app-update errors.
- Added backend-error fixtures that change operation phase or app ownership during execution. Added SQLite trigger fault injection aborting the app failure write after the operation update: the transaction rolls back both, retaining executing/stopping rather than a partial terminal status. This tests transaction failure semantics, not physical I/O/power-loss behavior. Full Go suite and workload race suite passed locally; the final fault regression also passed in the workload race run.
- Broader reconciliation, malformed intent handling, startup cleanup for uncertain runtime replies, maintenance coordinator failure/restart guarantees and complete backup/restore qualification still require work. No complete application or production hardware/release/network qualification is claimed.

## Admit exact app intent schema before runtime requests

- Added bounded UTF-8 app intent parsing with exactly four required fields and one optional boolean. Decoded duplicate/escaped keys, unknown fields, nulls, incorrect types, trailing values, invalid identifiers/workloads/actions and out-of-range revisions are refused. The action must match the operation kind; cooperative shutdown is valid only for stop. Old intents omitting the optional flag remain valid.
- Worker selection now validates before ownership/runtime processing. Still-matching invalid pending records move to requires-action with original contents retained, issuing no supervisor request; later valid operations remain selectable. Quarantine is conditional on the original device, phase and payload so a concurrent record change is not blindly overwritten.
- Added admission/scheduling fixtures for duplicate literal/escaped action, unknown field, null revision, unauthorized inspect action and cooperative start. Fixtures verify no runtime effect, retained requires-action contents and completion of the subsequent valid stop. Workload race suite and full Go suite passed locally. Complete guided intent repair, authority/clock edge cases, uncertain-start cleanup, maintenance drain/freeze/restart and end-to-end backup/recovery/release/network/hardware qualification remain unfinished.

## Teardown uncertain app startup without trusting reply identity

- Startup apply errors and mismatched runtime replies now receive the same bounded independent teardown previously used only after health failure. Teardown targets the persisted intended instance/revision, never the reply's identity, and uses stop rather than purge. Stop/shutdown failures do not gain a forced fallback through this startup-only path.
- Teardown requires a matching stopped reply. If cleanup fails or is inconsistent, app metadata and a requires-action operation with START_CLEANUP_REQUIRED plus the original intent commit atomically under current ownership. Ordinary failed status is used only after that teardown reply is confirmed. The uncertain operation is not selected for automatic execution retry. Domain-stop acknowledgement is still not a filesystem-clean/backup-consistency certificate.
- Backend fixtures model a response lost after start, wrong reply identity and unavailable cleanup. Tests verify exactly one start/one intended stop, stable instance/revision targeting, failed versus requires-action status and absence of automatic retry. Workload race suite and full Go suite passed locally. Actual lost-response/controller/libvirt behavior, guided repair, startup/cancellation reconciliation and complete maintenance drain/freeze/copy/restart/backup/recovery/hardware qualification remain unfinished.

## Reject queued app authority after clock rollback

- App execution now requires a positive creation timestamp no later than the current clock and at most 300 seconds old. Checking order before subtraction prevents extreme malformed timestamps from overflowing the age calculation. Future-dated intent fails closed after clock rollback rather than extending queued authority.
- Added boundary tests at exactly 300 seconds, 301 seconds, future/nonpositive clocks and integer extremes, plus worker fixtures proving invalid timestamps produce AUTHORIZATION_EXPIRED without any runtime request. Full Go suite and workload race suite passed locally.
- This verifies the controller's queued app timestamp policy only. It does not simulate host clock synchronization or establish completion of maintenance drain/freeze/restart, backup/restore workflows, networking, release or hardware qualification; those remain required by the complete product plan.


## Journal cooperative app stops under maintenance ownership

- Added an internal MaintenanceStop entry point for the coordinator. It validates the exact maintenance token and current unrevoked admin device in the transaction that creates stop intent. Ordinary app action admission stays closed. A token-derived hash supplies stable per-app retry keys without exposing the bearer token in operation payloads or keys. The existing worker independently rechecks device authority and queued expiry before issuing the typed supervisor shutdown.
- New stops require a running target app and no registered activity, nonterminal transfer/job/generation/operation, unfinished cleanup or orphan object. Admission rolls back entirely on blockers. Existing matching operations remain observable while the caller still owns the barrier. Shutdown refusal uses the ordinary failure checkpoint, retains the maintenance barrier and leaves the app as an inventory blocker without forced fallback. This is sequential per-app draining groundwork; no public endpoint or approval bypass was added.
- Regression fixtures verify wrong/released ownership, stable pre/post-execution retry, ordinary admission closure, cooperative-only runtime requests, blocked transfer/activity/malformed cleanup, revoked authority, unknown app phase and retained barrier after refusal. Full Go suite and workload/state race suites passed; the added final shutdown-failure fixture also passed in the maintenance race subset. These controller/backend fixtures do not establish real libvirt filesystem cleanliness or backup consistency.
- The complete backup coordinator still needs finite-work progress/cancellation and crash reconciliation, freeze of all byte activity, independently verified stable disks, privileged copy/snapshot integration, guaranteed app restart and owner-facing backup/restore workflows. Release, network and physical hardware qualification remain unfinished.


## Drain admitted work before persistent app maintenance shutdown

- Added an internal DrainMaintenance loop that retains the owned admission barrier, observes admitted finite work/cleanup, and journals running persistent app shutdowns sequentially through MaintenanceStop. The normal worker continues executing typed shutdown requests. Context cancellation does not release admission, cancel uploads, purge data or force a VM off. Requires-action operations and failed/unknown app states stop draining with a conflict for repair.
- A successful return means the management inventory was empty; it is explicitly not supervisor disk stability, clean filesystem, host byte-operation freeze or permission to copy disks. Independent supervisor authority and complete freeze/copy/restart orchestration remain required. No public backup route was enabled.
- Tests exercise two persistent app records through worker-issued cooperative shutdowns, stable retained admission after completion, cancellation with a pending upload, and immediate refusal of uncertain operations/failed apps without runtime effects. The second AI app is a seeded controller record in a backend fixture, not an actual AI VM boot. Full Go suite and workload race suite passed locally; the final uncertainty regressions passed in the drain race subset.
- Outstanding full-product work includes privileged stable-disk leases, freezing all relevant byte activity, app restart after backup failure, crash recovery, backup/restore owner journeys, deployment/release/network qualification and physical hardware validation.


## Refuse stale supervisor forced-stop completion

- While inspecting stable-disk prerequisites, found that forced-stop completion updated runtime instance state without revision/phase ownership checks. Completion now conditions stopped metadata on the exact revision, stopping phase and stopped desired state, and refuses an instance removed during the backend call. The matching operation must remain pending or an already succeeded retry. All completion writes share one transaction, so interrupted operation ownership rolls back the instance update. Stop for an initially absent instance remains supported.
- Backend mutation fixtures cover newer revision, interrupted instance phase, interrupted operation and removal during stop. These verify conditional database completion, not actual concurrent libvirt requests or a clean filesystem. Full Go suite passed; supervisor race suite passed with the final removal fixture.
- Stable disk authority, byte-activity freeze, backup/restart orchestration and full end-to-end release/network/hardware validation remain outstanding. This fix does not provide a disk lease or make domain-stop metadata a backup consistency certificate.


## Persist supervisor runtime maintenance admission barrier

- Added internal BeginRuntimeMaintenance/EndRuntimeMaintenance primitives with a root-store token that has no automatic expiry. Acquisition requires recorded instances stopped/removed with stopped desired state and recorded operations in known terminal phases. Overlapping acquisition and foreign/released-token release are refused transactionally. The shared supervisor operation admission helper rejects all new and replayed mutations while the barrier exists; inspect remains available. No public controller mutation route was added.
- Fixtures verify active/unknown/nonterminal runtime inventory refusal, mutation rejection without new operation rows, retained video bytes on refused purge, stable inspection, admission reopening only after correct release and barrier persistence across Manager.Initialize reconstruction. Full Go suite and supervisor race suite passed; final unknown-state inventory fixtures passed in the maintenance race subset. Reconstruction uses the same durable store and does not simulate host power loss.
- This is the admission component of the planned disk lease only. It neither drains already admitted external calls nor freezes independent audit/recovery, pins file descriptors, observes actual stopped domains or certifies clean guest unmount. A complete exclusive disk lease/copy implementation must supply those protections before any backup operation can rely on it. Backup copy/restart/crash recovery, user workflows and physical/release/network validation remain incomplete.


## Wait for whole supervisor calls before maintenance acquisition

- Added a Manager runtime read/write guard. Apply, Audit, Reconcile and Shutdown hold shared access for their entire work, including backend calls. Maintenance begin/end acquire exclusive access with context-aware bounded polling before touching the durable barrier. Shutdown uses an internal reconciliation helper to avoid nested guard acquisition. Normal mutations and emergency audit remain concurrent; maintenance waits for both before inventory verification.
- Held-backend fixtures verify acquisition cannot cross an admitted stop, cancellation returns deadline exceeded without persisting a barrier/retaining the guard, and acquisition succeeds after completion. A held audit fixture captures a running instance, permits ordinary stop to complete concurrently, then proves maintenance still waits for that earlier audit before acquiring. Full Go suite and supervisor race suite passed; final audit ordering fixture passed in the focused race subset. These are controller/backend concurrency fixtures, not actual libvirt/power-loss evidence.
- The guard covers the single live Manager owned by the supervisor service, not other root processes or independently constructed simultaneous managers. Durable admission remains closed across reconstruction. The complete disk lease still needs verified stopped-domain inventory, pinned descriptors, filesystem cleanliness and copy/restore/restart/crash recovery integration. All remaining full-product hardware/network/release/user-workflow gates remain active.


## Observe recorded domains before runtime maintenance acquisition

- Runtime maintenance acquisition now checks every recorded domain through Backend.Running while holding exclusive live-manager access. Running domains, probe failures, malformed instance IDs and cancellation refuse acquisition before the durable barrier is inserted. Removed records are checked too. Database rows close before external calls so probes do not hold the store's sole connection. Acquisition has a 90-second deadline and refuses inventories beyond 4096 records rather than performing unbounded command work. Historical-record retention/reconciliation must account for this bound.
- Backend fixtures cover stopped success, running despite stopped metadata, running despite removed metadata, unavailable domain probes, cancellation during a probe and malformed identifiers rejected before backend invocation. Refusal leaves no barrier. Full Go suite and supervisor race suite passed locally. These fixtures verify admission decisions, not actual libvirt domain inventory, physical crash/power-loss behavior or filesystem cleanliness.
- The observations cover recorded instances on the single live manager. Independent host processes and unrecorded domains still need qualification; pinned disk descriptors, filesystem checks, copy/restart/crash recovery and owner-facing backup/restore flows remain required. No disk-copy authorization or complete application/release/network/hardware qualification is claimed.


## Pin maintenance disks read-only for internal copier

- Added WithMaintenanceDisk, an internal trusted callback entry point. It verifies the exact root barrier owner, stopped persistent-app metadata and observed domain exit, then holds exclusive live-manager access and a read-only descriptor through the callback. Closing the descriptor and releasing process-local access occur on success/error or stack unwinding; the durable barrier remains until explicit owned release. No public callback/path RPC or backup endpoint was exposed.
- Linux volume admission opens only a validated instance-derived basename within a protected root-owned directory, compares directory identity and rejects symlinks, hard links, nonregular objects, wrong size/mode/owner. FIFO opens are nonblocking and rejected before any read. Non-Linux hosts refuse volume admission. Filesystems are neither mounted nor parsed/repaired as root.
- Added opt-in disposable Linux root fixtures for admission failures, read-only access and inode pinning after pathname replacement. An internal copier fixture verifies release times out while copying and a failed copy closes its descriptor before owned release. Added these fixtures to native CI volume tests. Local full Go suite/supervisor race suite passed; Linux test binary cross-compiles. Portable owner/path/released-token tests also passed. Native Linux execution is pending CI and must be observed before claiming these platform checks passed.
- This descriptor primitive does not prove ext4 clean unmount, exclude other independent root writers, validate unrecorded domain inventory or provide protected copy staging, backup manifests, restart recovery and complete owner-facing backup/restore flows. Physical power-loss, host confinement, release/network and full-product qualification remain outstanding.


## Stage pinned disk bytes with checksum and exclusive publication

- Added StageDisk for the trusted backup coordinator to copy a leased disk descriptor into an exclusively owned private staging root. Only files.raw/ai.raw with bounded exact sizes, known schema/protocol and image digest are admitted. Output creation is exclusive with private permissions, preserving existing files. Positional source reads retain the descriptor's shared offset; copying records SHA-256 and checks source identity/size/modification time before syncing output and directory. The caller must retain disk lease authority and separately qualify filesystem consistency.
- Failed or cancelled copying closes and removes its own partial output, syncs cleanup and returns no successful manifest entry. Cleanup compares inode identity and preserves another writer's replacement rather than removing unrelated data. Private staging must exclude concurrent writers; the replacement fixture is defensive failure handling, not an supported concurrent-copy mode.
- Tests verify exact output bytes/digest/permissions, preserved source offset, no existing-output overwrite, invalid path/workload/size/predeclared digest rejection, cancellation after partial bytes and replacement-inode preservation. Backup race suite and full Go suite passed locally. These tests do not prove physical power-loss durability or protection against independent privileged source writers.
- Completed and inspected CI run 36784316645 at e02296ab879561126fed4a4611e1c55c1bfeb57d: Linux job 110121935238 succeeded, including the root volume command explicitly selecting TestNativeMaintenanceVolume. Its fixture covers descriptor admission, read-only/pinned identity, copy-error descriptor closure and blocking owned release during callback. This resolves the preceding native-fixture pending check only, not actual libvirt filesystems or full backup/restore qualification.
- Protected staging capacity/reserve policy, app/metadata snapshot orchestration, filesystem cleanliness, restic integration under full freeze, app restart/crash recovery and owner-facing workflows remain required, along with physical/release/network validation.


## Preserve staging capacity reserve during disk copies

- StageDisk now checks the staging directory descriptor's available blocks before output creation, between copy chunks and after file sync. Admission includes a fixed 4 GiB host reserve in addition to remaining disk bytes. Block-count comparisons round up and avoid overflowing available-block multiplication. Unknown filesystem capacity and unsupported platforms fail closed with a staging-capacity error.
- Added accounting boundary fixtures covering exact/short reserve, block rounding, zero remaining bytes, extreme available-block counts and invalid/oversized inputs. Backup race suite passed with the final post-sync check; full Go suite passed before that final check and Linux backup test binary cross-compiles. Existing successful-copy, cancellation and replacement-output fixtures run through descriptor-based capacity checks on this Mac. No real disk-full, concurrent-writer, power-loss or storage-quota qualification is claimed.
- These checks detect capacity pressure at copy checkpoints; they do not reserve space against unrelated host writers or replace full-host disk-pressure qualification. Complete protected staging lifecycle, metadata snapshot/restic orchestration, app restart/crash recovery and owner-facing backup/restore flows remain unfinished, as do the full release/network/hardware requirements.


## Require owned drained inventory around management snapshots

- Added MaintenanceRecoverySnapshot as an internal coordinator entry point around the SQLite backup/sanitization API. It requires the exact maintenance owner and empty management drain inventory before output creation and rechecks both after snapshot completion. A changed owner/inventory rejects the output and removes/syncs the newly created artifact in exclusively owned private staging. The source barrier remains intact on success.
- Tests verify foreign-owner and running-app rejection without creating snapshot.db, then stopped-app snapshot success with the existing recovery validator and retained source barrier. The fixture includes an enrolled source identity, matching the recovery validator contract. State race suite and full Go suite passed locally. Post-copy ownership-loss fault injection and physical snapshot power-loss behavior remain unverified.
- This wrapper checks management state only. It does not hold supervisor disk leases, freeze all configuration/byte writers, exclude a future backup operation from inventory, orchestrate consistent disk/metadata staging or implement restore/restart/crash recovery. Full owner-facing backup/restore, deployment/release/network and hardware requirements remain active.


## Build recovery inventory from owned snapshot and disk callbacks

- Added StageRecoverySet to connect owned management snapshot creation, authority/schema validation, supervisor maintenance disk callbacks, checksummed disk staging and final recovery-set validation. Disk inventory comes from the sanitized snapshot. Each returned supervisor instance must match the declared instance/workload, stopped desired/state and the caller's approved image policy before copying. A manifest is returned only after every declared disk is staged and management ownership/inventory are rechecked.
- The builder requires empty private staging, retains source admission ownership and cleans its own snapshot/disk artifacts on failure with inode checks and directory sync. It does not remove a different replacement inode. Catalog/release policy is validated before staging effects. The trusted coordinator remains responsible for actual filesystem consistency, full byte/configuration freeze, verified policy inputs and exclusive staging ownership.
- Integration fixtures use the real SQLite snapshot/recovery validator and disk staging with a modeled disk provider. Two declared app disks produce a validated three-file manifest. Failure cases include a later missing disk after earlier output, wrong instance identity/image, lost management owner and invalid sanitized identity; they return no manifest and clean builder artifacts. Full Go suite and backup race suite passed; final invalid-snapshot regression passed in the recovery-set race subset. These fixtures do not boot actual VMs or establish production privileged IPC/filesystem cleanliness.
- The complete product still needs approved backup operation bookkeeping, privilege-separated descriptor/metadata IPC, restic publication under a full freeze, guaranteed app restart/crash reconciliation, drive/restore user journeys and all outstanding physical/release/network/security qualification. No complete backup feature or application completion is claimed.


## Restore apps stopped by the maintenance owner

- Added internal MaintenanceRestart admission. It requires the current barrier token, unrevoked admin device, a succeeded maintenance-stop operation scoped to that same token/device/workload, and an app still stopped under that exact operation. It refuses an unrelated/superseded stop and active registered activities. A token-derived restart key provides stable retries without exposing the token. The normal worker remains responsible for authority expiry, signed runtime policy, start/health checks and uncertain-start cleanup.
- Restart does not release management admission. The trusted coordinator must finish copying, release supervisor disk maintenance, request these restarts, observe their results and only then resolve management maintenance or guided repair. No public app approval bypass was exposed.
- Fixtures exercise a complete controller start/owned-stop/owned-restart/worker-success sequence, stable pending retry and retained admission until explicit release. Rejections cover absent/failed owned stop, revoked device, superseded app ownership, foreign token and an active trash-expiry activity marker. The marker fixture is not evidence that actual disk copying is registered or frozen. Full Go suite and workload race suite passed locally; the backend is modeled, not a booted libvirt guest.
- A durable backup coordinator cleanup path, independent cancellation/recovery context, crash reconciliation, restart failure repair and owner-facing backup/restore publication remain unfinished. Full deployment, security/release/network and hardware qualification remain active requirements.


## Observe maintenance restoration before coordinator release

- Added RestoreMaintenanceApps to enumerate this barrier's known files/AI stop keys, refuse foreign or ambiguous device ownership, request stable owned restart operations and wait for the normal worker. Pending/executing restart must still own a starting app; successful restart must own a running app. Failed, uncertain or superseded phases require repair instead of falsely confirming restoration. Current admin authority and maintenance ownership are checked.
- Cancellation leaves pending restart intent and admission intact. A later cleanup/recovery call can observe the same restart operation after worker completion without duplicating starts. The helper never releases management admission and requires the trusted coordinator to release supervisor disk maintenance before invoking it. No public approval bypass or automatic start after failure was added.
- Fixtures verify cancelled wait/resume through actual controller worker execution with a modeled backend, retained admission, and refusal of failed/superseded restarts, foreign/revoked devices and superseded pending app ownership. Full Go suite and workload race suite passed before the final disappeared-intent/pending-owner guard refinements; the final restoration race subset passed afterward. No actual libvirt backup-failure recovery or physical restart is claimed.
- Durable backup coordinator phases, independent cleanup context wiring, guaranteed cleanup on backup failure, crash reconciliation, approved public backup/restore workflows and full platform/security/release/network/hardware qualification remain unfinished.


## Persist internal maintenance coordinator identity and phases

- Added a private versioned maintenance job record using fixed settings fields, with validated job/device IDs, known phase transitions and root-token/phase consistency. BeginMaintenanceJob atomically records admission closure, device ownership and draining phase. Owned transitions recheck current admin authority; root acquisition is checkpointed atomically from freezing to staging and cannot replace an attached token. No public journal/token payload is exposed.
- Ordinary EndMaintenance refuses active journals. RequireAdmission also refuses orphaned journal records even if the barrier key is missing. Recovery snapshot sanitization removes all journal fields, and the recovery validator rejects any retained source journal. This is internal persistence groundwork, not a wired backup coordinator or automatic recovery implementation.
- Fixtures verify persistence through Store close/reopen, overlapping/stale/invalid transition refusal, revoked authority, unknown journal fields, orphan admission closure and SQL-trigger rollback for partial begin/root checkpoints. Snapshot fixture verifies removed journal/root-token bytes in exports and retained source fields. Full Go suite passed before final phase/token consistency checks; final state race suite passed afterward. Reopen/SQL fault fixtures do not establish physical power-loss durability.
- Root acquisition interrupted before token attachment remains an uncertain state requiring privileged reconciliation; no retry or invented token is issued. Journal completion/owned release, coordinator execution, independent cleanup context, crash recovery, approved backup/restore UI/API, filesystem consistency and complete platform/security/release/network/hardware qualification remain unfinished.


## Complete maintenance only after acknowledged release and restoration

- Added internal ReleaseMaintenanceRoot, which validates the current owned restoring journal/device, invokes a trusted supervisor bridge and clears the root token only after a successful response and still-matching journal. Failed or stale acknowledgement retains recorded authority. A crash between external release and checkpoint remains uncertain and requires privileged reconciliation; no release receipt/idempotent recovery is invented.
- Added CompleteMaintenanceJob to atomically remove journal and barrier only in restoring phase with empty root token, current admin authority, no unfinished activity/work/cleanup/orphans and confirmed restart ownership for each app this barrier stopped. A stopped or pending-restart app cannot be forgotten, and foreign/ambiguous stop ownership refuses completion. No public backup endpoint or permission bypass was added.
- State fixtures verify failed root release retains token, callback receives exact token, attached authority/active activity block completion and old completed owner cannot be reused. Controller fixture executes owned stop, pending restart refusal and worker-confirmed restart before reopening admission. Full Go suite/state race suite passed before the final pre-callback journal recheck; focused state completion and controller completion race fixtures passed afterward. Root bridge/controller backends are modeled, not physical libvirt/drive recovery evidence.
- Durable coordinator execution, supervisor release receipts, crash reconciliation, completion audit/history, backup-operation inventory handling, filesystem consistency and user-facing backup/restore remain unfinished. Full release/network/security/hardware gates remain active.


## Retain supervisor receipt for lost root-release acknowledgement

- EndRuntimeMaintenance now atomically removes its owned barrier and records SHA-256 of the released token. The most recent release can be acknowledged again without changing any newer barrier. Unknown tokens and a receipt superseded by a later release fail closed. Retaining one receipt bounds storage; it is not an unbounded release history or acknowledgement of arbitrary older releases. Raw tokens are not retained in receipts.
- Tests verify receipt persistence through actual Store close/reopen, preservation of a newer active barrier during old acknowledgement retry, refusal of unknown/superseded tokens and SQL-trigger rollback if receipt insertion fails. A two-store integration fixture commits supervisor release but models a lost response before management checkpoint: recorded root authority remains, a receipt retry reconciles it and an otherwise empty maintenance job completes. This tests transaction/retry orchestration with a modeled transport loss and no VM disks, not physical power loss or deployed IPC.
- Full Go suite and supervisor race suite passed before the final reopen/cross-store fixtures; final release/reconciliation race subset passed afterward. Production privilege-separated maintenance IPC, acquisition receipts, complete coordinator execution, history/audit, filesystem consistency and full backup/restore/restart/UI/release/network/security/hardware validation remain unfinished.


## Bind runtime maintenance acquisition to durable coordinator job

- Added BeginRuntimeMaintenanceForJob. Acquisition atomically records a domain-separated runtime operation for the coordinator ID, root ownership and token. Same-job retries recover only its active token after domain/runtime checks; another job cannot take authority and a released operation cannot acquire again. Release completes the job operation in the same transaction as barrier removal/receipt. Orphaned root job settings block mutation admission. The interface remains internal to a trusted maintenance bridge, not a public caller authentication mechanism.
- Supervisor reconciliation preserves the recorded active maintenance operation while continuing normal interruption handling. The root operation remains pending until owned release, allowing recovery of an acquisition response lost before the management root-token checkpoint. No TTL or token guessing reopens admission.
- Fixtures verify same-token retry, foreign-job refusal, persistence through Store close/reopen and initialization/reconciliation, released-job refusal, preservation of newer acquisition on old release retry and rollback of operation/owner/barrier if publication fails. The cross-store release retry fixture now uses the same coordinator job ID for acquisition and verifies repeated acquisition before management attachment. Full Go suite passed with final source; supervisor race suite passed before final replay inventory guard and focused job/reconciliation race subset passed afterward. These are durable-store/backend fixtures, not deployed privileged IPC or power-loss evidence.
- Production maintenance peer authorization/IPC, coordinator execution/resume policy, journal/history retention, stable filesystem consistency, backup publication/restart/user workflows and complete platform/security/release/network/hardware qualification remain unfinished.


## Connect maintenance journal phases and independent cleanup

- Added internal RunMaintenance coordinator connecting journal begin, app drain, job-bound supervisor acquisition, root checkpoint, separate staging/publication callbacks and restoration/completion. The trusted callbacks remain responsible for qualified filesystem consistency, protected staging and encrypted publication; no public backup endpoint invokes it.
- Deferred cleanup uses an independent three-minute context, reconciles a freezing acquisition using the stable job ID, advances to restoring, releases/checkpoints root authority, observes owned app restoration and completes the journal. Original work errors remain errors even if cleanup succeeds. Cleanup failure retains admission closure and attempts requires-action checkpoint; it never reports a completed backup solely because services were restored.
- Real-journal/modeled app/root bridge fixtures cover success, stage failure, publication cancellation, lost acquisition response, failed root release and failed restoration. They verify callback phase/token binding, live cleanup context after work cancellation, admission reopening only on successful cleanup and retained repair phase otherwise. Full Go suite and backup race suite passed before the final release-failure fixture; the final coordinator race subset passed afterward. These fixtures have no running VMs, actual restic drive, deployed IPC or physical shutdown/restart evidence.
- Production privilege-separated bridges, coordinator restart/resume/claim policy, staging lifecycle and audit/history, actual recovery-set/restic callbacks, filesystem consistency, public approved backup/restore journeys and all full release/network/security/hardware qualification remain unfinished.


## Recover interrupted cleanup without repeating backup publication

- Added internal RecoverMaintenance for a trusted caller after the previous runner is confirmed stopped. It validates journal identity, reconciles cleanup with a bounded context and never invokes staging/publication. Recovery still requires exclusive runner ownership; no production startup claiming or public recovery endpoint was enabled.
- Fixed loss of acquisition uncertainty: freezing cannot advance through ordinary phase changes until a root token is attached, and failed cleanup preserves that phase. A subsequent cleanup reconciles job-bound acquisition before release/restoration. New journals use version 2. Compatible version-1 records upgrade on checked transitions; an ambiguous old requires-action record with an empty root token fails closed rather than guessing whether acquisition occurred.
- Fixtures verify repeated unavailable acquisition preserves freezing/admission, wrong-job refusal, later bridge recovery attaches/releases/restores without backup callbacks, and restoration retry does not repeat successful publication. Legacy ambiguity fixture verifies admission stays closed. Full Go suite and state/backup race suites passed against the final current worktree. These are real journal transactions with modeled bridges, not actual restic/VM/process-crash/power-loss evidence.
- Production runner claiming/privileged IPC, durable publication outcome/history, filesystem consistency, staging cleanup lifecycle, approved backup/restore UI/API and all full release/network/security/hardware qualification remain unfinished.

Maintenance runner ownership now uses a nonblocking kernel file lock in the private management state directory. Execution and cleanup recovery acquire it before inspecting or mutating a maintenance job and retain it through cleanup. The lock file remains in place after release to preserve inode identity. Admission rejects symlinks, hard links, nonregular files, nonempty files, unsafe permissions, and foreign ownership; unsupported platforms fail closed. This excludes cooperating runners sharing that state directory, rather than providing confinement against independent privileged processes.

Validation: the full Go suite and backup race suite passed. Native tests verify competing claims, unchanged lock inode across release, canceled claims, unsafe filesystem objects, release after killing a separate holder process, and a second database connection attempting execution/recovery during active staging without journal mutation or cleanup effects. Linux compilation was checked separately; native Linux runtime execution remains a CI gate. Production maintenance IPC, backup publication wiring, clean filesystem qualification, startup recovery integration, and complete restore workflows remain unfinished.

The private `RunBackup` entry point now connects the maintenance coordinator to `StageRecoverySet` and the authenticated repository's `Snapshot` method. It pins the staging directory before maintenance, validates owned publishing state before invoking the publisher, and retains a successful snapshot ID when subsequent source cleanup fails. Staging failures cannot reach publication. Staging is deliberately retained for a separately protected disposal/recovery policy; this entry point is not exposed as a public API or wired to root IPC yet.

Validation: full Go suite passed. Backup tests use a real SQLite recovery snapshot and two 16 MiB staged disk fixtures, validate the resulting recovery set at publication, and check that both maintenance barriers remain held until publication returns. Staging, publication, and restoration failures verify publication counts, snapshot outcome reporting, and admission behavior. The publisher in these coordinator tests is modeled; existing Linux restic tests cover encrypted repository primitives separately. This does not prove the combined production root/libvirt/restic chain, clean filesystem qualification, staging disposal after a crash, or restore activation.

Restic publication summary parsing now rejects duplicate JSON keys (including escaped aliases and nested statistics), invalid UTF-8, nonobject messages, absent or malformed snapshot IDs, snapshot IDs attached to progress messages, multiple summaries, trailing malformed data, and output beyond the existing 32 KiB bound. Additional version-dependent restic statistics fields remain accepted. The Linux publisher uses this parser only after a successful subprocess exit and still revalidates staged contents afterward; a parsed summary alone is not a backup qualification.

Validation: backup race tests passed with accepted summary/progress fixtures and rejection cases. Linux backup test compilation passed. These parser tests do not replace real encrypted restic round-trip tests or production publication/restore qualification.

A disposable Linux integration test now exercises `RunBackup` through a real authenticated restic repository, verifies source cleanup/admission, checks encrypted repository integrity, restores the resulting snapshot into separate empty staging, and validates all three payload hashes plus the restored two-app management inventory. The test is included in the existing `HOMENODE_RESTIC_INTEGRATION=1` CI suite. App/root bridges and stopped disk providers remain modeled; production VM shutdown, clean-filesystem qualification, drive enrollment, and restored application activation are separate unfinished gates.

Local validation: Linux test compilation passed. Native runtime evidence for this new round-trip is pending CI; it cannot execute on the owner Mac and is not yet reported as passed.

Restore failure cleanup now records file identities from the descriptors created by the restore and removes only matching directory entries. Missing entries are tolerated; replacements (including symlinks) are retained and reported as manifest/ownership failures. Staging still requires exclusive ownership for the entire operation: an identity check followed by unlink is not confinement against an independent privileged writer.

Validation: backup race tests passed, including original-file cleanup, already-missing entries, replacement-file preservation, and replacement-symlink preservation. Linux backup test compilation passed. Native encrypted round-trip run 36805038391 at ff8594c remained in progress when checked; its runtime result is not yet claimed here.

Native run 36805038391 failed the new maintenance round-trip at restore admission: its `testing.TempDir` destination was not explicitly private. Existing restore security tests document that TempDir permissions depend on umask. The fixture now sets 0700 before opening the destination; restore's admission policy is unchanged. The corrected native round-trip remains pending another CI execution, not yet verified as passed.

Corrected native CI run 36805302570 completed successfully at 038d6069f58e6605b49c572a8432602a77915260. Its Linux Go/race log explicitly sets HOMENODE_RESTIC_INTEGRATION=1 and reports the backup package passing in 35.904 seconds, covering the maintenance-to-real-restic publication/restore fixture and restore cleanup changes. All remaining source-protection, packaging, guest overlay, and browser steps also passed. This fixture evidence does not qualify physical KVM shutdown, drive enrollment, production maintenance IPC, or replacement-host activation.

A separate supervisor maintenance HTTP handler now authorizes only an explicitly configured backup UID distinct from root, controller, and transfer roles, using the existing kernel peer context. It exposes only job-bound acquisition and token-bound release, with 512-byte requests, version checking, duplicate-key/escaped-alias rejection, null/unknown-field refusal, and bounded operation contexts. Controller/transfer peers cannot reach this handler's maintenance operations, and the backup handler cannot invoke general runtime operations. This handler is not yet mounted by the production supervisor or installer; protected socket installation, typed client, backup service packaging, and descriptor transport remain required.

Validation: supervisor race tests passed for peer separation, malformed requests, acquisition replay, release replay, foreign release refusal, and released-job reacquisition refusal. These use trusted test peer contexts; actual Linux peer-credential transport is covered by the existing peer adapter tests and still needs the combined production socket integration.

The dedicated `MaintenanceClient` now implements job-bound acquisition and token-bound release over its configured local Unix socket, without general runtime/guest methods. It validates request IDs before transmission and limits acknowledgements to 512 UTF-8 bytes, exact versioned fields, unique decoded keys, and valid acquisition tokens. Release acknowledgements cannot contain acquisition tokens. Service errors remain failures rather than successful ownership claims.

Validation: full Go suite passed; runtime-client race tests exercise real Unix-socket request/reply and replay plus malformed, duplicate/escaped-alias, null, unknown-field, wrong-version, oversized, and invalid-UTF-8 acknowledgements. The Mac fixture uses a short private temporary pathname to stay within native Unix-socket limits. This client and the supervisor maintenance handler still need production socket/service installation, peer-account policy integration, and disk descriptor transport; these tests do not prove that combined installation.

A Linux maintenance socket integration fixture now connects the actual typed client and supervisor handler through an HTTP Unix listener using `PeerContext`/SO_PEERCRED. It checks stable acquisition replay, closes/reopens the journal and reconstructs the manager, replays acquisition with the same token, replays release, refuses reacquisition of a released job, and rejects a real kernel peer when the configured backup UID differs without creating a barrier. It runs as an unprivileged test UID and skips root because root cannot be the backup role. There are no runtime instances or physical VMs in this fixture.

Validation: Linux runtime-client test compilation passed, and the available Mac runtime-client race suite passed. Native execution of this new Linux socket fixture is pending the normal CI suite; production service/account installation and disk descriptor transport remain unfinished.

Supervisor startup now supports an opt-in dedicated maintenance listener via explicit maintenance UID/GID and socket arguments. It refuses partial configuration, root/overlapping peer roles, shared controller/transfer socket groups, nonabsolute/noncanonical maintenance paths, and reuse of the runtime socket path. The socket is root-owned with mode 0660 for the distinct backup group, and serves only the maintenance handler with kernel peer context. Both HTTP listeners shut down within the same bounded deadline before runtime cleanup; maintenance listener failure stops the supervisor rather than silently dropping that service.

Validation: full Go suite and affected command/client/supervisor race suites passed. Startup role validation covers disabled operation, valid distinct roles, partial configuration, root, and UID/GID overlap. Actual root service startup with the new listener has not been executed on the owner Mac. The standard unit/installer does not enable these arguments yet: backup account creation, environment configuration, service packaging, descriptor transport, and combined native installation qualification remain unfinished.

Backup service account inspection now validates a bounded local passwd/group/shadow snapshot alongside the existing controller/transfer policy. `homenode-backup` must have a unique non-root system UID and private system GID, locked password, nonexistent home, and nologin shell. Foreign primary-group users, aliased UID/GID assignments, runtime/other supplementary memberships, and foreign or duplicate private-group memberships are rejected. The read-only host inspector requires supported local NSS configuration and checks live getent/id results against the local identity, including exactly one allowed group.

Validation: installer race tests passed for a valid backup identity and root/alias/foreign-membership/unlocked/login-shell refusal. The host inspector's real NSS path has not been qualified with a provisioned backup account. This is validation groundwork; the account is not yet created by the journaled installer or enabled in the standard service environment, and no existing install is silently adopted or migrated.

Backup account creation now has a separate read-only plan with fixed groupadd/useradd arguments, an install-owner marker, no supplementary groups, and unused system UID/GID selection. It refuses existing backup users/groups, orphan shadow records, dangling backup memberships, and occupied/dangling numeric identities. This intentionally does not adopt an existing account. The caller must journal this plan and check live NSS vacancy before execution; resumable mutation/rollback and service environment activation remain unfinished.

Validation: installer race suite passed, including occupied-name refusal, orphan authority refusal, fixed command shape, and numeric ID reservation. No native account mutation was performed on the owner host.

The installer can now durably prepare backup account creation intent in its private locked journal. Preparation requires a ready base account journal, matching ownership markers and memberships for all existing account steps, and exact current service identities. It checks live NSS name/UID/GID vacancy before committing the canonical fixed plan as a 0600 journal using file sync, atomic rename, and directory sync. Reopen reuses only the identical committed plan; altered/duplicate/trailing journal data or changed allocation is refused. This method performs no account commands and cannot run natively outside root on Linux with host root `/`.

Validation: installer race suite and targeted preparation tests passed. Tests verify persistence/reopen without extra account mutations, private permissions, altered-journal refusal, unowned/incomplete base refusal, NSS collision refusal, canceled preparation without publication, and lost base ownership refusal. Native root preparation with a real backup account migration remains unqualified. Resumable execution of these two commands, rollback, standard service environment activation, and disk descriptor transport remain unfinished.

Review found and corrected a native preparation wiring gap: the fixed-name getent lookup allowlist did not include `homenode-backup`, so fixture preparation passed while native preparation would refuse its own planned name. The allowlist now admits that exact backup name without permitting arbitrary names. A Linux test checks passwd/group lookup of the fixed name (present or definitely absent) and arbitrary-name refusal.

Validation: available installer race suite passed; Linux test compilation passed. Native execution of the new lookup test remains pending CI; account mutation/rollback and service activation remain unfinished.

Backup account step reconciliation now recognizes the planned group separately from the owner-marked user, enabling later resume after a command acknowledgement is lost. It refuses mismatched numeric IDs, foreign install markers, foreign primary-group users or aliases, supplementary memberships, unlocked credentials, orphan shadow authority, and a user without its planned private group. This helper does not execute or checkpoint commands; the resumable provisioning loop and rollback remain unfinished.

Validation: installer race suite passed for missing/group-only/complete states, lost-result reconciliation, and foreign-marker/UID-alias/GID-alias/extra-group/unlocked refusal. Existing native CI run 36806938356 was confirmed in progress; this new reconciliation fixture has not yet run natively.

Backup account provisioning now executes only a previously committed canonical plan under the install lock. It validates journal ownership, owner continuity, bounded progress, disjoint system identities, and the exact fixed command list; rechecks existing account ownership/isolation and live NSS vacancy before mutation; and reconciles uncertain results before retrying. Each verified group/user step is durably checkpointed, followed by live maintenance-account verification before readiness publication. Ready replay verifies existing identities instead of recreating missing accounts. Native entry requires root on Linux with host root `/` and has a 60-second deadline.

Validation: full Go suite and installer race suite passed. Interruption fixtures stop after group or user creation before checkpointing, close/reopen the journal, and resume without duplicating commands. Tests cover ready replay, missing completed group refusal, missing preparation, NSS collision, and canceled mutation. These use modeled account commands; actual disposable Linux account provisioning and rollback are not yet qualified. Standard installer/CLI/service activation, rollback, disk transport, and backup workflow exposure remain unfinished.

The CLI now exposes root-only Linux backup account preparation, resumable provisioning, and read-only verification against the existing installer journal. Unexpected positional arguments are refused, execution is bounded, and machine-readable output explicitly records that services have not been activated. Installation-journal documentation gives the actual command sequence and directs interrupted provisioning to resume its committed intent rather than preparing another plan. Native owner-host mutation was not performed; disposable Linux qualification and service configuration remain required.

The existing opt-in disposable Linux account fixture now includes the backup peer: it establishes backup user/group vacancy, registers only those known names for fixture cleanup, prepares and provisions through the real installer backend, replays readiness, and invokes the installed CLI's backup account check. It introduces a real root supplementary membership to verify both inspection and provisioning replay refuse changed isolation, then repairs that membership and rechecks the identity. The normal CI account-inspection step already executes this fixture after package installation. It remains opt-in and is never executed on the owner Mac.

Validation: Linux installer test compilation passed; available installer race tests passed with native mutation skipped. The extended native account fixture awaits its new CI run, so actual root backup provisioning is not yet claimed as qualified. Production rollback, service activation, descriptor transport, and complete owner backup/recovery workflows remain unfinished.

Native account qualification evidence: CI run 36807537186 at 60b2c60acccaffc1f0d61013b9cba1295967cdfb completed the installed-account inspection step successfully. The step runs the opt-in fixture as root after development package installation and covers real backup preparation/provisioning, readiness replay, installed CLI identity inspection, privileged supplementary membership refusal, repair, and cleanup. The broader run was still finishing browser checks when this evidence was recorded. This does not qualify production rollback, standard service activation, disk descriptor transport, or owner backup/restore workflows.

The same run subsequently completed successfully. Its inspected log explicitly shows `sudo env HOMENODE_ACCOUNT_INSPECTION_INTEGRATION=1` selecting `TestInstalledAccountInspection` and reports the installer fixture passing in 0.287 seconds; all remaining browser/package checks passed as well.

Owned configuration generation now binds the optional maintenance socket to the ready backup account journal and live account inspection. Production configuration ignores caller-supplied maintenance IDs, rejects unfinished/changed ownership or NSS state, and adds literal verified UID/GID arguments to the supervisor unit. Existing installations without a backup account journal retain the standard unit. Prepared-install checking accepts only the exact standard unit or the exact generated maintenance unit backed by a ready journal; live installation checking revalidates the backup identity when that socket is configured. Configuration application still starts no services.

Validation: full available Go suite and installer race suite passed. Pure configuration tests cover generated socket arguments and unsafe/overlapping identities. Root-only temporary configuration tests cover ignoring supplied IDs and binding observed IDs; they are pending Linux CI execution and were skipped on the owner Mac. Real standard service startup with the generated socket, backup daemon packaging, rollback, descriptor transport, and complete backup/recovery workflows remain unfinished.

Owned shadow-file buffers are now cleared when backup inspection, backup preparation, and account provisioning return, including partial-snapshot/error paths. The account fixture returns independent snapshots so clearing an observed buffer cannot alter the fixture's source account database. Tests track returned buffers through successful and conflicting preparation and confirm they are cleared while source data remains intact. This is cleanup of owned byte buffers, not a claim that parsed strings or all process memory are erased.

Validation: installer race suite passed. Native configuration CI run 36808205575 was confirmed executing the root installer fixture during this change; its terminal result had not yet been observed. Service startup, backup daemon packaging, rollback, descriptor transport, and complete owner backup/recovery workflows remain unfinished.

Linux descriptor transport primitives now send/receive one read-only regular-file descriptor with a bounded 4096-byte UTF-8 metadata packet over Unix sequenced-packet sockets. Receive uses MSG_CMSG_CLOEXEC, rejects missing/multiple/writable descriptors and truncated or invalid packets, and closes received descriptors on rejection. Context cancellation interrupts socket I/O; each call requires exclusive connection ownership. Unsupported platforms fail closed. Metadata is opaque to this layer: kernel peer authorization, root file ownership, typed metadata, maintenance authority, and copy acknowledgement remain the caller's responsibility.

Validation: Linux test compilation passed. New native tests cover original inode pinning after path replacement and sender close, write refusal, close-on-exec, repeated rejected packets without descriptor leaks, and cancellation. Native execution is pending Linux CI; the owner Mac does not run this adapter. This transport is not yet wired to the supervisor/client disk callback or a production socket, and does not yet implement the maintenance copy acknowledgement lifecycle.

Descriptor transport now supports bounded control packets that cannot carry descriptor authority: unexpected rights are closed and refused. Copy-session helpers own their connection, close the received descriptor before emitting the fixed successful acknowledgement, and emit no acknowledgement after callback failure, cancellation, or panic. The source helper waits for that exact acknowledgement and retains the caller's source descriptor. It must be invoked inside the supervisor's guarded disk callback; root/client peer and typed metadata validation are still required before use.

Validation: Linux test compilation and available Go suite passed. New Linux tests cover control-packet descriptor rejection without leaks, success acknowledgement after close, failed copy without acknowledgement, retained sender descriptor, and false acknowledgement refusal. Native CI run 36809652485 at 5a26874085a4cc37848653049a549409d19b60b2 passed the Linux Go/race step, covering the earlier descriptor pinning/read-only/close-on-exec/rejection-leak/cancellation tests; its remaining browser checks were still running when inspected. New session tests await the next CI run. Production supervisor/client disk wiring and socket/service installation remain unfinished.

Disk requests and metadata now use strict bounded versioned objects. Decoding rejects duplicate decoded keys (including escaped aliases), unknown/missing/null fields, invalid UTF-8/trailing data, malformed authority IDs, unsupported workloads, incompatible schema/protocol, invalid image digests, and out-of-range disk sizes. Linux descriptor admission separately requires read-only regular-file access, root ownership, exact 0600 mode, one link, and exact declared bytes. It neither opens a caller-selected path nor certifies filesystem consistency.

Validation: protocol race tests and Linux compilation passed. A new opt-in root fixture covers valid descriptor admission and writable/mismatched-size/unsafe-mode/hardlink/foreign-owner refusal; CI executes it only on its disposable Linux runner. Native execution is pending the next run. Previous CI run 36810002244 at e30e9db8595b66334263da1e74ce82c9088e5b53 completed successfully, including the copy/control session tests. Supervisor/client disk bridge and production socket wiring remain unfinished.

The supervisor disk connection handler now authenticates a distinct backup kernel UID, admits one bounded typed request, and invokes the owned `WithMaintenanceDisk` callback. Descriptor transfer and acknowledgement waiting occur inside that callback, retaining the root runtime guard throughout copying. The backup `DiskClient` authenticates UID 0, validates returned metadata against its requested instance, checks root-owned read-only descriptor admission, invokes the trusted staging callback, and closes before acknowledging. The returned Instance is only a stopped disk inventory summary, without runtime revision/start authority. These per-connection bridges are not yet mounted by the production accept loop.

Validation: full available Go suite and affected supervisor/client/transport race suites passed; Linux supervisor/client test compilation passed. New Linux refusal tests cover foreign backup peers, malformed requests, foreign maintenance tokens without root effects, and client rejection of a non-root service before invoking copy. A positive cross-UID root-to-backup disk bridge fixture and production socket/service wiring remain required.

Earlier native CI run 36810283565 completed successfully at 893ece8435ca5f6ee253689e2d46f89cb8840179. Its inspected log explicitly runs the opt-in disk admission fixture as root and reports success in 0.003 seconds, covering valid root descriptor admission and writable/size/mode/hardlink/foreign-owner refusal. This does not qualify the newly connected cross-UID bridge or physical guest consistency.

### Backup disk listener lifecycle

Supervisor startup now creates a separate Unix sequenced-packet disk socket when the verified maintenance identity is enabled. The socket is root-owned and accessible only through the backup group's 0660 permissions; its path must be absolute, clean, and distinct from both HTTP sockets. The disk server admits at most two connections, closes excess connections promptly, and cancels and joins admitted handlers on cancellation or listener failure. Startup joins disk service termination during shutdown. Each admitted request retains the existing kernel peer, maintenance authority, descriptor, and copy acknowledgement checks.

Validation: affected supervisor/startup race tests passed and the final Linux supervisor test binary compiled. Linux-only tests exercise two admitted connections, prompt overflow refusal, cancellation, and listener-error cleanup; native execution awaits this commit's CI. Previous CI run 36810631863 at fdae7cb4d83fe7ec782af43090ea29665dd0bdc4 completed successfully, including the disk bridge refusal tests. Positive cross-UID descriptor copying, production backup service activation, and physical guest filesystem consistency remain unproven.

### Native backup disk client fixture

A new opt-in disposable Linux root fixture launches the real backup disk client under UID/GID 1003 with no supplementary groups. A root-owned sequenced-packet server authenticates that kernel peer and the exact request token/instance; the client authenticates root. The fixture source lives inside a root-only directory: the child must fail direct path access while reading the passed root-owned 0600 descriptor successfully. It checks stopped inventory, denied writes, receiver closure after successful acknowledgement, and continued sender descriptor ownership. CI runs the fixture explicitly as root; ordinary tests skip it.

Validation: Linux runtimeclient test compilation passed. Native execution is pending CI. The root fixture server uses the transport directly, so this evidence does not establish the manager's entire maintenance guard lifecycle, actual guest clean shutdown, or the production backup workflow.

### Read-only guest filesystem qualification primitive

The backup package now provides QualifyExt4Disk for a pinned regular read-only disk held under an exclusive stopped-runtime lease. It refuses an invalid ext4 magic, any state other than clean, or the journal-recovery incompatibility bit before invoking the fixed /usr/sbin/e2fsck -f -n command through an inherited descriptor. The command has a 30-minute maximum, a fixed environment, discarded guest-controlled diagnostics, and no repair mode. Success also rechecks inode, size, modification time, and header. Unsupported platforms refuse. Sources: https://www.kernel.org/doc/html/v6.6/filesystems/ext4/super.html and https://man7.org/linux/man-pages/man8/e2fsck.8.html.

Validation: pure header tests passed for clean admission and dirty/error/orphan-recovery/journal-recovery/magic/truncation refusal; Linux backup test compilation passed. Native e2fsck fixture execution and integration into the production staging bridge remain required. This primitive alone does not prove application consistency, clean guest shutdown, or exclusion of other writers.

### Filesystem qualification fixture and lease adapter

QualifiedMaintenanceDisks wraps a trusted descriptor source and holds its existing stopped-runtime lease across QualifyExt4Disk and the copy callback. Non-stopped/mismatched inventory and failed filesystem checks prevent copying. It remains a production integration component rather than a public backup endpoint.

A native opt-in Linux fixture formats a real 16 MiB ext4 source and verifies clean admission, unchanged full-image hashes, writable-descriptor refusal, cancellation, pinned descriptor behavior after source path replacement, and dirty-state refusal without repair. It also exercises the adapter's retained source lease and proves a dirty disk never invokes copying. CI enables the fixture in its Linux race suite. Local backup tests and final Linux test compilation passed; this new fixture's native execution is pending.

CI run 36811625576 at 1f5044b09c5f49fcf45f16dc4d11b26346e7561d completed successfully, including the explicit root-to-backup UID disk client fixture. This establishes that descriptor/client boundary on the disposable runner; the full manager lifecycle and physical guest consistency remain separate requirements.

### Full checker refusal and encrypted round-trip integration

The native ext4 fixture now corrupts the root inode using debugfs while explicitly requiring that the preliminary clean header still passes. The qualified bridge must reject that filesystem before copying; whole-image hashing verifies the checker did not repair it. This covers the full checker's refusal path separately from header-based refusal.

The existing native maintenance/restic round trip now formats its source as real ext4, reopens it read-only, and stages through QualifiedMaintenanceDisks. After encrypted publication and restore, both restored app disks undergo filesystem qualification in addition to the existing manifest/hash and management inventory validation. App/root authorities and source disk providers remain modeled; no physical VM clean shutdown or replacement-host activation claim follows.

Validation: local backup tests and final Linux compilation passed. Native execution of these expanded cases awaits CI after the current run finishes.

### Mandatory filesystem admission in RunBackup

RunBackup now always wraps the supplied disk source in QualifiedMaintenanceDisks before staging. A caller cannot omit filesystem qualification by passing a raw descriptor source. A focused test verifies that an unqualified fixture disk never reaches publication, reports no snapshot ID, and still completes source release/app restoration and reopens admission. Modeled success/cleanup orchestration tests use an unexported helper; they establish orchestration only. The native restic round trip uses the enforced RunBackup path directly.

Validation: backup race tests passed and the final Linux test binary compiled. The previous native fixture CI run remains active; expanded corruption/round-trip and mandatory-admission execution will be verified in the next run. The production daemon, private controller bridge, approval workflow, and actual guest shutdown qualification remain required.

### Private controller app-maintenance handler

The controller now provides a separate MaintenanceHandler for a protected local socket using kernel-derived PeerContext. Only a configured distinct non-root backup UID is admitted; HTTP peer claims confer no authority. The bounded version-1 request requires exact token, job ID, and device ID fields, rejecting duplicates/escaped aliases, unknown/missing/null fields, invalid IDs/UTF-8, trailing values, and oversized packets. Drain requires the owned job's draining phase. Restore requires restoring phase and a released root token. Both reuse the existing durable workload orchestration under a three-minute request bound, without acquiring new maintenance or exposing general runtime methods.

Validation: controller race tests and Linux compilation passed. Strict schema tests and a forged HTTP peer test passed, with no maintenance state created. Production socket startup, private client bridge, positive kernel-peer/owned-job tests, and public backup approval integration remain required. This handler is not mounted on the browser routes.

### Controller maintenance kernel-peer fixture

A Linux-only integration test serves the actual controller handler over a Unix socket with kernel PeerContext. An unprivileged peer exercises owned empty-inventory drain and restoration. The test rejects foreign tokens/job IDs/device IDs, refuses general runtime routes, requires the draining/restoring phases, and blocks restore until root authority has been released. It verifies that successful restoration leaves the owned job and management admission barrier intact for the coordinator to complete. Root execution skips this unprivileged-peer fixture.

Validation: Linux controller test compilation passed; native execution awaits the next CI run. This test uses a real socket and SQLite job state but no app workers or actual root runtime; those boundaries require their existing and future integration suites.

### Backup client for controller maintenance

MaintenanceAppsClient implements the coordinator's drain/restore bridge. It resolves the existing owned job through an injected inspector and verifies device and job IDs before sending a fixed typed request. Its local transport authenticates the configured non-root controller kernel UID on every new connection, refuses redirects, limits connections, bounds operation duration, and accepts only bounded UTF-8 exact version-1 acknowledgements. It has no general app or runtime mutation methods and can close retained idle connections.

The Linux controller fixture now exercises this client for owned drain/restoration and tests mismatched controller-UID refusal. Local affected race suites and Linux controller test compilation passed; positive native execution awaits CI. The inspector callback and controller socket startup still require production daemon integration; this does not grant the backup account access to the management database.

### Native filesystem and encrypted round-trip evidence

CI run 36812172361 completed successfully at 24e1582fbbda3ba3240f0b2bcedb00acb1cc467b. Its inspected log sets both HOMENODE_FILESYSTEM_INTEGRATION=1 and HOMENODE_RESTIC_INTEGRATION=1 and reports the backup race suite passing in 36.728 seconds. At that revision, the suite includes clean/dirty/corrupt-header filesystem admission, no-repair hashing, qualified RunBackup publication and restic restoration, restored ext4 checks, and unqualified-publication refusal. All other CI build, root fixture, packaging, and browser steps also passed.

This evidence is for disposable Linux filesystem/repository integration with modeled app/root controls. It does not qualify actual VM shutdown, physical hardware, the newly added controller bridge, or the full user-facing backup workflow. Controller bridge native validation is now running in CI run 36830746568 at 5b458b48caa1692e41f8c6b6ce758e32b30799c8.

### Private inherited socket admission

A Linux socketactivation helper now consumes exactly one named inherited listener for the current process. It requires the canonical LISTEN_PID/LISTEN_FDS/LISTEN_FDNAMES tuple, refuses unsupported PIDFD metadata, clears activation environment fields, checks a listening Unix stream descriptor and exact configured path, and requires a root-owned 0660 socket with the backup GID under a root-owned parent without unprivileged write access. It marks the original descriptor close-on-exec and returns an owned Go listener. Unsupported platforms fail closed. This permits a controller to inherit a root-created backup-only listener without backup-group membership. Protocol reference: https://systemd.io/FILE_DESCRIPTOR_STORE/.

Validation: activation environment refusal tests passed and Linux compilation passed. Positive root-created listener/child fixture, controller startup, systemd socket unit, and installer activation remain required. CI run 36830746568 completed successfully at 5b458b48caa1692e41f8c6b6ce758e32b30799c8, including the Linux controller maintenance socket/client tests.

### Controller inherited maintenance listener startup

The serve command now supports explicit maintenance UID/GID and socket flags. It admits this feature only on Linux under a non-root controller with a distinct system backup identity and the named root-created activation listener. Development mode, incomplete identities, and unexpected activation metadata are refused. The feature remains disabled in the standard unit until the installer supplies verified identity and a socket unit.

ServeMaintenance owns the private listener, applies kernel peer context, bounds headers/body reading/idle time and operation responses, and limits active handlers to two. Its lifetime follows the controller's signal context; shutdown cancels requests and drains under a ten-second bound, force-closing on drain failure. Controller startup joins private service termination before closing management state, and a private listener failure stops the controller.

Validation: affected command/controller race suites passed, Linux command compilation passed, and listener cancellation/error tests passed. Actual systemd activation, positive inherited root-owned listener validation, installer unit/configuration ownership, and deployment remain required.

### Inherited listener credentials and root fixture

A new opt-in disposable root fixture creates a root:1003 0660 listener and passes it as descriptor 3 to UID/GID 1001 without supplementary groups. The child must fail direct filesystem socket access yet successfully consume and accept the inherited listener, clearing activation variables. The parent verifies the unprivileged controller response and observes root through SO_PEERCRED: peer credentials reflect listen-time creation, not the current accepting UID (https://man7.org/linux/man-pages/man7/unix.7.html). CI runs this fixture explicitly as root.

NewActivatedMaintenanceApps therefore provides an explicit client mode authenticating the root creator of the installed systemd socket. NewMaintenanceApps retains its non-root controller peer check for directly created listeners. Both preserve the same restricted request/acknowledgement and owned-job checks.

Validation: local runtimeclient/socketactivation race suites and Linux activation test compilation passed. Native inherited listener execution and systemd/installer deployment remain pending. Previous CI run 36831523945 completed successfully at 8ad8f1e1eb9616bde9fa5af9e32eb31cf0245d59; controller startup additions are included in the upcoming run.

### Owned installer configuration for controller backup socket

Configuration planning now adds homenode-app-maintenance.socket only with an observed valid backup account. The generated unit creates the fixed root-owned backup-group 0660 listener, names its descriptor, selects the controller service, and removes the path on stop. The corresponding controller unit binds the exact UID/GID arguments and explicit socket association/dependency/order. Standard installations retain their unchanged controller template. The installer plan allowlist includes this specific owned socket unit; prepared-installation checks require exact controller and socket bytes when maintenance is configured. Configuration does not start services.

Validation: installer race tests passed before the final explicit association/order refinement, final installer tests passed, and final Linux compilation passed. Plan tests check literal controller identity and socket root/group/mode/descriptor/service values. Disposable root configuration checks and actual systemd startup remain pending native validation.

### Prepared maintenance installation ownership fixture

The disposable root installer fixture now builds a maintenance-enabled configuration and complete modeled ownership journals before checking the prepared installation. Its success case verifies the returned backup identity while preserving the distinction from live account verification. Refusal cases cover a changed controller unit, changed socket permissions/configuration, a missing socket unit, and an unfinished backup-account journal. Existing image/configuration fixtures retain their standard behavior through an optional maintenance fixture parameter. No accounts or services are changed by this fixture.

Validation: local installer race tests and Linux compilation passed; the root-specific prepared-maintenance cases are skipped on the owner Mac and await native CI. Current CI run 36832293129 for the installer implementation remains active.

### Private controller request cancellation on listener failure

The private maintenance server now uses its own cancellable context for every request, cancelling it on listener failure as well as owner shutdown. Admission is synchronized with cancellation; admitted handlers are joined after HTTP shutdown so management state cannot close while trusted maintenance callbacks remain active. A private injected-handler fixture exercises cancellation and listener error with an already admitted blocked request, verifying its context is cancelled and its handler exits before service return.

Validation: controller race tests and Linux compilation passed. CI run 36832293129 completed successfully at 86586e9d1e58ff3092263264571a2f479c8097b4, validating the installer socket configuration implementation. The new root prepared-maintenance fixture and active-request cancellation additions await the upcoming native run. Actual systemd activation and production backup daemon/public workflows remain required.

### Actual systemd private-listener fixture

A new explicitly enabled disposable Linux root fixture creates temporary systemd socket/service units, compiles the activation test into a root-owned executable location, and starts the root-created backup-group listener. The service runs as UID/GID 1001 without capabilities, using NoNewPrivileges and filesystem restrictions. It must receive systemd's actual current-process activation PID/name, fail direct filesystem socket access, and accept through the inherited descriptor. The connecting fixture validates root listen-time credentials, the unprivileged response, and a zero service exit. Cleanup stops both units, removes temporary unit files, reloads systemd, resets failed state, and removes the fixture runtime directory. It never runs on the owner Mac.

Validation: Python syntax inspection, local activation tests, and final Linux test compilation passed. Actual systemd execution is pending the next CI run. This fixture qualifies the socket/service inheritance mechanism, not the complete installed controller or backup workflow.

CI run 36832665192 failed in the new root prepared-maintenance fixture because it passed maintenance-accounts.json to saveJournalBytes, which accepts the canonical journal name maintenance-accounts and appends the extension itself. The fixture now uses the existing canonical API; production validation is unchanged. Native root success remains unproven until the rerun.

CI run 36832958587 passed the root prepared-maintenance configuration tests after the journal-name correction, then failed while starting the new systemd socket fixture. Its captured exception did not retain systemd's startup diagnostics. The fixture now uses Type=simple to match the controller and prints unit/journal diagnostics before cleanup on failure. Actual systemd success remains unproven until rerun.

Additional maintenance app-client tests verify that foreign device ownership, invalid job identity, and failed inspection never send a controller request. Reply cases require exactly version 1 and reject duplicate/extra/null/wrong-version/trailing/oversized/non-UTF-8 acknowledgements. The runtimeclient race suite passed locally.

### Private sanitized management snapshot stream

The controller maintenance handler now supports a snapshot-only operation for the exact owned staging job with attached root authority. It builds a sanitized SQLite backup under a private controller-owned temporary directory, validates recovery authority/inventory, then streams a bounded 256 MiB snapshot with an exact content length and fixed media type. No peer-selected host path or live database file is exposed. Temporary plaintext storage is removed on completion/error, and the existing maintenance barrier remains in place. Pre-stream failures return refusal; partial streams require client integrity validation.

Validation: controller race tests and final Linux compilation passed. A snapshot test validates the streamed database, absence of temporary backup directories, and retained management admission. Production snapshot client/staging integration and a fully frozen configuration/byte-writer boundary remain required.

CI run 36833296588 failed its systemd fixture because systemd could not resolve the nonexistent numeric socket group 1003. The fixture now creates a dedicated named group only after checking both name/GID vacancy and removes that owned group after unit cleanup. The production socket already uses the separately provisioned homenode-backup group. Actual systemd success remains pending rerun.

### Backup-side management snapshot receiver

MaintenanceAppsClient now receives the owned sanitized snapshot through its authenticated private controller transport. It requires the fixed media type and exact positive length up to 256 MiB, creates snapshot.db exclusively in a private staging root, checks exact stream completion, validates the recovery database, and syncs file/directory before success. Failure cleanup removes only the inode created by the call and preserves a pre-existing staging file. Caller staging ownership remains required.

Tests cover invalid database bytes, truncation, excess bytes, incorrect media type, absent/oversized length, and existing-file preservation. The Linux kernel-peer controller fixture now exercises positive snapshot transfer and validation in the owned staging phase. Local affected race tests and Linux controller compilation passed; positive native execution awaits CI. Recovery-set staging still needs to use this receiver instead of direct management database access in the backup process.

### Recovery staging without management database access

StagePrivateRecoverySet now obtains the sanitized management snapshot through a ManagementSnapshots bridge, qualifies app disks through the pinned disk bridge, and confirms the owned staging job/drained inventory before returning a validated manifest. The controller exposes only an owned staging confirmation operation, and the client accepts its strict acknowledgement. Existing local staging shares the same manifest/copy core for focused tests; the backup-side private path requires no management Store handle.

The Linux controller/client fixture now stages and validates a management-only recovery set through the actual Unix socket and tests out-of-phase staging confirmation refusal. This empty app inventory does not exercise VM disk copying; the separate descriptor/ext4/restic fixtures cover those primitives. Local backup/controller/client race suites and Linux controller compilation passed; the new private staging integration awaits native execution. Production coordinator/daemon connection, all-writer freeze, publication outcome, staging disposal, and user approval remain required.

CI run 36833747369 completed successfully at c423757b4edab80889b73022fb28190ebe8e628c. Its inspected log confirms actual systemd activation of the root-created named listener by UID 1001 with direct filesystem access denied and inherited accept succeeding. Root installer checks also passed. This proves the disposable activation mechanism, not the full installed controller/backup workflow.

### Owned cleanup after private staging confirmation failure

Recovery-set failure cleanup now uses the shared inode-ownership cleanup helper instead of silently skipping substituted files. A failed private staging confirmation removes the captured snapshot only while its inode still matches, syncs the staging directory, and clears the returned manifest. A substituted snapshot is retained and reported as ErrManifest alongside the confirmation failure.

A management-only private staging fixture covers successful capture/confirmation, confirmation failure cleanup, and substituted-file preservation/conflict reporting. It models the private management bridge and uses a real sanitized SQLite snapshot; it does not prove socket or VM boundaries. Backup race tests passed locally. Current native CI remains active. Production successful-staging disposal, crash-recovery ownership, and backup publication history remain required.

### Owned private publication transition

The controller now exposes only a staging-to-publishing transition and publishing confirmation for the exact owned job/device with retained root authority and drained inventory. Repeating the transition acknowledgement for the same publishing job succeeds; other phases are refused. The backup client provides corresponding restricted methods. PublishPrivateRecoverySet validates the staged inventory before transition, confirms ownership before repository effects, publishes through the authenticated repository interface, and confirms again afterward. It preserves a successful snapshot ID if that final confirmation fails. Transition replay must not be interpreted as permission to repeat an uncertain repository publication; durable outcome/recovery policy is still required.

Focused tests cover snapshot outcome preservation on final confirmation failure; the Linux socket fixture exercises transition/replay/confirmation and subsequent out-of-phase staging refusal. Local affected race suites and final Linux compilation passed; new native execution awaits CI. CI run 36834304153 completed successfully at f546c5749bace601e3f4bb6e9f52f988663e0f76, including private management-only staging through the kernel-peer controller/client bridge and actual systemd listener activation.

The backup daemon, action approval, durable publication outcome/history, successful-staging disposal, and complete replacement-host recovery remain unfinished.

### Durable one-time publication claim

Private publication now requires a durable controller claim immediately before repository writes. ClaimBackupPublication checks owned job/device, publishing phase, retained root authority, current admin identity, and drained inventory in one transaction before persisting the current job ID. A same-job retry refuses after restart or lost acknowledgement. A later uniquely owned job may replace the prior claim; old tokens cannot authorize it. The claim records intent only, not repository success. The controller/client expose a dedicated typed claim operation, and PublishPrivateRecoverySet cannot reach Snapshot without it.

Recovery snapshots remove the claim, and recovery validation rejects restored claims as authority. Tests verify non-publishing/foreign-job/active-inventory refusal, persistence across reopening the database, and same-job replay refusal. The Linux private socket fixture exercises claim success and replay refusal. Local affected race suites and final Linux compilation passed; the final added active-inventory test also passed in the state suite. CI run 36834789691 completed successfully at 3c6933610a11b13acee2bde9e2a565a96f6c6180. New claim native validation awaits the next run. Durable repository outcome/history and guided recovery of uncertain publication remain unfinished.

### Publication refusal integration correction

CI run 36835262262 failed because the Linux bridge fixture accidentally expected a claim during draining to succeed. The controller correctly refused it. The fixture now explicitly requires draining refusal and retains the later publishing success/replay checks. A separate backup orchestration test proves claim refusal returns its error and no snapshot ID, with zero repository Snapshot calls. Local affected race suites passed. Native Linux execution awaits the corrected CI run; durable repository outcomes and the end-to-end backup daemon remain unfinished.

### Durable publication outcome foundation

The state store now atomically records an unknown repository outcome with a publication claim, then accepts a separately authenticated, job/device-bound full snapshot acknowledgement while publishing authority is retained. Matching acknowledgement replay is idempotent without changing its timestamp; changed snapshot IDs and unclaimed acknowledgements refuse. Published current and last-success records are committed together. A later claim retains the prior last success, and cannot replace an unknown outcome. Existing legacy claims without an outcome also require reconciliation. These records prove publication acknowledgement only, not staging cleanup, repository integrity, or successful application restore.

Bounded typed records reject malformed stored data; recovery snapshots strip both records and recovery validation refuses them. Tests cover restart persistence, acknowledgement replay, foreign identities, invalid snapshot IDs, outcome replacement refusal, uncertain-outcome preservation across a later job, and last-success retention. Affected local race suites passed, including final state changes. CI 36835843927 at 326d662 has passed native Go/adapters and root/systemd fixtures; full run remains active. The new outcome acknowledgement is not yet connected to the private controller/client protocol or backup daemon. Owner recovery/reconciliation UI, backup history, restore-test status, and full lifecycle workflows remain unfinished.

### Private durable publication acknowledgement

The fixed private controller endpoint /v1/maintenance/ack-publish accepts exactly the owned version/token/job/device and a full lowercase repository snapshot ID. Its strict decoder rejects missing, duplicate (including escaped aliases), null, malformed, and trailing fields; snapshot fields remain forbidden on all other maintenance routes. Kernel backup UID authentication and controller job ownership checks precede the state acknowledgement. The client validates snapshot IDs before transport, binds the inspected owned job, and uses the existing bounded authenticated socket transport and strict acknowledgement decoder.

PublishPrivateRecoverySet now records successful repository publication durably before its final ownership confirmation. A failed durable acknowledgement preserves the repository snapshot ID and returns failure; it never reports successful completion or retries repository writes. Focused tests cover acknowledgement failure outcome preservation and invalid client snapshot rejection. The Linux socket fixture checks acknowledgement persistence, idempotent lost-ACK replay, and changed snapshot refusal. Local affected race suites and Linux control compilation passed; native execution of these new bridge assertions awaits CI. The actual daemon dispatch, owner recovery workflow, successful-staging disposal, history, and full replacement-host restore remain unfinished.

### Coordinator publication invariants

The existing RunBackup coordinator previously called Repository.Snapshot directly without the new durable publication claim or acknowledgement. Its publication callback now claims the exact owned publishing job before repository effects, validates the successful full snapshot ID, and records publication through the state transaction before maintenance cleanup. Repository errors retain an unknown durable outcome; staging errors never create publication intent. A successful snapshot remains available alongside acknowledgement or restoration errors. Both direct coordinator and private bridge publication paths now require claims and acknowledgements.

The orchestration fixture refuses repository effects without a matching unknown claim. Scenario tests verify no outcome on staging failure, unknown on repository failure, and matching current/last-success on success and cleanup failure. Local backup race tests passed. Native encrypted restic/qualified-filesystem integration for this change awaits CI. The existing coordinator still uses trusted direct Store access; actual backup daemon dispatch through private IPC, staging disposal, owner reconciliation, and the full replacement-host lifecycle remain unfinished.

### Store-free dispatched backup client and operation

NewOwnedMaintenanceApps and its activated-listener variant bind a client to exactly the dispatched token/job/device without opening the controller database or receiving a Store inspector callback. This static binding grants no authority: each endpoint still validates kernel peer identity and current controller-owned job state. Foreign token/device requests refuse before transport; malformed dispatch IDs, relative sockets, and root UID use through the non-activated constructor refuse. The Linux controller socket fixture now uses this Store-free client binding for its existing staging, publication, and restoration assertions.

RunPrivateBackup connects sanitized management staging, mandatory disk qualification, validation, one-time publication claim, repository publication, and durable acknowledgement through the private interfaces. It accepts an already admitted/frozen job; the coordinator remains responsible for acquiring/reconciling barriers, exclusive staging/repository ownership, app restoration, and disposal. It preserves a snapshot ID alongside acknowledgement errors and never releases maintenance itself. Management-only modeled tests cover success and staging/claim/acknowledgement failures, including zero repository effects before successful staging/claim and retained admission. Local affected race suites and Linux control compilation passed; native execution awaits CI. Actual authenticated daemon dispatch and complete owner/restore workflows remain unfinished.

### Combined private backup Linux integration fixture

The Linux controller socket fixture now invokes RunPrivateBackup through the Store-free job-bound client rather than manually staging and synthesizing publication. The repository fixture validates the management-only staged recovery set, retained publishing/root authority, and matching durable unknown claim before returning its modeled snapshot ID. The actual controller socket, kernel peer checks, snapshot stream, phase transition, claim, and durable acknowledgement are exercised together. Replay using a fresh empty staging root refuses and does not make a second repository call. Existing acknowledgement replay/change refusal and restoration/barrier assertions remain.

Local affected race suites passed (the Linux-specific fixture is excluded on macOS); final Linux compilation passed. Native execution awaits CI. The repository is modeled and there are no app disks or actual VM barriers in this fixture, so it does not prove full restic/guest shutdown/replacement-host recovery or daemon dispatch. Those lifecycle requirements remain unfinished.

### Backup worker dispatch contract

The backup package now defines a versioned controller-to-worker Dispatch containing only owned job/device IDs, management/runtime tokens, release, and catalog version. Its encoder and bounded 1024-byte decoder validate IDs/version/release/floor and reject unknown or missing fields, nulls, duplicate decoded keys including escaped aliases, invalid UTF-8, wrong scalar types, and trailing JSON. Errors do not echo tokens or input. Client-selected paths, commands, repository secrets, and restore policy are absent; those must come from trusted service configuration.

The contract is job data, not authorization. The future dispatch transport must authenticate controller kernel identity, and existing controller/disk endpoints independently check subsequent authority. No dispatch listener or daemon has been added yet. Strict-contract tests and local backup race suite passed. Native combined private backup fixture validation from d813ac4 awaits its active CI run. Complete dispatch, destination/password workflows, disposal/reconciliation, and replacement-host restore remain unfinished.

### Authenticated backup dispatch receiver

ReceiveDispatch accepts one controller-authenticated unixpacket message on a caller-owned accepted connection. It rejects nil/stream sockets, configured root controller identity, and mismatched kernel peer UID before consuming job data. A five-second bound and context cancellation interrupt receipt. It reuses the existing packet transport's truncation checks, atomically close-on-exec received descriptors, and rejection/closure of unexpected descriptors, then applies the strict 1024-byte dispatch contract. Errors do not echo message content. Caller retains connection closure, admission/concurrency limits, and worker lifecycle responsibility.

Linux tests cover matching/foreign/root peer configurations, unexpected descriptor, malformed/oversized message, and cancellation. Linux backup compilation and local backup race suite passed; Linux-specific test execution awaits CI. No listener, dispatch sender, worker daemon entry point, repository/password handoff, or owner approval workflow has been added yet. Full lifecycle and hardware qualification remain unfinished.

### Authenticated controller dispatch sender

SendDispatch encodes the strict job contract, authenticates the configured non-root backup kernel peer on an exclusively owned unixpacket connection, and sends one descriptor-free packet with a five-second/cancellable bound. Root or mismatched peers, invalid jobs, wrong socket types, and cancelled requests refuse. A successful send is explicitly transport evidence only; it cannot stand in for worker admission, publication acknowledgement, app restoration, or cleanup. No automatic retry is introduced after uncertain delivery.

Linux sender/receiver roundtrip tests and refusal tests verify that foreign/root peer configurations, invalid jobs, and cancellation expose no job packet to the receiver. Final Linux backup compilation and local backup race suite passed; Linux-specific execution awaits CI. The installed listener/daemon, activation policy, cross-UID complete worker dispatch, destination/password handoff, and owner lifecycle workflows remain unfinished.

### Backup dispatch server lifecycle

ServeDispatch owns a private unixpacket listener, admits one received/running job, and closes overflow connections without consuming tokens. Each received job uses authenticated bounded reception and a maximum two-hour cooperative work context. Cancellation and fatal listener errors cancel active work and join workers and the listener-close callback before return. Worker errors stop the service and propagate to its caller, requiring coordinator reconciliation rather than an automatic claim of success. Connection closure and the slot are retained for the complete worker call.

Linux fixtures cover cancellation, listener failure, worker failure, worker-exit joining, and overflow closure. Local backup race suite, final focused dispatch/private-backup races, and final Linux compilation passed; native execution awaits CI. CI 36839124196 failed in the previous sender refusal assertion because ReadMsgUnix can return a negative byte count on timeout. The test now rejects positive bytes and requires an actual timeout to prove no packet was sent. The sender still refused all tested requests. No daemon command, installed listener/service, job approval workflow, destination/password handoff, or full recovery lifecycle is finished.

### Dispatched private worker adapter

RunDispatchedBackup now binds a validated Dispatch to trusted installed management/disk socket paths, exact installed release/catalog version, and restore policy floor. It constructs the fixed job-bound private client and authenticated root disk client, calls the qualified private staging/publication operation, and returns the dispatched job identity and any snapshot ID even alongside acknowledgement errors. It opens no live management database. Trusted callers must still authenticate dispatch, derive installed configuration from verified metadata, retain barriers, and supply exclusive staging/repository handles. Root listener UID explicitly selects the known systemd-created listener; it is not discovered by probing an arbitrary peer.

The server now checks cancellation immediately before worker invocation. Refusal tests verify invalid job, release/catalog mismatch, policy floor, invalid paths, and cancellation return no result and leave staging/repository untouched. The Linux controller integration fixture now executes this worker adapter with the real private socket bridge and modeled repository. Local affected race suites, final runtimeclient races, and Linux control compilation passed; native adapter execution awaits CI. CI 36839498106 at 165f1c6 passed, including server cancellation/listener failure/worker failure/overflow and corrected sender refusal assertions. Installed daemon command, socket/service setup, exclusive staging lifecycle, approval/destination/password workflows, and complete recovery remain unfinished.

### Private per-job staging lease

OpenJobStaging now pins a pre-provisioned private parent owned by the running backup UID, holds a persistent-inode exclusive kernel runner lock, and creates a new 0700 job directory using a validated ID. It checks that the opened child and current parent entry still match the created directory. The runner lock helper now supports an already pinned Root, preventing a second path lookup between acquiring the lease and creating staging. Unsafe parent permissions/ownership, lock files, overlapping jobs, and existing job directories refuse.

JobStaging.Close is idempotent, closes the child/parent handles, and releases the lock last without unlinking the lock inode or staged data. Existing staging after crash or acknowledgement uncertainty requires reconciliation/protected disposal instead of adoption. Tests verify overlap refusal, release/new-job acquisition, same-job replay refusal, preserved staged bytes, unsafe parent permissions, and invalid job names. Local backup race suite and final Linux compilation passed; native execution awaits CI. This lease is not yet wired into daemon dispatch. Protected disposal, durable staging recovery records, repository leasing, installed daemon/service setup, and full owner/replacement-host workflows remain unfinished.

### Leased staging integrated into dispatched workers

RunLeasedDispatchedBackup now validates job and installed release/catalog/policy/socket configuration before creating staging beneath the fixed installed private parent. It retains JobStaging through the entire private worker operation and joins close errors without discarding returned snapshot identity. It never disposes data or releases management/root barriers. OpenJobStaging now syncs the pinned parent after creating/verifying the job directory before returning the accepted lease.

Invalid-dispatch tests exercise both direct and leased worker entry points and require no staging/repository effects. The Linux controller fixture now uses the leased worker, and its repository callback explicitly tests overlap refusal while publication runs. It verifies same-job leased replay refusal, later acquisition after worker return, and retained original recovery snapshot. Local affected race suites, final runtimeclient tests, and final Linux control compilation passed; native execution awaits CI. Repository ownership remains a trusted caller requirement, and installed daemon/service dispatch, staging disposal/reconciliation, password/destination setup, and complete replacement-host recovery remain unfinished.

### Repository lifetime kernel lease

Authenticated Repository handles now acquire a nonblocking exclusive kernel lease on the pinned repository directory before running restic config authentication and retain it until Close. Initialization/config qualification also holds the same lease for its complete command. Admission requires a directory owned by the running backup UID with exact private 0700 permissions. Independent HomeNode handles refuse overlap across processes as well as within one process; restic's operation locks remain in use. This cooperative lease does not exclude an unrelated writer that ignores it or a privileged directory replacement.

Linux tests cover independent descriptor overlap, acquisition after owner close, public-permission refusal, and cancellation. The native restic fixture now rejects a second authenticated handle while the first remains open. Final Linux backup compilation passed; local backup race suite passed but excludes Linux-only changes. Native encrypted repository roundtrip and new lease execution await CI. Installed worker repository/credential setup, mounted-drive ownership provisioning, protected staging disposal/reconciliation, and full backup/recovery UI/lifecycle remain unfinished.

### Registered repository lifecycle in the private worker

RunRegisteredDispatchedBackup now validates dispatch/installed policy and configured staging/registered target before opening the external drive. It uses OpenRepository's admitted mount, credential authentication, private directory checks, and retained kernel lease, then invokes the leased per-job staging/publication worker and closes the repository afterward. Returned snapshot/job identity survives close errors. Destination is fixed trusted configuration, not dispatch input; password bytes must arrive through trusted local handoff and remain caller-owned for clearing. No password is added to dispatch or command arguments.

Tests verify invalid target, missing/unregistered drive, release mismatch, invalid staging path, and cancellation refuse without creating staging or reporting a job/snapshot result. Local affected race suites, final runtimeclient tests, and final Linux runtimeclient compilation passed. Complete registered drive + real private controller/disk + encrypted publication integration remains unverified; the new Linux repository lease tests await active CI. Installed daemon/credentials/drive ownership provisioning, approvals and UI, disposal/reconciliation, and replacement-host recovery remain unfinished.

### Sealed credential handoff and worker consumption

CreateRepositoryPassword exposes the existing anonymous sealed memory descriptor producer. ReadRepositoryPassword accepts only a close-on-exec regular descriptor with zero links, all immutable write/grow/shrink/seal seals, and a 1–8192 byte payload. It reads at offset zero without moving shared offset, rejects prohibited NUL/newline bytes, and clears temporary data on failure or cancellation. Ordinary disk files, pipes, and mutable memory descriptors refuse; unsupported platforms fail closed.

RunCredentialedDispatchedBackup explicitly consumes/closes its credential descriptor on every path, validates installed job/target/staging configuration before reading it, invokes the registered leased worker, and clears its temporary password copy afterward. Ordinary dispatch packets still reject descriptors/secrets; actual credential handoff must independently authenticate the local parties. Linux tests cover valid immutable read/offset preservation, disk/pipe/mutable descriptor refusal, invalid sealed payload, and descriptor closure on worker refusal. Local affected race suites and final Linux compilation passed; Linux-specific execution awaits CI. No credential transport, owner password UI, installed daemon command/service, or complete recovery lifecycle is implemented yet.

### Separate authenticated sealed credential dispatch

The credential producer now returns a read-only close-on-exec descriptor reopened through its own descriptor reference, after verifying inode identity with the original sealed memory file. This lets the existing descriptor transport enforce read-only admission without weakening seal validation. SendCredentialDispatch and ReceiveCredentialDispatch provide a separate authenticated unixpacket handoff containing one strict job message plus one immutable anonymous descriptor, with five-second/cancellation bounds. Sender retains its source handle; rejection closes the received duplicate; successful receipt transfers closure responsibility to the worker. Temporary validation copies are cleared.

Ordinary dispatch still rejects all descriptors. Linux tests cover sealed read-only transfer, sender-handle retention after receiver close, ordinary-route credential refusal, foreign controller identity, malformed job, and regular disk credential refusal. Local affected race suites and final Linux compilation passed; native execution awaits CI. A dedicated credential listener/server, cross-UID handoff qualification, installed daemon/service, owner password/approval UI, and full recovery lifecycle remain unfinished. Dispatch delivery is never treated as publication success.

### Credential server lifecycle and registered worker binding

ServeCredentialDispatch now reuses the ordinary server's private unixpacket ownership, single-job admission, overflow closure, bounded work context, worker-failure stop, cancellation, and joined shutdown. It authenticates and receives credential jobs through the separate sealed handoff receiver and closes received descriptors on every post-receipt exit, including pre-work cancellation and worker failure. Callbacks may consume credentials earlier; the server's fallback closure does not preserve any received handle after callback return. Ordinary dispatch keeps its descriptor-free receiver.

ServeRegisteredBackupWorker binds admitted credential jobs to RunCredentialedDispatchedBackup and trusted installed configuration, preserving repository/staging lease and credential-clearing behavior. Publication evidence remains the durable controller outcome. Linux server tests verify cancellation/listener failure/worker failure, worker joining, received descriptor closure, and sender-handle retention. Local affected race suites and final Linux compilation passed; native execution awaits CI. Actual cross-UID installed service activation, daemon command/config loader, public approvals/password UI, coordinator dispatch/reconciliation, and full replacement-host recovery remain unfinished.

### Credential-bearing overflow admission coverage

The credential server lifecycle fixture now attempts a second credential-bearing dispatch while the first worker is active. Either send outcome is permitted because server admission can close before or after the write; the receiver side must close promptly without returning data or invoking a second worker. The atomic callback count remains one, and existing post-shutdown sender-handle and received-descriptor closure checks remain in each cancellation/listener-failure/worker-failure scenario.

Final Linux backup compilation and focused local dispatch/private-backup race tests passed. The new fixture is Linux-only and native execution awaits CI. This is admission coverage, not proof of cross-UID installed activation or complete daemon/owner/recovery workflows. Those original lifecycle requirements remain unfinished.

### Named private packet socket activation

TakePrivatePacketListener now consumes exactly one named systemd descriptor with the existing PID/fd-count/name validation and environment clearing, root-owned 0660 socket/group admission, protected root-owned parent, listening state, exact path, and ownership transfer checks. It explicitly requires SOCK_SEQPACKET and a unixpacket listener; stream activation retains its separate SOCK_STREAM check. Unsupported platforms refuse both entry points.

The disposable root inherited-listener fixture now has a packet variant: a root-created protected socket is inherited by UID 1001 without filesystem group access, and the child must accept through the descriptor while client kernel identity still reports the root creator. CI explicitly runs both stream and packet variants. Local socketactivation race suite and final Linux compilation passed; native packet acceptance awaits CI. This is inherited packet activation qualification, not actual systemd backup daemon activation or credential handoff through installed cross-UID roles. Daemon command/configuration, service ownership/activation setup, public approvals/UI, and full recovery remain unfinished.

### Root-created activated credential sender and cross-UID fixture

SendActivatedCredentialDispatch explicitly authenticates the root creator of the known installed systemd credential listener before sending a sealed job credential. Ordinary SendCredentialDispatch continues to reject configured root peers and never falls back after refusal. This matches the already qualified SO_PEERCRED listen-time creator behavior; the accepting worker remains unprivileged. Callers must use the trusted configured protected listener.

A new opt-in disposable root Linux fixture creates a root-owned controller-group-only packet listener, passes it to worker UID 1003 with no supplemental groups, and launches controller UID 1001. The worker must fail filesystem dialing but accept via named inherited activation, authenticate controller UID 1001, validate/close the sealed received credential, and acknowledge the fixture handoff. The controller requires ordinary-sender root-creator refusal followed by explicit activated-sender success. CI runs this fixture separately under disposable root. Local focused backup race tests and final Linux compilation passed; native cross-UID execution awaits CI. CI 36844111259 at a1db63c passed both inherited stream/packet listener fixtures and existing systemd stream activation. Installed backup daemon/systemd service, owner approvals/password UI, actual dispatch/reconciliation, and full restore lifecycle remain unfinished.

### Backup worker executable entry point

cmd/homenode-backup now parses trusted service launch configuration for controller UID/GID, distinct absolute private socket paths, private staging parent, registered external target, installed release/catalog version, and recovery floor. Startup requires Linux, a non-root system UID/GID in the provisioned range, distinct controller identity/groups, and no supplemental groups beyond the worker primary group. It consumes exactly the named root-created packet listener before application-file access and runs ServeRegisteredBackupWorker with signal-driven joined shutdown. Passwords are not accepted as arguments or configuration fields; they arrive through the sealed credential channel.

Configuration tests cover missing settings, unexpected args/password flag, root controller, relative/alias socket paths, invalid release/floor, and cancellation before activation. Initial compilation found an unused import, which was removed; final startup/runtimeclient race tests and final Linux executable build passed. The launch values are trusted root/service inputs and still require a verified installed configuration source; this command does not independently verify signed release metadata. It is not yet included in the Debian payload or installed service units, and native executable startup qualification remains pending. CI 36881839173 at aa6b0ee passed the activated credential cross-UID fixture. Coordinator/public approval dispatch, password/destination UI, disposal/reconciliation, and full replacement-host recovery remain unfinished.

### Development package backup worker payload

The Debian builder now includes `/usr/lib/homenode/homenode-backup` with root-owned executable permissions and includes it in the payload checksum manifest. It declares the host `restic` dependency required by the fixed repository runner. Package CI checks that the extracted worker is executable, is an amd64 Linux ELF, and that package metadata declares restic; the existing checksum check covers the new binary. Ownership documentation explicitly retains the inactive provisioning boundary.

Local shell syntax validation, a static Linux amd64 worker build, ELF inspection, and diff whitespace validation passed. Actual Debian build/install checks for this change await Linux CI. Backup socket/service installation, isolated account provisioning, trusted release/repository configuration, controller dispatch and owner workflows remain unfinished. Including a worker in an unsigned development package does not qualify production backup or release deployment.

### Provisioned private backup staging

Configuration preparation with an observed maintenance identity now journals a root-owned `/var/lib/homenode-backup` parent and a backup-owned mode 0700 staging directory. The directory admission allowlist permits only this specific staging path with private permissions and system UID/GID in the provisioned 100–999 range. Existing ownership-conflict and resume behavior applies; it does not adopt existing unowned content. Configuration without a maintenance identity does not request these paths.

The initial race run identified the missing installer path admission; after adding the bounded rule, the installer race suite passed. Configuration coverage checks both directory identities/permissions. The disposable root prepared-install fixture additionally changes staging to public permissions and requires refusal; native execution of that case awaits CI. Worker service/socket configuration, credential dispatch and owner-facing backup/recovery workflows remain unfinished.

### Actual systemd packet activation qualification

The disposable systemd activation fixture now supports an explicitly selected packet transport in addition to stream. Packet mode installs `ListenSequentialPacket` with the exact `homenode-backup-credential` descriptor name, passes the mode to the unprivileged receiver test, and connects with SOCK_SEQPACKET. Both modes require filesystem dial refusal, named inherited acceptance, root creator credentials, bounded response, clean service exit, and fixture cleanup. CI runs the two variants sequentially after compiling the native receiver.

Python AST validation, local socketactivation race tests, and diff validation passed. Actual systemd packet execution awaits Linux CI. This qualifies the systemd descriptor handoff, not the complete installed backup worker or encrypted external repository lifecycle. CI 36883193019 at 213432c completed successfully, including all existing Linux and browser checks. Packaging/staging commit 0212adc has its own CI 36885051090 still running; new packet qualification is not included in that run until pushed.

### Prepared staging identity requirement

Prepared installation checks now require both backup directory records when the journal-bound maintenance account is enabled, bind staging UID/GID to that account, and enforce root parent/private staging permissions. They reject duplicate, missing, foreign-identity, or unenabled backup directory records instead of treating absent journal paths as verified. Existing filesystem matching still verifies actual ownership and modes. Older maintenance configurations without staging need an explicit configuration migration; no directories are silently adopted.

Local installer race tests and diff checks passed, including omitted/duplicate/foreign staging record cases and absence without a maintenance identity. Native prepared-install and package validation remains pending in CI. Installed worker activation, trusted target/release configuration, coordinated dispatch, public approvals and complete recovery remain unfinished.

### Administrator publication outcome endpoint

GET `/api/v1/backups/outcomes` now exposes the bounded durable current and last-published records through the existing administrator session gate, host checks and no-store response policy. It grants no maintenance authority. Missing evidence is null; malformed evidence returns a generic 503 without raw state or partial publication records. An unknown current publication remains separate from an earlier acknowledged snapshot. This endpoint describes publication only, not cleanup, repository integrity or successful restoration.

Control race tests passed for unauthenticated and non-admin refusal, absent evidence/no-store, corrupt evidence refusal without disclosure, and unknown current publication alongside earlier success. Browser integration, drive/password enrollment, approved backup launch, reconciliation, restore test status and complete recovery remain unfinished. Native CI for prior packaging/staging remains running; this endpoint awaits its subsequent CI run.

### Browser backup publication status

Administrator Settings now loads the durable publication endpoint and shows missing acknowledged backup evidence, an uncertain current outcome, earlier acknowledged publication time, and last-observed time. Refresh clears prior evidence while loading and on error, so failed observations do not present stale success. Request sequence tracking ignores late responses after navigation or newer refreshes. Non-admin Settings omits this panel. The panel explicitly distinguishes publication from repository/restore health and states that launch/recovery controls remain unavailable.

The frontend production build passed. The first local browser attempt could not launch because its matching Playwright binary was absent; after installing the matching Chromium build, the full identity browser workflow passed. Added coverage exercises real empty status, modeled uncertain-plus-earlier-publication rendering, modeled 503 without stale publication, and paired non-admin panel omission plus actual endpoint refusal. Mocked outcome rendering does not prove encrypted publication or restore. Actual service activation, approved launch, destination/password setup, repository health and replacement-host recovery remain unfinished.

### Inactive isolated backup service templates

Development packaging now includes a named root-created controller-group credential packet socket and an unprivileged backup service template. The service explicitly receives its socket, uses trusted root-provided launch values, fixes the external mount at `/mnt/homenode-backup`, drops capabilities, restricts network families to AF_UNIX, denies block-device access, isolates host application state, and bounds memory/tasks. Automatic restart is disabled because publication uncertainty requires reconciliation. Device metadata visibility is retained for registered UUID checks; PrivateDevices would hide the required identity. Root-owned verified configuration generation remains a provisioning requirement.

The package copies socket templates, embedding includes socket sources, and Linux package CI verifies all service/socket/slice sources together after binary installation. Local installer and backup-command race tests, shell syntax and diff checks passed. Actual systemd source verification and worker confinement/startup await native CI; these templates are inactive and are not yet promoted by installation. The documented development memory ceiling still requires measured host reserve qualification. Root configuration generation, repository registration, owner approval/credential dispatch and complete recovery remain unfinished.

### Native backup command activation fixture

A new explicit disposable-root Linux fixture launches the backup command startup path as UID/GID 803 with no supplementary groups and an inherited root-created controller-group packet listener. It supplies the validated launch settings, exercises named activation and the registered worker binding, requires prompt refusal of a root peer that does not match controller UID 351, and verifies cancellation returns after joined server shutdown with cleared activation environment. It never admits a repository job or touches a real destination. CI runs this fixture separately.

Final Linux test-binary compilation, local command race tests and diff validation passed. Privileged execution remains pending on disposable CI; no root fixture was run on the owner's Mac. This verifies startup/foreign-peer/shutdown behavior, not installed systemd service confinement, authorized repository publication or owner recovery workflows. Those requirements remain unfinished.

### Journaled backup worker deployment sources

Maintenance-aware configuration preparation now installs the embedded backup service and credential socket through the existing ownership journal. The path allowlist admits those exact unit names. Prepared checks require their unchanged embedded content in addition to staging/account bindings and the existing controller management listener. Configuration preview and prepared reports explicitly retain external repository qualification and trusted backup launch configuration/activation as pending work; no socket is enabled and no password/configuration is invented.

Final installer race tests and diff validation passed. Native ownership/resume and unit syntax validation awaits subsequent Linux CI. Old maintenance configurations require an explicit migration rather than journal deletion or unowned unit adoption. Root launch configuration generation, registered media workflow, fresh owner approval/credential dispatch, publication reconciliation and replacement-host recovery remain unfinished.

### Overview backup attention

Administrator Overview now includes the same publication-status panel as Settings, independently of the host report result. Missing external publication evidence and an uncertain current outcome are visible on the landing screen, with manual refresh and last-observed time. Non-administrator Overview omits backup details. Publication still does not attest repository integrity or restoration, and backup launch/recovery controls remain unavailable.

Production frontend build and the final identity browser workflow passed. Coverage checks the real empty publication state on owner Overview, non-admin Overview omission, and the existing Settings unknown/error/access cases. Native Linux packaging/staging CI remains live in prerequisite installation; subsequent deployment changes await their own CI. The complete goal remains unfinished.

### Publication observation claim consistency

Outcome inspection now requires the current typed publication record to match its durable claim. A legacy claim without an outcome, an outcome without a claim, or a mismatched claim refuses inspection and returns no partial records. Owner status therefore reports unavailability instead of converting legacy uncertainty into absent evidence. Earlier acknowledged history remains distinct from current publication authority; this change does not reconcile or retry repository effects.

State and control race suites passed, including orphan, legacy and mismatch cases and the updated administrator unknown/earlier-publication fixture with its matching claim. Diff validation passed. Native Linux CI for previously pushed packaging/staging remains active; later queued deployment and owner status changes remain unpushed until it reaches a terminal result. Complete launch/reconciliation/recovery workflows remain unfinished.

### Linux CI allowance for observed prerequisite latency

CI 36885051090 completed prerequisite installation successfully from 15:32:33 to 15:51:51 UTC, consuming over 19 minutes before the native suite. The existing 20-minute whole-job ceiling cannot accommodate that observed provisioning latency plus required qualification. Subsequent Linux checks use a bounded 45-minute ceiling, retaining every test and package/browser gate. This changes observation capacity, not implementation or qualification success. The current run remains active and its result must still be inspected.

### Credential worker completion handoff

ServeAcknowledgedCredentialDispatch adds a distinct successful-callback completion response bound to the exact job ID. It closes the received credential before emitting the descriptor-free response; callback or response failure stops the service through the existing joined failure path. Ordinary credential dispatch server behavior remains separate. The registered backup worker now uses this acknowledged mode, so repository/staging/temporary-password cleanup inside its callback precedes the completion response.

SendActivatedCredentialDispatchAndWait authenticates the trusted root-created activated listener through the existing sender, sends once, and waits within the caller deadline capped at two hours for the exact completion packet. It never retries and rejects mismatched packets or unexpected descriptors through the shared transport. Completion does not prove durable publication, repository health, or successful restoration. Delivery/response failure requires owned-job reconciliation before barrier release/replay.

Local backup/runtimeclient/command race suites passed, final Linux backup test compilation and diff checks passed. A Linux fixture checks job-bound completion after received descriptor closure, sender-handle retention, and joined shutdown; native execution awaits CI. Controller coordinator integration and crash/lost-response reconciliation remain unfinished, along with owner launch/drive/password and complete replacement-host recovery.

### Controller orchestration for completed isolated workers

RunDispatchedMaintenance now owns the controller runner lease and management job, drains apps, freezes the root runtime through a stable job ID, attaches root ownership, and constructs the bound credential dispatch. Its injected delivery operation must send once to the trusted activated worker and await exact completion. After completion it requires matching durable published outcome and the owned publishing/root checkpoint before normal bounded restart/release cleanup.

Any error after delivery is attempted conservatively retains app/root barriers and moves the owned checkpoint to requires-action. This includes lost completion after acknowledged repository publication; publication alone never proves worker termination. Pre-dispatch failures use the existing bounded cleanup/reacquisition path. Caller credential ownership is retained and temporary validation password bytes are cleared. No public route invokes this coordinator yet.

Local backup race suite passed and the final Linux test binary compiled. New Linux orchestration fixtures model delivery failure, missing publication, lost completion after publication, and successful completion/publication, asserting retained barriers or cleanup accordingly plus caller credential ownership. Native execution awaits subsequent CI. The current cc8a761 run has advanced to Go/Linux tests. Fresh owner admission, actual socket delivery binding, explicit reconciliation, trusted launch configuration, registered media UI and complete restore remain unfinished.

### Native package checker dependency correction

CI 36887914531 at cc8a761 failed because the runner did not provide rg for the new package assertions. Debian creation and all payload checksums passed, including the worker and unit templates; package installation/unit verification and browser checks were not reached. The native Go/installer/activation fixtures passed. The two package assertions now use grep available on the base runner, retaining ELF architecture and declared restic dependency checks. New dispatcher work below awaits its own native run.

### Controller activated credential socket dispatcher

ActivatedBackupDispatcher now implements the coordinator delivery binding using installed socket/controller-group configuration. It rejects invalid paths/groups, invalid jobs and cancellation before dialing, requires a root-owned 0660 controller-group socket under a root-owned parent without unprivileged write access, dials with a three-second connection limit, and performs explicit root-creator credential dispatch plus exact worker completion waiting. It never retries or adopts a public/request-selected target. Non-Linux socket admission fails closed; credential ownership stays with the caller.

Local runtimeclient race tests passed for invalid configuration/cancellation and retained credential handles; final Linux test compilation and diff validation passed. A new explicit disposable-root socket admission fixture checks protected acceptance and rejection of public permissions, foreign ownership/group, writable parent and symlink alias. CI invokes it separately. Native fixture execution, actual cross-UID acknowledged coordinator delivery and public fresh approval/drive/password workflows remain unfinished.

### Durable isolated worker dispatch checkpoint

The controller now records uncertain dispatch before attempting socket delivery. Completion is recorded only after the exact worker response plus atomically verified owned publishing checkpoint and matching acknowledged publication/claim. Recovery cleanup, transition to restoration, root release, and maintenance completion refuse an uncertain/foreign/corrupt dispatch record. New jobs refuse orphan dispatch records. Completing the owned job removes its completed dispatch checkpoint atomically; source recovery snapshots remove dispatch authority and recovery validation rejects retained markers.

State restart coverage proves uncertain intent persists, blocks restoration, and cannot be cleared by publication alone; completion then permits ordinary release and a later job. Snapshot coverage confirms exported dispatch authority is absent while live intent is preserved. Local state/backup/control/runtimeclient race suites and final Linux backup test compilation passed. The Linux coordinator fixture now also attempts RecoverMaintenance after uncertain delivery/lost completion and requires retained barriers; native execution awaits later CI. Actual worker termination/repository reconciliation for uncertainty, fresh owner admission and complete recovery remain unfinished.

### Orphan dispatch admission quarantine

General transactional workload admission and legacy EndMaintenance now refuse any retained backup dispatch checkpoint, including malformed, uncertain or completed orphan records. The new-job-specific duplicate admission query was removed in favor of the shared guard. Normal owned maintenance completion still atomically removes its completed checkpoint and reopens admission; no new repair/adoption path is introduced.

State/control/workload race suites and diff validation passed. New cases model orphan checkpoint values, require legacy release rollback to preserve its barrier, then remove that barrier in the fixture and require ordinary admission plus both maintenance entry points to remain blocked. Current 5acbd19 native CI is still installing prerequisites. Explicit termination/publication reconciliation and owner launch/recovery workflows remain unfinished.

### Atomic publication and worker-completion observation

Administrator backup status now reads publication and dispatch completion in one database transaction. It exposes only a completion enum, alongside current/earlier publication records, and validates any checkpoint against the owned maintenance job and expected phase. Malformed/orphan checkpoints return no partial status. The UI shows paused-work reconciliation when completion remains uncertain even after repository publication was acknowledged; publication never obscures worker uncertainty.

Final state/control race tests, production frontend build and identity browser workflow passed. State cases observe restart uncertainty, acknowledged publication with uncertain completion, completed worker, and orphan refusal. Browser rendering coverage models published data with uncertain completion and retains existing missing/error/access checks. Native 5acbd19 CI remains running. Public launch, trusted registration/configuration, explicit worker termination/repository reconciliation and full recovery remain unfinished.

### Cross-UID completion sender qualification

The disposable root credential fixture now uses SendActivatedCredentialDispatchAndWait under controller UID 1001 after ordinary non-root sender refusal. Worker UID 1003 consumes named inherited activation, authenticates the controller, validates and closes its sealed credential, then emits the production job completion packet. Matching completion succeeds with sender credential ownership retained. A second explicit fixture changes the response job ID and requires ErrManifest; CI runs both variants.

Final Linux backup test compilation, focused local credential/dispatch race tests and diff validation passed. Actual privileged matching/mismatched completion execution awaits subsequent Linux CI. This covers completion transport across kernel identities, not full repository/guest coordination or owner recovery. Current 5acbd19 CI remains live in prerequisite installation.

### Completed worker checkpoint restart boundary

The state dispatch fixture now closes and reopens SQLite after persisting verified worker completion, then requires the completion observation, cleanup admission, root release, atomic checkpoint removal and later job admission to remain valid. The same fixture already restarts with uncertain intent and refuses cleanup/publication-only release. This exercises both sides of the crash boundary without treating publication as worker termination.

Final state race suite and diff validation passed. Native 8980109 CI remains in prerequisite installation. Complete physical worker/repository termination reconciliation and owner launch/recovery flows remain unfinished.

### Approval transaction maintenance admission

BeginMaintenanceJobTx now allows fresh approval consumption, controller admission closure and durable maintenance job identity to share one SQLite transaction. The existing standalone entry delegates to the same implementation. The transaction caller must roll back failures and wait for commit before using returned authority; no root or worker effects occur during admission.

The backup.create approval integration test injects an admission write failure, verifies the grant remains reusable and admission stays open, retries successfully, verifies the exact persisted owner/job and closed admission, then rejects grant replay. State, identity and backup package tests passed locally. Full Linux CI for b320931 completed successfully. This transaction primitive does not yet expose backup launch: registered target configuration, coordinator resumption and the owner route still need implementation.

### Coordinator takeover of approved admission

RunAdmittedDispatchedMaintenance accepts the exact already-committed draining job instead of creating a second admission. The shared coordinator acquires the exclusive runner, checks token/job/device/phase/root ownership before registering cleanup or producing external effects, and retains existing completion/publication uncertainty behavior. Advanced jobs are refused for explicit recovery rather than repeated dispatch.

The Linux coordinator fixture now exercises admitted jobs across publication success, failed delivery, missing publication and lost completion. Before valid takeover it rejects wrong tokens, jobs, devices and advanced phase without releasing barriers or dispatching; successful takeover preserves the original job ID. Local backup/state/identity tests and Linux test compilation passed; actual admitted-path Linux execution awaits CI. Owner route, trusted target launch configuration and full external-drive/guest qualification remain incomplete.

### Current authority at admitted-job takeover

Admitted coordinator takeover now verifies the current administrator device and absence of any dispatch record in the same transaction as checking token, job and draining phase. Previously committed approval does not authorize a revoked device or a device that lost admin capability. A malformed, uncertain or completed dispatch marker cannot be adopted as a fresh draining job. Refusal does not change retained authority or reopen admission.

Portable state fixtures verify current-owner success, revocation, capability removal and all three marker classes with unchanged journal/barrier evidence. State, backup and identity race suites passed; Linux backup test compilation passed. Current d7d8544 CI is live; this does not establish owner launch or physical backup qualification.

### TUF metadata download boundary

Pinned upstream go-tuf v2.4.2 after inspecting its updater/config/fetcher implementation. The product plan requires TUF for release trust; Debian version labels and SHA256SUMS do not supply that trust. Added an operation-scoped metadata fetcher implementing the upstream interface: fixed HTTPS repository origin/path, TLS 1.3 minimum, no ambient proxy, no redirects or compression, 30-second request bound, cancellation propagation, 16 MiB hard metadata ceiling plus each caller's narrower bound, and typed HTTP status errors needed for root rotation. It is deliberately not a package/image downloader.

TLS HTTP fixtures verify exact-bound success, streaming overflow, redirect refusal without destination access, compression refusal, repository/path/size rejection, typed root-rotation 404, cancellation during a stalled body and canceled-operation refusal. The update package race suite and full local Go suite passed. The fetcher is not yet wired into a TUF updater, trusted cache or installer: root bootstrap/rotation, release targets, disk streaming, install journals and rollback qualification remain required. Current c586b65 Linux CI remains live; this dependency change still needs its own Linux CI run after that run completes.

### Metadata cache durability gate

Added syncMetadataCache for the pinned private directory used by a successful TUF refresh. With exclusive updater ownership supplied by its caller, it checks current effective ownership and directory mode, bounds file count/individual size/aggregate bytes, requires root/timestamp/snapshot/targets, rejects peer-writable files, symlinks, hardlinks, FIFOs, directories and uncommitted temporary files, flushes each regular file, confirms inode identity and flushes the directory before returning. Canceled operations do not gain installation authority. Signature/root selection and safe directory opening remain caller requirements; this helper alone does not verify metadata.

Update race fixtures cover a valid complete cache and each refusal above, and Linux test compilation passed. Actual TUF session/bootstrap/rotation, protected cache locking, release target lookup, installer wiring and physical power-loss testing remain incomplete. Both new update commits stay local while c586b65 CI remains live, avoiding cancellation of its native qualification run.

### Exclusive update cache ownership

Added lockMetadataCache over an already-provisioned pinned private update directory. A persistent zero-length 0600 lock lives outside the metadata directory, uses nonblocking kernel flock, and is retained after close. Parent and metadata ownership/mode and inode identity are checked; symlink/hardlink/FIFO/public/content-bearing lock files and linked/public metadata directories are refused. Cancellation before acquisition produces no lock. No existing unsafe provisioning is repaired or adopted.

Update race tests prove concurrent exclusion, same-inode reacquisition and the unsafe/canceled cases; Linux update test compilation passed. This is a prerequisite for the TUF session, not yet a running updater. The current trusted root selection, cache durability gate and bounded fetcher still need to be combined with upstream verification and real signing/rotation fixtures. Existing c586b65 Linux CI remains active, so pending update commits have not been pushed yet.

### TUF metadata verification session

Added a Linux metadata-only session combining the operation-scoped fetcher, exclusive private cache and upstream go-tuf updater. Initialization uses only the previously provisioned/current protected root.json, never an automatically substituted bootstrap root. An open directory descriptor anchors upstream path operations through procfs; no package download or install API is exposed. Root rotations/delegations and metadata sizes are bounded. Refresh success requires complete cache durability; failed refreshes attempt bounded independent-context flushing of partial root/timestamp evidence before ownership is released. Target lookup flushes delegated metadata and returns an independent copy of the verified descriptor.

Linux fixtures compile for missing, invalid and linked persisted roots, requiring failure, unchanged root state and released cache ownership. Portable update race suite passed and final Linux update compilation passed. Actual Linux session execution, independently signed repository fixtures for role thresholds/root rotation/expiry/rollback, trusted root provisioning, clock rollback policy, package streaming and installer integration remain unfinished. Current c586b65 CI remains live and pending update commits remain local.

### Signed TUF repository restart fixtures

Added a Linux TLS repository fixture with independently generated Ed25519 keys for root, timestamp, snapshot and targets. It advances the trusted root from version 1 to 2, refreshes signed version-2 roles, looks up a target, verifies descriptor copies cannot alter trusted state, closes/reopens the session and requires version-2 root persistence without re-requesting version 2 from an old bootstrap. It then replays a signed older timestamp, supplies expired metadata and uses the wrong role signer; each case must reject without replacing trusted timestamp evidence or retaining cache ownership.

The session's private initialization helper permits a test HTTPS transport while production initialization still constructs its fixed-policy fetcher. Final Linux test compilation and portable update race suite passed; actual signed-fixture execution awaits Linux CI. This tests root version advancement using the same root key, not root key replacement/threshold transitions, physical crash durability, package acquisition or installer activation. Existing c586b65 CI remains live; update commits remain local pending its completion.

### TUF root key replacement fixture

The signed repository fixture now generates a new root key for version 3. A root signed only by its replacement key must fail without replacing version-2 trusted root evidence. The same root signed by the previously trusted root key and its replacement key must succeed, persist and initialize another session after restart. Subsequent timestamp rollback/expiry/wrong-role cases then execute under the replacement root.

Final Linux update test compilation and diff validation passed. Actual execution still awaits the next Linux CI; current c586b65 CI has passed its Go/Linux adapter step and is running the root installer fixture. This does not yet cover multi-key threshold changes, clock rollback, install rollback or package acquisition.

### Persisted TUF reference clock

The verification session now durably records its exact upstream TUF reference time before refreshing remote metadata. The private checkpoint uses a bounded canonical nanosecond timestamp, exclusive pending-file creation, file sync, atomic rename and directory sync. A reference earlier than its retained checkpoint is refused. Unsafe/linked/noncanonical clock files, stale pending writes and a missing clock alongside previously refreshed metadata require explicit recovery; the baseline is never silently reset. This detects observed backward time, not a frozen clock or hostile root rewriting private state.

Portable update race tests verify restart persistence, one-nanosecond rollback refusal with unchanged checkpoint, equal/forward time, malformed/public/linked/hardlinked evidence, lost clock, pending intent and cancellation. A failing dangling-symlink fixture led to explicit Lstat/inode checks before treating a checkpoint as absent. Final update race suite and Linux test compilation passed. HomeNode checks 36903122988 at 64f8b9b have passed Go/Linux adapters including the signed repository/session fixtures and privileged fixtures; the run is still live at go vet. Guest-image run 36903123198 is a separate workflow. Clock integration awaits the next Linux CI; install/release promotion remains unfinished.

### Streamed verified package acquisition

AcquirePackage now obtains target evidence from the TUF session and streams a Debian target from a fixed HTTPS target repository on the same configured origin. Consistent snapshots use the SHA256-prefixed target name. Package staging requires a private owned pinned directory, bounds package size to 512 MiB and checks target length plus a 2 GiB free-space reserve before starting. Requests are cancellation-aware, bounded to ten minutes, reject redirects/compression and require exact length plus SHA256 and any declared SHA512; unsupported hash algorithms fail closed. Exclusive pending files retain failed intent. Successful files are synced, made read-only, published with RENAME_NOREPLACE and a directory sync, then returned through a read-only descriptor with matching inode identity. No installer authority is returned.

Linux TLS fixtures compile for valid content, corruption, streaming overflow, truncation, redirects, public staging and existing pending/verified paths. They require failed acquisition to return no descriptor and publish no new verified artifact. Final Linux update compilation and portable update race suite passed; actual package fixtures await subsequent Linux CI. Reserve checks are observations, not atomic filesystem reservations. Package platform/release policy, acquisition journal/cleanup, trusted service configuration, installer integration and power-loss qualification remain incomplete. Current 9e2bfeb CI is still live; this commit remains local until it finishes.

### Package cancellation boundary

Added a Linux TLS fixture that stalls after headers, cancels the package operation and requires bounded completion with context.Canceled, no verified descriptor/artifact and refusal to reuse the canceled operation. Final Linux update test compilation and diff validation passed; native execution awaits the next CI run. HomeNode checks 36903805096 at 9e2bfeb completed successfully, including the integrated reference-clock guard and signed TUF fixtures, privileged fixtures, package and browser workflows.

### Signed release compatibility policy

Defined the signed TUF package custom-metadata contract in docs/RELEASE_TARGETS.md. Package acquisition now checks exact Ubuntu 24.04 amd64 platform, independently supplied positive release-sequence/catalog floors and current state-schema compatibility before any package request. Source/result schema declarations cannot authorize a data downgrade. Distinct bounded SHA256 SBOM/provenance targets must resolve from verified TUF metadata. The flat custom object rejects repeated decoded keys/escaped aliases, case aliases, unknown fields, trailing values and oversized data; release/evidence paths reject injection/traversal.

Portable update race tests exercise valid policy, platform/sequence/catalog/schema incompatibility, unsafe/repeated evidence references, release injection and ambiguous JSON. Final update race suite, Linux update test compilation and diff validation passed. Evidence contents/promotion/scans, trusted policy persistence, actual installer integration and qualified hardware remain incomplete. Package acquisition CI 36918945508 at bc3232c remains live in dependency installation, so this policy commit remains local pending its completion.

### Verified release evidence acquisition

Package acquisition now obtains the referenced SBOM and provenance bytes through the same fixed-origin target fetcher, using consistent snapshot names, bounded target length and upstream TUF hash verification. Invalid JSON or mismatched/truncated evidence is refused before package acquisition. AcquiredRelease holds the read-only package, compatible release metadata and both evidence byte sets for later semantic review; it does not grant installation authority. Updated the release contract to distinguish verified bytes from verified SBOM completeness/build identity/vulnerability status.

Portable TLS race fixtures exercise correct consistent target lookup, exact content, corrupted/truncated evidence, invalid JSON, traversal, missing SHA256 and oversized evidence. Final update race suite and Linux compilation passed. Package acquisition CI 36918945508 at bc3232c has passed Linux package fixtures and privileged checks and is currently building the Debian package. This evidence change remains local with the pending release-policy commit until that run finishes. Semantic evidence validation, promotion, acquisition journals and installer integration remain unfinished.

### Signed release acquisition integration fixture

The signed TUF repository fixture now publishes a package target with release custom metadata plus distinct hash-bound JSON evidence targets. It exercises the production acquisition path through a fixture HTTPS transport: refreshed TUF descriptors, release floors/schema, evidence downloads and a streamed read-only package. It checks the returned package/metadata/evidence bytes, requires a higher security floor to refuse without target requests, and corrupts SBOM bytes to require refusal before any package request. Existing root rotation/restart/expiry/signer/rollback cases remain in the same fixture.

Final Linux update compilation, portable update race suite and diff validation passed. HomeNode checks 36918945508 at bc3232c completed successfully, including actual streamed package/cancellation fixtures, native privileged checks, packaging and browser workflows. The new policy/evidence/integrated fixture still needs its next Linux execution. Fixture bytes are not an installable Debian package and evidence documents are not a production SBOM/provenance review; installation and release qualification remain unfinished.

### Signed release acquisition Linux qualification

HomeNode checks 36919941660 at 06d6715 completed successfully. Its actual Linux Go suite executed the signed repository/root rotation/restart/clock fixtures, compatibility policy, verified evidence and package acquisition integration, corruption/cancellation boundaries and read-only package admission. Privileged ownership/activation/transport fixtures, go vet, command builds, Debian package/install/service validation and browser workflows also passed.

This establishes the tested acquisition behavior, not installation or release qualification. The existing static installer journal records content hashes; a rotating TUF cache root cannot be enrolled as immutable configuration without causing legitimate rotation to conflict with ownership checks. Upcoming integration must keep an immutable independently trusted bootstrap/configuration record separate from the exclusively owned mutable metadata/cache lifecycle. Semantic SBOM/provenance review, release promotion, install/migration/rollback journals, trusted launch config and full product/hardware acceptance remain unfinished.

### Optional installer TUF trust provisioning

ConfigurationPlan and install-prepare now accept an independently pinned TUF bootstrap root. Validation bounds the document to 128 KiB, matches its exact SHA256, verifies upstream TUF root signatures, requires at least a two-key root threshold and separates keys across root/timestamp/snapshot/targets roles. Paired local-root/pin flags refuse partial inputs. The existing ownership journal provisions immutable root-owned 0400 bootstrap data and exactly root-owned 0700 update/metadata/download directories. It deliberately excludes the rotating current cache root from static content records; preview reports bootstrap identity and pending initialization/activation.

Installer tests cover valid threshold trust together with backup provisioning, private-directory invariants, absent immutable current-root enrollment, mismatched pins, single-key roots and invalid documents. CLI tests cover paired-input refusal and a valid independently pinned threshold root. Final command/installer/update race suites, Linux command/installer compilation and diff validation passed. Documentation-only bb4af5a CI 36920285700 completed successfully. Current-cache initialization, trust-policy persistence, semantic release review, installer activation and full hardware/product acceptance remain unfinished.

### Resumable current-root initialization

InitializeCacheRoot now owns the separate mutable cache initialization lifecycle under the exclusive update lock. It validates independently pinned threshold bootstrap data, durably records bootstrap identity before creating root.json, and records completion only after root/file/directory synchronization. A restart resumes only a matching recorded intent and exact bootstrap root bytes. Completed initialization retains the current signed root/version instead of rewriting it from bootstrap. Missing current root, changed bootstrap pin, inconsistent intent, preexisting unowned root, unrelated metadata, clock evidence and canceled acquisition are refused without automatic trust reset.

Update race fixtures simulate interruption after intent and root writes, reopen state, resume and verify repeated completion. Refusal fixtures cover unowned/lost/conflicting state. Final update race suite, Linux update compilation and diff validation passed. Installer bootstrap CI 36920970755 at 95fdcf8 completed successfully, including command/installer/update tests, privileged fixtures, packaging and browser workflows. Initializer execution through the installer/CLI, trust-policy persistence, semantic release review, approved install/migration/rollback and complete physical/product acceptance remain unfinished.

### Installer-owned update initialization command

Engine.InitializeUpdateCache now verifies a completed ownership journal, exact root-owned private update directories, immutable bootstrap permissions/hash and all current owned configuration before invoking the resumable cache initializer. The local Linux/root-only `homenode update-trust-initialize` command accepts only an existing canonical journal directory; it cannot take replacement root bytes/pins or activate updates. Incomplete/missing/duplicate/public/foreign/adopted bootstrap records are refused.

Portable command/installer/update race suites and Linux command/installer compilation passed. Added an opt-in disposable Linux root fixture to the existing privileged installer CI step: apply owned bootstrap configuration, initialize, reopen, verify retained trust, reject changed static bootstrap, and refuse recreation of a lost completed cache root. Actual root fixture execution awaits the next CI run. Initializer CI 36921522507 at 7541679 completed successfully, including existing ownership/transport/package/browser checks. Trust-policy persistence, semantic evidence review, approved install/migration/rollback, backup owner workflows and full physical/product acceptance remain unfinished.

### Installer-owned inspection staging

The installer now provisions a private inspection directory and exposes a Linux/root-only staging bridge that checks the completed ownership journal and configured release floors before publishing an acquired package. Staging holds an exclusive directory lock, records durable operation identity, verifies the read-only package against its signed hash and length, and publishes without replacing existing state. Interrupted or occupied staging requires explicit recovery. This grants no installation authority and activates no service.

HomeNode CI 36931102154 at 6eb8b81 passed the operation-correlated inspection worker checks. Portable installer/update race tests and Linux installer compilation passed for the staging bridge. The expanded disposable Linux root fixture checks release-floor refusal without staging mutations, exact copied package bytes and refusal to overwrite an occupied operation; its execution and the Linux staging tests await the next CI run. Protected worker launch/completion, recovery, semantic release qualification, approved installation/migration/rollback and the full product/hardware acceptance scope remain unfinished.

### Protected inspection stage admission

OpenInspectionStage now retains an exclusive directory lock and a read-only package descriptor for the protected execution caller. It requires exactly the three published staging entries, private owned regular single-link files, byte-exact canonical intent/ready records matching independently retained operation identity, and a fresh package hash/length check. No identity is inferred from disk records and closing admission preserves state for explicit recovery. Linux fixtures exercise concurrent admission refusal, different operation identity and tampered ready records. Linux compilation and portable update race tests passed; actual Linux execution of this addition is pending. Worker activation/result persistence and installation authority are not implemented by this primitive.

### Inspection admission refusal coverage

Added Linux execution fixtures for missing ready state, unexpected entries, symlink/hardlink/FIFO records, public record or directory permissions, writable or changed package bytes, oversized ready records and canceled admission. Each refusal must release the directory lock and retain durable evidence instead of deleting or repairing it. The FIFO case exercises nonblocking file opening before regular-file validation. Linux compilation, portable update race tests and diff checks passed; native execution awaits publication after CI 36932221459 finishes.

### Installer-owned inspection execution admission

Engine.OpenUpdateInspection now joins the completed installer journal and current owned repository configuration to protected stage admission. It requires the independently retained acquired release, platform and sequence/catalog floors and the caller operation; only the journal-recorded private staging path can be opened. The returned descriptor retains the execution lock while the temporary root handle is closed. Expanded disposable Linux root assertions cover correct admission, wrong operation, below-floor release and changed owned repository configuration. Portable installer/update race tests, Linux installer compilation and diff validation passed; native execution remains pending. CI 36932221459 is still running and is not restarted or canceled.

### Post-inspection package/result binding

InspectionStage retains the independently supplied operation identity and exposes VerifyResult to strictly parse bounded worker output and rehash its pinned package descriptor after authenticated worker completion. A result cannot approve a package changed since admission; successful JSON still is not proof of isolated execution or installation authorization. Linux tests cover matching and malformed output and changed package bytes. Linux compilation and portable update race checks passed. CI 36932221459 at 88aaf8b completed successfully, including actual staging and installer root execution, package/source-service checks and browser workflows. Stage admission and result binding additions await their next Linux CI execution.

### Operation-bound inspection service inputs

InspectionEnvironment renders exactly the release and operation variables consumed by the packaged source service from a validated retained identity. Restricted alphabets reject newline/CR/NUL, quoting, shell syntax, escapes and systemd specifier injection; invalid signed package identities also refuse generation. InspectionStage.Environment binds these inputs to the identity admitted under its retained execution lock. Portable race tests execute injection cases and passed; Linux update compilation and diff validation passed. Protected environment-file publication, service activation and authenticated completion remain unfinished; this renderer neither launches nor grants authority. CI 36932829177 is running for the previously pushed admission/result additions.

### Inspection descriptor lifecycle

InspectionStage now privately owns its pinned package, serializes close/configuration/result operations and refuses reuse after close. DuplicatePackage produces an independently closable read-only close-on-exec descriptor for explicit worker inheritance; closing that duplicate cannot release the stage execution lock. Close is idempotent and handles nil/zero state. Linux fixture assertions verify duplicate hash/length, continued verification after duplicate close, repeated close and refusal of all closed-stage requests. Portable update race tests and Linux compilation passed; native lifecycle assertions await CI publication. CI 36932829177 remains active.

### Metadata cancellation versus clean EOF

CI 36932829177 failed TestMetadataDownloadCancellationDuringBody because cancellation can close a response with clean EOF before the transport reports a context error. The fetcher now checks its bounded operation context after reading the body and refuses canceled bytes. A deterministic transport fixture cancels during a clean EOF read and requires nil data plus context.Canceled. The original TLS cancellation fixture passed 100 race-enabled repetitions locally; the complete portable update race suite, Linux compilation and diff validation passed. The failed CI run is terminal; the correction and pending inspection configuration/lifecycle work will be pushed together for fresh execution.

### Durable inspection launch configuration

InspectionStage.PublishEnvironment now requires the privileged coordinator to supply a private owned update parent whose fixed inspection directory is the admitted inode. Under the retained stage lock it rehashes the package and exclusively writes/synchronizes private inspection.env; existing or interrupted launch state is never replaced. The packaged inactive source service now reads this fixed configuration from the protected update parent. Linux tests cover unrelated parent refusal, cancellation without effects, exact operation inputs, 0600 permissions and existing-state refusal. Linux compilation and diff validation passed; native execution and installer-coordinator/service activation integration remain pending. CI 36933201712 is running for the previously pushed cancellation/configuration/lifecycle changes.

### Installer-owned launch preparation

Engine.PrepareUpdateInspectionLaunch now combines verified installer configuration/release floors, owned stage admission and durable fixed environment publication without releasing the execution lock between steps. It returns the admitted handle for future authenticated service execution/completion. Failure closes all temporary handles and admission while retaining durable state. The disposable Linux installer fixture asserts matching environment bytes, concurrent admission refusal, occupied launch refusal and released admission after failed repeat preparation. Portable installer/update race tests, Linux installer compilation and diff checks passed; native execution is pending. CI 36933201712 remains active. Service activation, authenticated completion, explicit recovery and the complete application acceptance scope remain unfinished.

### Atomic service-input publication

Inspection environment publication now writes and synchronizes an exclusive pending record before a no-replace rename and parent-directory synchronization. The service-visible path cannot expose a partially written environment file. Existing final records refuse before pending creation; interrupted pending records refuse replacement and retain evidence for explicit recovery. Expanded Linux assertions exercise retained interrupted bytes, absent final publication and unchanged occupied state. Portable update/installer race suites, Linux update compilation and diff checks passed; native publication tests await their next CI execution. CI 36933201712 is still active.

### Launch configuration verification

InspectionStage.VerifyEnvironment now checks a private owned parent, fixed staging inode, absent conflicting pending state and bounded single-link non-symlink 0600 launch inputs against exact retained identity, then rehashes the pinned package. Installer launch preparation verifies its publication before returning the retained execution handle. Linux assertions cover valid publication, unrelated parent, changed release configuration and conflicting pending state. Portable update/installer race tests, Linux update/installer compilation and diff validation passed. Native execution and authenticated service activation/completion remain pending; CI 36933201712 is still active.

### Fixed service package path identity

Both environment publication and verification now require the service fixed package path to remain an owned private single-link regular file at the same inode as the retained admitted descriptor. Rehashing a descriptor alone cannot establish which file systemd will open later. The Linux fixture replaces that path with identical bytes at a different inode, requires refusal, then explicitly restores the original inode and requires verification. Portable update/installer race tests, Linux update compilation and diff validation passed. CI 36933201712 at a8f7082 completed successfully, including actual Linux stage admission/lifecycle/result fixtures, privileged installer checks, package/source-service isolation and browser workflows. The later publication/preparation/path-binding additions still need their next native CI execution.

### Inspection service completion contract

ValidateInspectionCompletion now checks bounded exact systemd properties from a future privileged local-manager fetch against a retained nonzero invocation ID and monotonic start boundary. It requires successful normal exit, status zero, inactive/dead completion, canonical timestamps and no stale start, duplicate, missing or unknown properties. Runtime property meaning was checked against https://wiki.freedesktop.org/www/Software/systemd/dbus/; actual manager transport/invocation retention still requires native qualification. Portable race fixtures cover successful completion and stale, failed, active, reordered-time, noncanonical and ambiguous responses; update race tests, Linux compilation and diff validation passed. This parser cannot authenticate browser/worker-supplied properties and does not launch services or grant install authority. CI 36933879587 is running for the preceding publication/preparation changes.

### Bounded local-manager completion query

VerifyInspectionServiceCompletion now runs the fixed /usr/bin/systemctl local system-manager query for homenode-inspect.service with exactly the completion properties, no inherited bus/remote environment, discarded stderr, output capped at 2048 bytes, a five-second context and bounded wait delay. Invalid invocation/start boundary and canceled calls refuse before process creation; successful output passes the strict completion contract. Linux fixtures cover preflight/output bounds and command/environment scope using a harmless printf replacement, not a fake manager success claim. Portable update race checks, Linux compilation and diff validation passed. CI 36933879587 at 6cc09e4 completed successfully, including native publication/path binding and installer preparation; actual manager invocation retention, launch/result transport and full product/hardware acceptance remain unfinished.

### Real system-manager completion qualification fixture

Added an explicitly opted-in disposable Linux/root fixture and CI step that exclusively creates a separate temporary oneshot unit, records the pre-launch monotonic boundary, starts it without blocking, captures the real manager invocation ID and runs the production bounded query/parser through a unit-name-only seam. It requires refusal while running, acceptance after normal exit and refusal of an unrelated invocation, then removes only its fixture unit. This will test actual inactive-unit invocation retention rather than relying on fabricated completion properties. Linux compilation, portable update race tests and diff checks passed. Native execution has not occurred; CI 36963492790 for 4156817 remains active, and this fixture is held locally until that run terminates. Production inspector activation/result capture and full application/hardware acceptance remain unfinished.

### Manager subprocess refusal fixtures

Added Linux subprocess fixtures requiring refusal of success-looking properties from a nonzero-exit process, excessive output, malformed properties and a blocked process canceled by the caller deadline. The command-scope assertion now checks the exact completion property list. These helpers execute the test binary through the production bounded command runner and do not simulate authenticated systemd success. Linux compilation, portable update race suite and diff checks passed; Linux-only subprocess cases have not yet executed locally or in CI. CI 36963492790 remains active, so the native manager and subprocess fixtures remain local pending its terminal result.

### Fixed-size invocation validation

Completion parsing and manager-query preflight now share a fixed 32-byte lowercase hexadecimal nonzero invocation check. Oversized input is rejected before decoding/allocation; duplicate validators cannot drift. Portable fixtures cover canonical IDs, zero/uppercase/nonhex/wrong-length/control input and allocation-free refusal of a megabyte-long value. Update race tests, Linux compilation and diff checks passed. CI 36963492790 remains active; native manager/subprocess qualification and production inspection launch/result transport remain unfinished.

### Failed real invocation qualification

Extended the opt-in native manager fixture to reconfigure only its owned disposable unit for a failing oneshot, require systemd start failure and a distinct canonical invocation, and refuse completion for that fresh failed invocation. Cleanup now clears that fixture failed state before deleting/reloading its unit. This verifies that freshness alone cannot turn a failed service into inspection evidence. Linux compilation and diff validation passed; actual native execution remains pending with the held fixture commits. CI 36963492790 was confirmed active at go vet.

### Fresh inspection invocation capture contract

InspectionInvocationFromManager now extracts an invocation only from the shared bounded exact manager property parser, with a retained pre-launch monotonic boundary. It accepts a fresh running oneshot with no reported exit or delegates fast already-completed services to strict successful-completion verification. Stale starts, failed exits/results, pre-start or unrelated active states and missing boundaries refuse. Portable race fixtures cover running and fast completion plus those refusals; update race tests, Linux compilation and diff validation passed. The privileged query still needs to wire this capture into launch coordination; native fixture execution and production result transport remain pending. CI 36963932736 is running for the real-manager qualification changes.

### Privileged manager invocation capture

CaptureInspectionServiceInvocation now obtains fresh invocation evidence through the same fixed bounded local-manager transport as completion verification. Shared transport retains no inherited bus configuration, output/time bounds and command status refusal. Native qualification now captures running invocation through this production query/parser path via the disposable unit-name-only seam. Linux fixtures add capture preflight and completed-invocation transport coverage. Portable update race tests, Linux compilation and diff checks passed; actual native capture execution remains pending. CI 36963932736 is confirmed active in Linux Go adapters; production inspector activation/result capture and complete application acceptance remain unfinished.

### Retain completed inspection manager evidence

Native CI 36963932736 failed the real-manager completion fixture: inactive-unit garbage collection cleared invocation and process timestamps/status after normal exit. The earlier inactive/dead completion assumption was invalid. The source inspection service and disposable manager fixture now use RemainAfterExit=yes; completion requires active/exited plus successful normal process exit and retained fresh timestamps/identity. The future coordinator must explicitly stop the retained unit after verified result handling. Running activating/start capture still refuses reported exits. The separate source-security fixture overrides only RemainAfterExit=no so its existing systemd-run --wait deactivation protocol can finish; it does not claim manager-evidence retention qualification. Portable update race checks, Linux compilation, Python fixture syntax and diff checks passed. Actual corrected native execution remains pending, and production activation/result/stop coordination remains unfinished.

### Kernel inspection launch boundary

InspectionLaunchBoundary now obtains the current-boot CLOCK_MONOTONIC boundary in systemd microseconds with cancellation, range/overflow and zero checks. Coordinator code must capture it immediately before start and retain it for that boot; wall time or a reboot-restored numeric boundary is not equivalent. Native completion qualification uses this production helper for successful and failed starts instead of its own conversion. Linux fixtures exercise live monotonic progression and canceled refusal. Portable update race tests, Linux compilation and diff validation passed; Linux helper execution and corrected retained-unit qualification await CI. CI 36964303828 remains active.

### Retained source-service qualification and package EOF cancellation

The Python source-service fixture now preserves RemainAfterExit=yes, starts without blocking, polls bounded exact manager properties, verifies a retained canonical fresh invocation and successful exited main process, then explicitly stops its unique unit. No retention override remains; isolation and completion retention are tested together. Syntax/diff checks passed, actual execution pending.

CI 36964303828 failed earlier in TestPackageDownloadCancellationRetainsNoVerifiedAuthority: canceled clean EOF was classified as a target-length policy error. Package streaming now checks operation cancellation immediately after copy, preserving cancellation before length/hash classification and before any descriptor publication. Added a deterministic canceled-EOF transport fixture requiring context.Canceled, no returned descriptor and no verified package publication. Linux compilation and portable update race tests passed; Linux-only acquisition regression execution awaits the next run. Corrected retained-manager qualification still has not executed in CI.

### Installer-owned inactive inspection service

ConfigurationPlan now provisions the embedded reviewed homenode-inspect.service as a root-owned 0644 journal record only when a trusted update repository policy is configured. Its exact source bytes retain isolation and completion evidence settings; no Install section enables it automatically. The bounded installer path allowlist includes this fixed service file. Portable configuration tests require exact embedded bytes/ownership, inactive enablement behavior and absence without repository configuration. Installer/update race suites, focused configuration checks, Linux installer compilation and diff validation passed. Service activation still must authenticate owned unit/runtime configuration and persist operation intent; CI 36964571140 is active for preceding completion/cancellation qualification.

### Launch admission requires reviewed owned service

Installer inspection launch preparation now requires a completed journal record for the fixed inspection service with exact root ownership, 0644 mode and digest matching the current embedded reviewed source, plus current file verification. Failure closes admission before publishing launch inputs. The disposable root fixture provisions the reviewed unit and verifies that changed service bytes prevent publication, then explicitly restores its fixture for valid preparation. Portable installer/update race tests, Linux installer compilation and diff checks passed. Native assertions await publication after CI 36964571140 finishes. Effective manager properties/drop-ins, activation, authenticated result transport/stop and complete product acceptance remain unfinished.

### Refuse unreviewed inspection drop-ins

Installer launch preparation now refuses service-wide, homenode-prefix and exact inspection-service drop-in entries in the usual administrator/runtime/local-vendor/vendor/legacy unit paths. Missing entries alone pass; existing entries or inspection errors are not silently ignored. The disposable root fixture checks all three scopes and requires no environment publication on refusal. This filesystem check supplements the owned embedded unit and does not establish effective manager settings or cover every generated/transient/alias path; activation still requires effective-property/DropInPaths qualification. Portable installer/update race tests, Linux installer compilation and diff validation passed. Native assertions are pending; CI 36964571140 remains active.

### Manager load-identity prerequisite

ValidateInspectionUnitIdentity and its privileged fixed bounded manager query now require the exact inspection Id, loaded owned /etc fragment, no manager drop-ins, no pending daemon reload, nontransient oneshot, DynamicUser and retained-exit setting. Portable fixtures refuse substituted/masked/transient/stale/overridden and ambiguous snapshots. Completion and load queries share transport bounds and fixed local service scope. Update race tests, Linux compilation and diff checks passed. This is a load-identity prerequisite only: effective security/resource properties, actual loaded-unit qualification and activation/intent/result/stop coordination remain unfinished. CI 36964571140 is active.

### Effective inspection resource prerequisites

Added bounded exact configured-manager resource validation and a fixed privileged query requiring MemoryMax 256 MiB, zero swap, CPU quota 50%, 32 tasks, OOM kill, control-group termination and no restart. Portable fixtures reject relaxed/missing/duplicate/unknown limits. The retained source-service fixture now queries these values while its worker continues to probe actual cgroup enforcement; configured values alone are not proof of kernel enforcement. Update/installer race tests, Linux update compilation, Python syntax and diff checks passed. CI 36964571140 at 7901d72 completed successfully, including the corrected real retained-unit success/failure fixture, cancellation regressions, isolation with retained completion and browser workflows. New owned-service/drop-in/load/resource and launch-boundary additions await the next CI execution. Activation/intent/result/stop coordination and full product/hardware acceptance remain unfinished.

### Core effective inspection confinement

Added a fixed privileged bounded manager query and strict core confinement validator for no-new-privileges, empty capability sets, strict filesystem/home protection, private temporary/devices/network, kernel/control-group protection, invisible process access/pid subset and SUID/realtime/personality restrictions. Portable fixtures require every field and refuse modified/missing/duplicate/unknown settings. The source-service fixture now observes these values alongside resource settings and worker runtime probes. Update race tests, Linux compilation, Python syntax and diff validation passed; actual property formatting/effectiveness awaits native execution. Network/address-family/syscall/path/namespace policies and complete activation authorization still require additional qualification. CI 36965142904 remains active.

### Effective deadlines and private worker identity

The configured-resource prerequisite now includes the source 150-second start and five-second stop deadlines; core confinement also requires UMask 0077 and an empty supplementary group list. Fixed bounded queries and retained source-fixture observations include these properties. Portable refusal fixtures cover relaxed deadlines and missing/modified mask/group fields. Update race tests, Linux compilation, Python syntax and diff validation passed. Exact native manager formatting and enforcement are still pending qualification. CI 36965142904 remains active; this closes additional prerequisite coverage without claiming complete activation authorization.

### Configured namespace/socket/ABI restrictions

Added fixed bounded manager querying and strict namespace/socket-family/syscall-architecture prerequisites: namespace restriction enabled, AF_UNIX only and native architecture only. Portable fixtures refuse broader namespace/socket/ABI settings and ambiguous/missing snapshots. The retained source-service fixture now observes actual manager values alongside worker runtime probes. Update race tests, Linux compilation, Python syntax and diff validation passed; serialization and native effectiveness remain pending. Syscall denysets, IP/path policies and complete live enforcement/activation authorization are still unfinished. CI 36965142904 remains active.

### Effective IP and protected host-path policy

Added fixed bounded manager querying and strict access-policy validation requiring both IPv4/IPv6 deny-all prefixes, no IP allow exceptions and the exact protected configuration/state/update/backup/runtime paths. Prefixes must be canonical masked zero-bit networks and duplicate/missing/unknown entries refuse; source ignore-missing path markers do not weaken requirements. Portable race fixtures cover allow exceptions, missing or partial/noncanonical deny coverage and missing/duplicate/unclean protected paths. Update race tests, Linux compilation and diff checks passed. Native serialization and live enforcement remain pending; syscall denysets and complete activation/intent/result/stop coordination remain unfinished. CI 36965142904 was confirmed running package qualification.

### Native access-policy observation

The retained source-service fixture now queries IPAddressDeny/Allow and InaccessiblePaths and requires canonical complete IPv4/IPv6 denial without allow exceptions and exactly the six protected host paths. This extends actual property observation alongside its existing private-network, hidden host-file, capability/identity and live cgroup probes. Python syntax, diff checks and portable update race tests passed; expanded native property qualification is pending. CI 36965142904 at b108194 completed successfully, including actual configured-resource observation and prior worker/manager/installer/browser fixtures. Unrelated uncommitted edits in internal/install/update_configuration_test.go were observed and left untouched. The next push includes only committed work; complete activation/syscall/intent/result/stop and full product acceptance remain unfinished.

### Expanded syscall denyset comparison

Checked systemd v255 primary source: https://raw.githubusercontent.com/systemd/systemd/v255/src/core/dbus-execute.c and https://raw.githubusercontent.com/systemd/systemd/v255/src/core/execute.c expose an expanded native syscall list, not the original @group names. Added a bounded exact denyset comparator requiring an independently qualified complete ABI profile; absent, duplicate or malformed profiles refuse. It rejects allowlist mode, missing/extra/duplicate calls, group aliases, changed errno/action suffixes and ambiguous records. Portable race fixtures use a synthetic three-call profile solely to test comparison semantics; they do not qualify the production five-group policy. Update race tests and diff checks passed. The supported Ubuntu/native-ABI full profile, manager query integration and actual seccomp enforcement qualification remain unfinished; no profile is inferred from observed unit output. CI 36965636368 remains active.

### Worker-side installed seccomp probe

The source-isolated worker fixture now requires PR_GET_SECCOMP filter mode and a positive unambiguous kernel Seccomp_filters count from its own process, alongside existing identity/capability/network/temp/cgroup probes. This confirms presence of installed filtering, not completeness of the source denyset or a production qualified ABI profile. Linux inspector compilation and diff checks passed; actual worker-side probe execution awaits publication. CI 36965636368 is still active, confirmed through native volume and package qualification; unrelated installer test edits remain untouched.

### Fixed-unit expanded syscall manager query

CI 36965636368 at 3d01c10 completed successfully, including the source inspector's effective IP/host-path and prior resource/process/isolation observations. Added a root-only fixed local manager query for SystemCallFilter with a bounded independent-profile preflight before process creation. Native fixtures require exact executable, arguments and unit scope, refuse malformed/duplicate/missing profiles before querying, and reject an unexpected expanded denyset. Portable update tests and Linux compilation passed; execution of the new Linux fixtures and installed seccomp probe awaits the next CI run. A complete independently qualified supported ABI profile remains necessary; these helpers do not authorize activation or replace durable intent, authenticated result collection, explicit stop, or full product acceptance.

### Combined effective-confinement prerequisite

Added VerifyInspectionConfinement to require the independent syscall profile plus every implemented local-manager unit identity, resource, isolation, process and access-policy check. It fails on the first refused or canceled observation and remains root-only. Linux compilation and diff checks passed. These sequential observations are explicitly not an atomic snapshot, activation authority, or live enforcement proof: owned package/unit admission, durable launch intent, qualified ABI policy, authenticated result collection and stop/recovery remain required. CI 36966385683 is currently in progress; no outcome is assumed.

### Bounded aggregate preflight refusal coverage

The combined confinement query now has one 30-second overall deadline in addition to each query's five-second limit. Added Linux process-factory fixtures exercising all six manager observations, rejecting each independently, stopping immediately on refusal, preserving fixed local syscall/unit query scope, refusing missing independent profiles before effects, and preventing canceled requests from reaching a process factory. Linux compilation and diff checks passed; fixture execution awaits Linux CI publication. Synthetic profiles test orchestration only and do not qualify the production ABI policy.

### Dormant inspection-unit admission

Added a bounded strict dormant-state validator and root-only fixed local-manager query requiring loaded/inactive/dead. Retained active/exited successful invocations, running/failed/missing units, duplicate/unknown/missing fields and malformed records refuse before a new launch can be considered. Extended the opted-in disposable native manager fixture to require fresh and explicitly stopped units to pass and retained completed/failed invocations to refuse. Portable update tests, Linux compilation and diff checks passed; native execution of these new assertions awaits publication. This observation does not reserve the unit or exclude concurrent manager jobs and cannot replace durable protected coordinator admission. CI 36966385683 remains live; its Go/Linux adapters, installer ownership and prior native completion fixture have passed, with package/service/browser stages still pending.

### Kernel boot binding for retained launch boundaries

Added launch-epoch capture from the fixed kernel boot_id and CLOCK_MONOTONIC sources, with bounded canonical nonzero boot identity parsing, cancellation handling and validation that a retained boundary belongs to the current boot and does not lie in its future. This closes a prerequisite for interpreting monotonic launch evidence during recovery; it does not yet persist launch intent or authorize activation. Linux fixtures cover live capture/verification, a different boot, a future boundary, malformed/oversized/noncanonical boot IDs and canceled capture/verification. Linux compilation and diff checks passed; native execution awaits the next published CI run. The active preceding run is not treated as terminal.

### Durable boot-bound inspection launch intent

Added root-only PublishLaunchIntent on the locked inspection stage. It revalidates the installer-owned parent, exact published environment, pinned service package inode/hash and current boot-bound monotonic epoch under the same stage mutex before publishing an exclusive synchronized 0600 pending record and no-replace final inspection.launch record. The record contains schema, independently admitted operation/release/hash/length and kernel epoch; no worker/browser claims are copied. Existing final or interrupted pending records are retained and require explicit recovery. Linux fixtures verify canceled/unrelated/future admissions have no final publication, interrupted evidence is not overwritten, all seven persisted fields match protected inputs, private permissions and repeated-publication evidence retention. Portable update tests, Linux compilation and diff checks passed; native execution awaits publication. No start, installation authorization, authenticated result collection or recovery workflow is claimed. CI 36966796592 remains the live published run at 84c7cdb.

### Protected launch-intent verification

Added root-only VerifyLaunchIntent, comparing the bounded private single-link canonical record with independently retained package identity and kernel epoch. It rechecks current boot/boundary and environment/fixed package inode/hash under the stage mutex and refuses conflicting pending records. Canonical bytes avoid accepting duplicate, unknown, reordered, extra or differently encoded persisted claims. Native fixtures cover valid record, different retained boundary, cancellation, changed/oversized/noncanonical records, public permissions, conflicting pending evidence and explicitly restored exact record. Portable update tests, Linux compilation and diff checks passed; native execution awaits publication. Recovery of an unknown epoch, manager start/result authentication and installation authority remain unfinished. CI 36966796592 remains the previously confirmed live run.

### Launch record filesystem and owned-installer qualification

Extended native launch-intent fixtures to reject missing, symlink, hard-linked, FIFO and directory substitutions while retaining the original record and proving explicit restoration works. O_NONBLOCK/NOFOLLOW and regular-file/single-link/ownership checks remain enforced by the shared protected opener. The opted-in disposable root installer fixture now exercises the public launch-intent publication and verification methods on its installer-owned parent after package/service/environment admission, and proves a closed stage cannot verify launch state. Update and installer Linux compilation and diff checks passed; native execution awaits publication. Unrelated installer configuration-test edits remain untouched. The preceding CI run is still observed live, so publication is deferred to avoid canceling it.

### Bounded inspection identity preflight

Corrected shared inspection identity validation to reject field lengths before hexadecimal decoding or regular-expression scans. Oversized hash input previously allocated before refusal; bounded operation/release/hash/package lengths now refuse immediately across environment, persisted intent and worker result paths. Portable fixtures assert zero allocations for preallocated one-megabyte operation/release/hash inputs, accept supported maximum identity/package boundaries, and reject below/above supported limits. Targeted allocation tests, full portable update tests and diff checks passed. CI 36966796592 is still in progress; no terminal outcome or native execution of unpublished changes is assumed.

### Native preflight and boot-epoch qualification result

CI 36966796592 at 84c7cdb completed successfully. This executes the published combined confinement refusal fixtures, dormant-state assertions against the real disposable system manager, live kernel boot-epoch checks, installed seccomp/source-service probes and existing installer/package/browser workflows. It does not yet execute the later durable launch-intent publication/verification/filesystem fixtures or identity allocation fix. Those accumulated committed changes are now being published for their own native qualification; launch execution, authenticated result transport, recovery and full end-to-end acceptance remain unfinished.

### Independent syscall source candidate and ABI observation tooling

Extracted the complete recursive union of the reviewed five syscall groups from systemd v255 src/shared/seccomp-util.c, retaining the source URL and SHA256 b97066aee652ea9ff26c9f4c331de0f149686911d11f813fc553e87bbec6bfa8. The checked-in candidate contains 63 names across eight transitive groups and is explicitly not qualified. Added Linux-amd64-only libseccomp resolution tooling that emits a separate unqualified observation with canonical native names, numeric IDs and unknown names, without querying the manager or learning its baseline. Primary parser source ignores only __NR_SCMP_ERROR (-1), so negative pseudo-syscalls are preserved rather than incorrectly excluded. Tool syntax and diff checks passed; execution, distribution version/backport qualification and complete effective-manager comparison remain pending. This source candidate is not wired into production activation. Sources: https://raw.githubusercontent.com/systemd/systemd/v255/src/shared/seccomp-util.c and https://raw.githubusercontent.com/systemd/systemd/v255/src/core/load-fragment.c. CI 36967312726 remains in progress.

### Complete syscall denyset observation in source service fixture

The opted-in disposable source-service fixture now resolves the independent source candidate through native libseccomp before starting its reviewed transient fixture. A separate bounded fixed manager property query requires denylist mode and exactly the independently resolved full set, refusing duplicate/extra/missing calls and ambiguous records. It does not derive a baseline from the service output. The resolver additionally records libseccomp version; a successful source-service fixture retains the full independent observation in CI logs. Python syntax and diff checks passed; native resolver execution and manager comparison are pending publication. This is qualification evidence only: distro version/backport promotion and protected production ABI policy still remain necessary before activation authority. CI 36967312726 remains in progress.

### Exact syscall candidate and distribution package evidence

The ABI resolver now reads at most 16KiB plus one refusal byte and pins the exact reviewed candidate artifact SHA256 8fd83013bc7302b5b2043ddc44daf6acfcb5882848b6e51aed6bc7a16098b61b before decoding; a claimed upstream source hash alone no longer admits changed candidate names. It records bounded fixed dpkg-query identities for installed amd64 systemd and libseccomp2 packages alongside native ABI and library version, preserving distro backport provenance for subsequent qualification decisions. Native FFI calls now declare their argument/return types explicitly. Python syntax and diff checks passed; Linux execution and complete manager equality still await publication. Package observations do not automatically promote an ABI policy or release. CI 36967312726 remains live.

### Candidate refusal before native effects

Added portable process/library-spy fixtures proving altered syscall names with unchanged source claims, semantically equivalent re-encoding, oversized candidate input and duplicate-field injection all refuse before any dpkg-query subprocess or native FFI loading. All four tests passed locally, and the normal Linux workflow now executes them before privileged fixtures. These tests cover source candidate admission, not native resolution or manager equality; the real source-service qualification remains necessary. Diff checks passed. CI 36967312726 remains the published run and has not been assumed terminal.

CI 36967312726 at df13e8e has now completed successfully, executing durable launch-intent publication/verification/filesystem refusals, the public owned-root installer integration and inspection identity allocation bounds alongside existing native/package/browser qualification. The independent syscall resolver and complete denyset comparison are newer and remain unexecuted until the next publication.

### Boot-bound manager execution evidence

Added root-only capture and completion APIs retaining the fresh manager invocation together with its independently captured launch epoch. They verify current boot/boundary before and after the bounded fixed-unit manager query and expose no successful execution object on refusal. Linux process fixtures cover successful typed capture/completion, missing/malformed/other-boot/future epochs, absent invocation and cancellation before manager effects. The opted-in real manager fixture now captures its epoch before start and exercises boot-bound invocation capture and completed execution verification. Linux compilation and diff checks passed; native execution awaits publication. This evidence does not authenticate worker result transport, persist invocation state, recover operations or authorize installation. CI 36967775573 remains in progress.

### Running manager identity in syscall qualification evidence

The disposable source-service qualification now records the local manager's own Version and Architecture via a fixed bounded systemctl manager query, rather than assuming the installed systemd package version identifies the currently running process. Strict unique field parsing requires both properties and the supported x86-64 manager architecture. The retained syscall observation includes this running identity together with installed package versions/native library version and independently derived candidate provenance. Checked the upstream v255 manager Version/Architecture property implementation in https://raw.githubusercontent.com/systemd/systemd/v255/src/core/dbus-manager.c. Python syntax, all four portable candidate-refusal tests and diff checks passed; actual serialization and native comparison await publication. These observations remain explicitly unqualified and grant no production activation authority. CI 36967775573 remains live.

### Durable manager-captured execution publication

Added root-only CaptureAndPublishExecution on the admitted stage. Under its retained mutex it verifies exact protected launch/environment/package/epoch evidence, refuses existing or interrupted execution records before querying, captures invocation directly through the fixed manager path, and synchronizes an exclusive private pending record followed by no-replace final publication. Persisted execution binds schema, canonical launch identity and the manager-captured invocation; failed publication exposes no successful execution object, and evidence is retained for explicit recovery. Linux fixtures exercise interrupted refusal before manager effects, exact persisted fields/private permissions, successful publication and no query/overwrite on retry. Linux compilation and diff checks passed; native execution awaits publication. This method does not request start, authenticate worker output, recover operations or authorize installation. CI 36967775573 remains in progress.

### Recorded invocation required before completion evidence

Added root-only VerifyRecordedExecutionCompletion on the locked admitted stage. It requires exact canonical private execution publication matching the independently retained invocation, launch/package identity and current kernel epoch, refuses conflicting pending evidence and only then queries fixed-unit completion. Native fixtures cover valid persisted completion, another invocation, changed/oversized/noncanonical records, conflicting pending state and explicit restoration, proving rejected evidence cannot reach the manager process factory. Linux compilation and diff checks passed; these new assertions await publication. CI 36967775573 at f2a3b76 completed successfully, including the independently resolved complete syscall denyset equality and candidate refusal tests. Later running-manager identity and recorded execution changes remain unexecuted natively. Full launch/result/recovery and product acceptance remain unfinished.

### Execution evidence filesystem refusal before manager queries

Extended the native recorded-completion fixture to reject public execution-record permissions and missing/symlink/hard-link/FIFO/directory substitutions before the manager process factory can be called. The original private regular inode is retained through the hostile substitutions, and explicit restoration re-enables exact recorded completion verification. These assertions exercise the shared NOFOLLOW/NONBLOCK/owned-single-link opener in the actual execution-record workflow, rather than assuming launch-record tests cover it. Linux compilation and diff checks passed; native execution awaits publication. CI 36968304349 at b63b48c remains in progress. Full activation/result transport/recovery and product acceptance remain unfinished.

### Persisted evidence cannot override manager failure

Extended the recorded-completion workflow fixture with a failed manager invocation/status, another current invocation, manager process failure and canceled verification before manager effects. Correct private persisted bytes are insufficient in each case, and refusal must preserve the exact execution record. These checks exercise the complete stage/record/epoch/manager path rather than relying solely on leaf completion parser tests. Linux compilation and diff checks passed; native execution awaits publication. CI 36968304349 at b63b48c remains in progress. Authenticated result collection, start/stop recovery and complete product acceptance remain unfinished.

### Strict result journal envelope prerequisite

Added a bounded single-entry journal envelope validator requiring the exact retained kernel boot hex ID, manager invocation, fixed inspection unit and stdout transport before validating bounded result JSON against admitted package identity. Duplicate/unknown/non-string/null fields, extra records, malformed payloads and abnormal _LINE_BREAK values refuse. Trusted journal metadata claims only have meaning when fetched by the protected coordinator itself; this parser does not authenticate browser-supplied JSON. Portable fixtures cover valid context and wrong boot/invocation/unit/transport, truncation and ambiguous records. Primary field semantics reviewed in https://raw.githubusercontent.com/systemd/systemd/v255/man/systemd.journal-fields.xml. Local journal acquisition, native serialization qualification and full result/start/stop recovery integration remain unfinished.

### Fixed local journal result query

Added root-only ReadInspectionJournalResult with independent identity/invocation preflight, kernel epoch verification before/after acquisition, fixed /usr/bin/journalctl system scope and exact unit/invocation/boot matches. It uses a clean environment, five-second deadline, one-second WaitDelay and 8KiB streaming output bound. At most two matching records are requested so the single-entry parser rejects multiplicity rather than selecting one ambiguous message. Fixed output-fields retain journalctl's mandatory cursor/time/boot metadata, per upstream v255 journalctl.xml; null/array fields fail strict string validation. Linux fixtures cover exact command/environment, valid context, cancellation/invalid invocation before effects and streaming bounds. Linux compilation and diff checks passed. Actual native journal serialization, authenticated stage integration and result retention remain unfinished; this helper must follow recorded completion under operation admission and grants no install authority.

### Native system-journal result qualification fixture

Added an explicitly opted-in disposable root Linux fixture installing only an exclusive temporary fixed inspection unit when no installed unit exists. A DynamicUser cat process receives root-private synthetic result bytes through manager-opened stdin and emits them through the real stdout journal transport. The fixture captures/verifies the production boot-bound manager execution, synchronizes the local journal, and invokes the production root journal query, requiring valid context and rejecting a wrong package identity. A fresh invocation emits duplicate messages and must refuse. Cleanup stops/removes only its owned temporary unit and reloads the manager. The Linux workflow now runs this fixture separately; Linux compilation and diff checks passed, while actual native serialization/stream identity execution awaits publication. This qualifies transport semantics with synthetic content; actual isolated inspector output and full stage/result retention integration remain required. CI 36968802162 at 0b4a90e remains in progress.

### Native journal refusal evidence strength

Strengthened native transport qualification to explicitly observe the expected one or two persisted journal entries, each with the exact emitted payload, before invoking result acceptance/refusal. Bounded per-query output/deadlines and a three-second convergence window account for journal delivery timing. Duplicate-message refusal can no longer pass merely because journal acquisition returned no records. This validates the negative fixture's precondition; production still refuses missing/ambiguous output. Linux compilation and diff checks passed; native execution awaits publication. CI 36968802162 at 0b4a90e remains in progress.

### Admitted-stage journal result collection

Added root-only CollectInspectionResult retaining the stage mutex throughout protected execution-record/package/environment/epoch validation, manager completion, fixed local-journal acquisition, and repeated protected-record/package/manager verification after reading. It uses a 30-second overall deadline and returns no result on any failure; successful content validity grants no installation authority. Linux fixtures cover the full synthetic valid chain, invalid journal payload and a failed post-read manager observation despite valid journal content. Linux compilation and diff checks passed; actual native journal qualification and isolated worker/stage integration remain pending. Installer/service/effective-policy admission remains the coordinator's prerequisite, and result retention plus start/stop recovery are unfinished. CI 36968802162 remains in progress.

### Durable locally collected inspection result retention

Added root-only CollectAndPublishInspectionResult, retaining the admitted stage lock while collecting through protected recorded manager completion and local journal context, then synchronizing exclusive private pending/no-replace final result evidence. The record binds schema, canonical admitted execution and the internally collected typed result; no caller-supplied result is accepted and installAuthorized remains false. Existing/interrupted records refuse before manager/journal effects, partial evidence is retained and any publication/close/cancellation failure returns no successful result. Linux fixtures cover interrupted refusal without effects, exact retained execution/result fields, private permissions and no overwrite/recollection on retry. Linux compilation and diff checks passed; native execution awaits publication. Recovery/readback/stop authorization and full product acceptance remain unfinished. CI 36969245179 at fe9892a remains in progress.

### Native journal envelope refusal investigation

CI 36969245179 at fe9892a failed in the new real system-journal fixture after its independent message-count/payload check succeeded: the strict production envelope parser refused the native entry. This is a genuine unresolved serialization/context mismatch, not proof that journal transport is qualified. Added bounded synthetic fixture envelope diagnostics on failure so the next native run can identify the exact field/type/value; no acceptance rule was weakened by guesswork. Linux compilation and diff checks passed. Earlier portable/recorded result assertions do not establish native transport completion, and launch/result/recovery/full product acceptance remain unfinished.

### Protected retained result readback

Added root-only ReadRecordedInspectionResult comparing bounded private canonical evidence with independently retained execution and locked package/launch/environment/current-epoch identity. It does not infer invocation identity from disk or grant installation authority. Result publication and readback share one canonical encoder requiring exact admitted successful content with installAuthorized=false. Linux fixtures cover exact readback, cancellation, changed/oversized/noncanonical data, conflicting pending evidence and explicit restoration. Linux compilation and diff checks passed; native execution awaits publication. The prior native journal envelope mismatch remains unresolved pending diagnostic CI 36969628326 at afe978a; this independent readback work is not proof of transport qualification. Full result/start/stop recovery and product acceptance remain unfinished.

### Native serializer sequence metadata and retained result filesystem checks

Primary systemd v255 src/shared/logs-show.c shows output_json always inserts __SEQNUM and __SEQNUM_ID, despite the manual's output-fields list naming only cursor/time/boot headers. Added only these two known metadata fields with paired presence, canonical positive uint64 sequence and nonzero canonical 128-bit sequence ID validation; unknown fields still refuse. Portable fixtures cover valid native headers and malformed/incomplete sequences; full portable update tests passed. This independently establishes a parser protocol gap, while the diagnostic native envelope must still confirm the actual failure and subsequent execution must prove correction. Source: https://raw.githubusercontent.com/systemd/systemd/v255/src/shared/logs-show.c.

Extended Linux retained-result readback fixtures to reject public permissions and missing/symlink/hard-link/FIFO/directory substitutions with no result exposure, then require explicit restoration of the original private regular inode to pass. Linux compilation and diff checks passed. Native execution remains pending; CI 36969628326 is still the diagnostic run at afe978a.

Diagnostic CI 36969628326 has now completed with failure and retained the exact bounded native envelope: it includes __SEQNUM and __SEQNUM_ID, while the expected boot/invocation/unit/stdout context and emitted result payload are present. This confirms the observed serialization mismatch matches the source-established missing header support. The explicit sequence-header correction is now published for native requalification; success is not assumed before that run completes.

### Final-query package mutation refusal

Closed a result-collection verification interval: after the final manager completion query, revalidate protected execution/launch/environment/current epoch and fixed pinned package hash again before returning a result. The existing overall deadline still bounds the combined operation. Added a Linux fixture that changes the owned package during the second manager query while returning otherwise valid completion and journal context; collection must expose no result. Explicit restoration of exact admitted bytes must permit collection again. Linux compilation and diff checks passed; native execution awaits publication. CI 36969952183 at 217a723 remains the journal sequence-header requalification run and is in progress. Complete start/stop recovery and full product acceptance remain unfinished.

### Installer-owned retained result admission

Added root/real-host-only ReadRecordedUpdateInspectionResult on the installer engine. It reopens the package stage against protected signed release/operation and repository floors through the completed ownership journal, requires the reviewed owned service without refused drop-ins, then reads canonical private result evidence against independently retained execution. Any read/parent/stage close failure exposes no result. Opted-in root installer fixtures cover missing result/execution evidence and unrelated operation refusal, and prove failed readback releases admission for explicit recovery. Linux installer compilation and diff checks passed; native execution awaits publication. It grants no install authority and requires the caller's earlier stage closed; full collection/retention/stop recovery integration and product acceptance remain unfinished. CI 36969952183 at 217a723 remains in progress.

### Retained result installer policy refusal evidence

Extended opted-in root installer fixtures to require ownership-policy ErrConflict on changed reviewed service bytes, each refused systemd drop-in, a below-floor release and modified repository policy during retained result readback. Requiring the specific policy conflict prevents these negative tests from passing merely because result evidence is absent. After restored policy, stage admission must still reopen, proving refusal released its lock. Linux installer compilation and diff checks passed; native execution awaits publication. CI 36969952183 at 217a723 remains in progress; complete update start/stop/recovery and full product acceptance remain unfinished.

### Installed inspector output under reviewed service protections

The disposable source-service fixture now runs both the existing confinement test binary and the installed production homenode-inspect executable against the actual built development package, each in its own unique retained invocation with all reviewed service protections and full independently resolved syscall comparison. The production run queries the real local journal and requires one bounded record with exact unit/invocation/boot/stdout context and a strictly typed result matching independently computed package hash/length/release/operation, with installAuthorized=false. The reviewed service now explicitly selects journal stdout/stderr, and the fixture also observes those effective manager properties. Python syntax, portable installer/update tests and diff checks passed; native execution awaits publication. This extends actual worker transport qualification beyond synthetic cat output but does not yet prove the fixed-unit installer/stage/start/stop/recovery chain or full product acceptance. CI 36969952183 at 217a723 remains in progress.

CI 36969952183 at 217a723 has now completed successfully, including the real native journal fixture after explicit sequence-header correction, result retention/readback/filesystem assertions and prior native/package/browser workflows. Later final-query package rebinding, installer retained-result admission and installed-worker journal qualification remain pending in the next publication.

### Effective journal routing admission

The fixed manager process-policy query and aggregate confinement preflight now require StandardOutput=journal and StandardError=journal alongside namespace/socket/native ABI restrictions. This closes the gap between the reviewed service's explicit journal routing and effective manager admission. Portable fixtures refuse inherited/discarded routing, missing fields and duplicate output fields; the Linux aggregate fixture includes both required observations. Portable update tests and Linux amd64 compilation passed; native execution of this change remains pending publication. CI 36970696187 at 6eaacb2 is still in progress, so these changes remain local to avoid canceling that qualification run. Full update activation/recovery and complete product acceptance remain unfinished.

CI 36970696187 at 6eaacb2 completed successfully: installer policy/readback refusals, final-query package mutation refusal, native journal collection and the actual installed inspector's journal output under reviewed service protections all passed, alongside the existing package, native adapter and browser checks. Added explicit argument/path/service assertions for all six aggregate confinement observations so synthetic successful output cannot hide a missing journal-routing query or redirected manager scope. Linux amd64 test compilation passed; execution of the new routing/query assertions remains pending the next CI run. This evidence does not establish complete activation/recovery or full product acceptance.

### Installer-owned result collection and publication

Added root/real-host-only CollectAndPublishUpdateInspectionResult. It reopens the stage against signed release/operation and owned repository floors, admits the reviewed installer-owned service, then invokes recorded-completion/local-journal collection and durable result publication itself. It shares admission and zero-result-on-read/close-failure handling with retained readback. It accepts no caller result document, activates no service and authorizes no installation; prior stage must be closed and execution independently retained. Opted-in root fixtures refuse missing execution, changed service, below-floor release and modified repository policy before collection can proceed. Portable installer tests and Linux amd64 compilation passed; native execution remains pending publication. CI 36971178998 remains in progress; full activation/stop/recovery and complete product acceptance remain unfinished.

Collection refusal fixtures now independently require no final/pending result publication and immediate reopening after missing execution evidence, proving refusal releases the stage lock. They also reject an unrelated operation and each refused service drop-in with the specific ownership conflict rather than merely failing on absent evidence. Linux amd64 compilation and diff checks passed; root fixture execution remains pending disposable CI. These fixtures do not establish a successful fixed-unit collection chain.

### Post-collection installer policy verification

The installer result boundary now rechecks owned repository policy/release floors and reviewed service ownership after successful collection/readback, before returning any result. Manager and journal queries can wait; relying solely on admission before those waits left a policy verification interval. A root fixture substitutes reviewed service bytes during a successful synthetic collection callback and requires the specific ownership conflict plus a zero result. This fixture exercises installer policy ordering, not real worker transport. Portable installer tests and Linux amd64 compilation passed; native fixture execution remains pending publication. CI 36971178998 is still in progress.

Post-collection installer fixtures additionally substitute repository bytes, lower the retained release sequence/catalog below configured floors, and cancel during an otherwise successful synthetic collection callback. Each requires the specific policy/cancellation error, zero evidence, and reopening after explicit restoration to prove stage lock cleanup. Linux amd64 compilation and diff checks passed; these ordering fixtures still require opted-in disposable root execution. CI 36971178998 at 87dbc49 has now completed successfully, including the effective journal routing/query assertions. Subsequent installer collection/publication and post-collection verification are being published for native qualification; full product acceptance is unfinished.

### Native journal retained-stage lifecycle fixture

Extended the opted-in disposable Linux journal fixture to create a real private staged package with independently computed SHA/length, publish environment and boot-bound launch intent before starting its temporary fixed unit, capture/publish execution through the actual manager, collect/publish the actual local-journal result, explicitly stop the fixture unit, then read retained evidence successfully without requiring an active unit. The fixture still uses synthetic cat output and package bytes; it proves transport/staging/retention integration rather than production package inspection or installer-owned fixed-unit activation authority. Existing duplicate-message rejection retains a separate fresh invocation. Linux amd64 compilation and diff checks passed; native execution awaits publication. CI 36971532716 remains in progress, and full product acceptance is unfinished.

The native retained-stage fixture now closes and reopens package admission after explicit unit stop before readback, ensuring success relies on protected disk evidence plus independently retained identity/execution rather than the earlier in-memory stage. A later real invocation must not substitute its execution into the retained result; the original recorded result must remain readable using the original independently retained execution. Linux amd64 compilation and diff checks passed; native execution awaits publication. CI 36971532716 remains in progress. Unrelated local edits in update_configuration_test.go and systemd_fixture.py are preserved and excluded from this change.

The native lifecycle fixture now explicitly observes dormant manager state after stop and requires active completion verification to refuse before retained-stage reopening/readback. This prevents stopped readback success from passing merely because the temporary unit accidentally remained active. It distinguishes recorded historical content evidence from current manager completion authority. Linux amd64 compilation and diff checks passed; native execution awaits publication. CI 36971532716 remains in progress.

### Retained-read verification interval and native recollection refusal

Retained result readback now revalidates protected execution/launch/environment/current epoch and fixed package hash after reading canonical result bytes, before exposing evidence. This supplements its initial admission across the result-read interval. The native lifecycle fixture additionally requires live recollection with the previous execution to refuse after an actual later manager invocation, and recollection with the new execution to refuse because that execution was never recorded for this stage. Original historical retained readback remains permitted. Portable update tests, Linux amd64 compilation and diff checks passed; native execution remains pending publication. CI 36971532716 remains in progress. Production activation/stop/recovery and full product acceptance remain unfinished.

CI 36971532716 at dd5b862 completed successfully, including opted-in root installer result-collection refusal/post-policy rechecks and lock-cleanup fixtures, real manager/journal checks, installed worker source-service qualification, package checks and browser workflows. Later retained-stage native lifecycle integration/reopening/dormant-state/recollection assertions and post-read package rebinding remain pending the next publication. The fixture transport remains synthetic and does not establish complete production activation/recovery or full product acceptance.

### Retained acquisition identity stability

The installer result boundary now snapshots comparable release metadata and package SHA/length before collection and refuses any change before exposing evidence. Floor rechecks alone would not notice platform/release or package identity replacement. Extended root ordering fixtures mutate platform, release, hash and length during the successful synthetic callback; each requires ownership conflict, zero evidence and restored-stage reopening. Portable installer tests, Linux amd64 compilation and diff checks passed; native execution awaits publication. This is sequential identity stability, not synchronization of concurrently mutated caller memory; callers must still retain protected acquisition/operation state. CI 36971939695 remains in progress and full product acceptance remains unfinished.

### Post-capture launch binding

Execution capture now revalidates protected launch/environment/current epoch and fixed package admission after its manager query, before publishing the invocation record. Added a Linux fixture that alters environment bytes during otherwise valid manager capture and requires zero execution plus absence of final/pending records, then permits capture after explicit restoration. This closes a verification interval before durable execution publication. Portable update tests, Linux amd64 compilation and diff checks passed; native execution awaits publication. CI 36971939695 remains in progress; complete production activation/recovery and full product acceptance remain unfinished.

### Post-completion recorded state verification

Recorded completion verification now rechecks protected execution/launch/environment/current epoch and fixed package after the manager completion query. A Linux fixture alters environment bytes while returning valid completion and requires refusal; restored inputs permit subsequent verification. This aligns the standalone completion API with collection's existing post-query rebinding rather than exposing success against stale admission. Portable update tests, Linux amd64 compilation and diff checks passed; native execution awaits publication. CI 36971939695 remains in progress. Full production activation/recovery and product acceptance remain unfinished.

CI 36971939695 at 91adfab is confirmed live. Its installer root fixture and native local-journal step have completed successfully. The journal step executes the retained-stage lifecycle, explicit dormant stop observation, reopening/readback, later invocation refusal and post-read package rebinding introduced in that pushed state. This provides actual Linux manager/journal execution evidence for the synthetic-output transport/staging chain. The overall job remains in progress; subsequent acquisition identity stability and post-capture/post-completion rebinding remain local and unqualified until a later native run. This does not qualify production fixed-unit installer activation/recovery or full product acceptance.

### Worker argument identity bounds

The Linux inspector now validates operation/release lengths before compiled regex matching, and requires the same release-name grammar used by signed metadata instead of merely a nonempty release. This occurs before host identity/capability/descriptor admission. Direct pure-validator fixtures distinguish argument refusal from unrelated root identity refusal and cover malformed/newline, 65-byte and 1MiB inputs plus a valid development release. Linux amd64 test compilation and diff checks passed; Linux execution awaits disposable CI. Full production activation/recovery and product acceptance remain unfinished.

### Worker inherited descriptor access authority

The inspector now checks F_GETFL before reading inherited FD3 and requires O_RDONLY without O_PATH. Root-owned mode-0400 inode checks alone do not constrain an already-open writable descriptor; O_PATH also has read-only access bits while granting no readable contents. Direct Linux fixtures accept an actual readable read-only descriptor and refuse write-only, read/write, path-only and closed descriptors independently of worker UID admission. Linux amd64 compilation and diff checks passed; actual Linux tests/worker service qualification remain pending publication. Full production activation/recovery and product acceptance remain unfinished.

### Combined evidence operation deadlines

Execution capture/publication, recorded-completion verification and retained-result readback now apply a 30-second deadline after acquiring stage admission, matching result collection's overall deadline. Manager queries retain their shorter subprocess deadline; repeated package/record checks consume the same operation deadline. This does not make mutex admission context-aware or guarantee cancellation of kernel filesystem I/O. Portable update tests, Linux amd64 compilation and diff checks passed; execution of the new wrappers awaits publication. CI 36972372888 at 0b6022b completed successfully, qualifying worker argument/descriptor checks, acquisition identity stability and post-capture/post-completion state rebinding alongside existing native/package/browser checks. Unrelated edits to inspection_publish_linux_test.go, update_configuration_test.go and systemd_fixture.py are preserved and excluded. Full product acceptance remains unfinished.

### Exact worker package inode admission

Inherited worker package admission now requires root UID/GID, one link, exact regular-file mode 0400 without special permission bits, and size 1..512MiB before hashing/parsing. Earlier permission-only checks omitted group ownership and allowed setuid/setgid/sticky bits; length was checked later. Direct stat-policy fixtures refuse owner/group/link/special-mode/type/empty/negative/oversized cases and admit the maximum bounded length. Linux amd64 compilation and diff checks passed; actual Linux fixture/production worker qualification remains pending publication. CI 36975073291 remains in progress. Full production activation/recovery and complete product acceptance remain unfinished.

### Worker final descriptor/inode authority verification

The worker now applies the same descriptor/inode authority checks before hashing and after package parsing/final hash, and requires its operation context still live before emitting JSON. The shared check refuses changed permissions, link count, ownership, special mode or descriptor access even when package contents remain unchanged. Extended the opted-in root entry fixture with a real root-owned read-only inode, permission broadening and a new hard link; each mutation must refuse, and explicit restoration must pass. Linux amd64 compilation and diff checks passed; native execution remains pending publication. This supports the complete application's worker boundary but does not establish production activation/recovery or overall completion.

The opted-in root descriptor-authority fixture additionally changes an open package's group ownership away from root, requires refusal, restores ownership and requires acceptance, then unlinks the package and requires the still-open descriptor's zero-link inode to refuse. These use actual kernel inode state rather than only constructed stat values. Linux amd64 compilation and diff checks passed; root execution awaits publication after the still-live CI 36975073291 finishes. Complete maintenance executable activation orchestration remains absent; library evidence admission is not equivalent to the full update workflow.

### Trust initialization CLI success boundary

The root update-trust initialization CLI now explicitly closes its installer engine and combines initialization, close and deadline errors before emitting success. Previously deferred Close errors were discarded and fatal's process exit skipped the defer on initialization failure. Success output therefore follows cleanup, and failure closes admission before exiting. Portable CLI tests and diff checks passed; native initialization and the latest worker inode authority changes await qualification. CI 36975073291 remains in progress. Complete maintenance activation orchestration and overall product acceptance remain unfinished.

CI 36975073291 at ca19a81 completed successfully, qualifying the combined evidence deadlines alongside existing Linux adapter, installer root, real manager/journal retained-stage lifecycle, worker package/source-service and browser workflows. Subsequent exact worker inode admission, post-parsing descriptor/inode rechecks, root group/unlink refusal fixtures and trust initialization cleanup boundary are being published for their own native qualification. The full production update workflow and overall product acceptance remain unfinished.

### Unambiguous acquired evidence JSON

Signed SBOM/provenance target acquisition now requires one bounded JSON object with unique decoded keys at every object depth, bounded nesting and token count, rather than json.Valid alone. This rejects escaped duplicate aliases and nested duplicate fields before semantic consumers could interpret signed bytes differently. Arrays remain supported inside objects; scalar/root-array/multiple-document evidence refuses. Portable fixtures cover valid object structures, duplicate/escaped/nested ambiguity, malformed/trailing data and structural/byte bounds; update tests and diff checks passed. This is deterministic syntax admission only: SBOM completeness, vulnerabilities, build issuer/identity and release promotion remain unimplemented qualification requirements. CI 36975618377 remains in progress.

Evidence acquisition fixtures now serve duplicate, escaped-alias and nested duplicate JSON, scalar evidence and invalid UTF-8 with matching signed target SHA/length. The downloader must still expose no bytes, proving target integrity cannot bypass deterministic syntax admission. Raw evidence now requires valid UTF-8 and counts closing delimiters against the token limit. Portable update tests and diff checks passed. Semantic evidence qualification remains unfinished; CI 36975618377 remains in progress.

### Evidence trailing-document resource refusal

Evidence parsing now probes a single token for EOF after the admitted root object, instead of decoding a complete second value outside the structural budget before rejecting trailing input. A regression fixture appends a 10,000-object second document and requires refusal with bounded allocations, preventing accidental materialization of an otherwise disallowed trailing tree. Portable update tests and diff checks passed. This strengthens resource refusal without claiming SBOM/provenance semantics or release qualification; CI 36975618377 remains in progress.

### Provenance subject and declared builder binding

Added bounded ValidateProvenanceBinding for in-toto Statement/v1 and SLSA provenance/v1 claims. It requires exactly one subject matching independently retained canonical package SHA256, exact independently supplied builder ID/build type and object-valued external parameters; exact JSON key lookup prevents case-insensitive alias substitution. Unknown extension fields remain tolerated per the standard, while duplicate syntax refuses via deterministic evidence parsing. Portable fixtures cover wrong subject/builder/type, casing, null parameters, repeated matching subjects and malformed/oversized expected digest; update tests and diff checks passed. Sources: https://slsa.dev/spec/v1.2/build-provenance and https://raw.githubusercontent.com/in-toto/attestation/main/spec/v1/statement.md. This library primitive does not authenticate signer/issuer, verify external parameters or qualify a builder, is not wired as complete release approval, and grants no installation authority. These remaining gates must be implemented before production promotion. CI 36975618377 remains in progress.

### Provenance reviewed inputs and resolved source binding

Added ValidateProvenanceInputs to extend subject/builder/build-type claim checks with exact independently reviewed external parameter structure and one exact resolved source URI/gitCommit. Parameters reject missing/extra fields and type changes; numeric lexemes remain conservatively exact via UseNumber. Source commits require bounded canonical lowercase 40/64-character hex and duplicate/mismatched source claims refuse. Portable tests cover changed refs/commit, parameter injection/types and ambiguous expected policy; update tests and diff checks passed. This implements the SLSA external-parameter/resolved-dependency binding direction from https://slsa.dev/spec/v1.2/build-provenance but still does not authenticate issuer/signature, qualify a builder, enforce protected production policy or authorize installation. CI 36975618377 at 5dd1a9f completed successfully for earlier worker inode/descriptor checks and trust initialization cleanup; subsequent evidence/provenance primitives await publication.

### DSSE pinned-key threshold authentication

Added VerifyProvenanceEnvelope for exact DSSE authenticated payload bytes using at least two distinct pinned Ed25519 keys. It verifies protocol PAE framing, fixed in-toto media type, bounded envelope/signatures/payload and deterministic payload JSON; keyid remains an unauthenticated hint and cannot count identities. Duplicate pinned key material refuses, repeated signatures cannot meet threshold, and standard/URL-safe padded/unpadded canonical base64 are supported. Tests use an independent literal PAE and real Ed25519 signatures, covering valid distinct threshold, missing/repeated signatures, media-type mutation and aliased trust keys. Portable update tests and diff checks passed. Primary protocol: https://raw.githubusercontent.com/secure-systems-lab/dsse/master/protocol.md. This primitive does not yet load protected production keys, rotate/revoke trust, authenticate certificate issuer identity, or combine provenance policy with release promotion. CI 36976201309 remains in progress; full product acceptance remains unfinished.

### Combined authenticated provenance policy gate

Added ProvenancePolicy and VerifyReleaseProvenance to chain distinct pinned-key DSSE threshold authentication directly into subject/builder/build-type/reviewed parameters/resolved source checks on the exact returned authenticated bytes. The envelope is not reparsed to obtain a potentially different payload. Combined cryptographic fixtures require authentication and semantic binding together, including validly signed wrong package, parameters and source revision refusal. Portable update tests and diff checks passed. Policy still needs protected provisioning/rotation and production integration; this is one library release gate and does not satisfy SBOM/vulnerability/rollback/install or complete application acceptance. CI 36976201309 remains in progress.

DSSE cryptographic fixtures now explicitly refuse payload substitution with retained signatures, untrusted additional signers, single-key policy and corrupted signatures, always exposing no payload. Base64 fixtures independently qualify padded/unpadded standard and URL-safe alphabets, and refuse whitespace, noncanonical pad bits and malformed input. Portable update tests and diff checks passed. Protected production key management and complete release promotion remain unfinished; CI 36976201309 remains in progress.

### Provenance pinned-key policy configuration parser

Added a 16KiB exact-schema provenance policy parser with required fields, schema1, threshold≥2, ≤16 distinct canonical Ed25519 key encodings, bounded builder/build-type/source identifiers, canonical 40/64-character source commit and unambiguous object parameters. Any refusal returns zero policy/key material. Portable fixtures reject duplicate key material, invalid thresholds, casing aliases, null parameters, noncanonical keys, unknown fields and oversized configuration; update tests and diff checks passed. Configuration parsing does not establish protected ownership/trust epoch and is not yet wired to installer-owned policy provisioning or rotation; those integrations and complete product acceptance remain unfinished. CI 36976201309 remains in progress.

### Installer-owned provenance trust readback

Added root/real-host-only ReadUpdateProvenancePolicy. It requires completed installer-owned repository/bootstrap policy, the unique created root0400 provenance record, an exact private single-link regular inode before/after bounded reading and journal-matched SHA before parsing trust keys. Added the fixed provenance configuration path to the installer plan allowlist and extended opted-in root initialization fixtures to admit owned policy and refuse broadened permissions with zero keys. Portable installer tests, Linux amd64 compilation and diff checks passed; root execution awaits publication. Configuration generation/CLI provisioning, policy rotation and production acquisition verification are still required. CI 36976201309 at 6b5f61a completed successfully for earlier evidence/source binding; later DSSE/policy work is being published for qualification.

### Generated installer provenance policy ownership

ConfigurationPlan now accepts explicit reviewed provenance policy bytes, requires accompanying update repository/bootstrap, validates the bounded distinct-key policy before generating any items, and provisions exact copied bytes at etc/homenode/update-provenance.json as root0400. Portable configuration fixtures require exact bytes/private ownership and refuse missing update trust or incomplete policy. Installer tests and diff checks passed. CLI input wiring, production acquisition verification and rotation/revocation remain unfinished; CI 36976891646 remains in progress. No production keys are invented or generated by this change.

### Reviewed provenance policy installer CLI inputs

install-prepare now accepts paired update-provenance/update-provenance-sha256 flags only with configured update repository/bootstrap. It opens a regular no-follow/nonblocking input, caps reads at16KiB, checks the independently supplied canonical SHA256 and parses the distinct-key policy before carrying exact bytes into generated root0400 ownership. Portable CLI fixtures require exact pin/bytes and refuse wrong/noncanonical pins and symlink inputs; CLI tests and diff checks passed. Policy is explicitly supplied, not generated or fetched from an envelope. Rotation, protected acquisition verification and full product acceptance remain unfinished; CI 36976891646 remains in progress.

CI 36976891646 at 1714b70 failed the opted-in root fixture before policy readback: newly added provenance file preceded its explicitly owned parent directory, so the installer correctly refused the invalid plan. Corrected the fixture to place the file after etc/homenode and added a portable plan-ordering regression requiring refusal before the parent and admission after it. Portable installer/CLI tests, Linux amd64 installer compilation and diff checks passed; native correction requires requalification. This was a fixture ordering defect, not successful native policy evidence. Generated configuration and pinned CLI inputs are being published with the correction; full product acceptance remains unfinished.

### Installer-owned acquired provenance verification

Added root/real-host-only VerifyUpdateReleaseProvenance. It requires owned repository floors and protected provenance policy, rehashes the acquired package against retained SHA/length, applies authenticated DSSE plus reviewed build/source policy, then rehashes/rechecks acquisition identity and owned policy before success within a shared30s deadline. Root fixture requires specific provenance refusal when acquired evidence is missing. Portable installer tests, Linux amd64 compilation and diff checks passed; native execution awaits publication. This is an explicit qualification entry point, not installation authority; acquisition auto-enforcement, policy rotation and other release/application gates remain unfinished. CI 36977267198 remains in progress.

### Owned provenance verification root success fixture

Extended opted-in root ownership fixtures with two deterministic test-only Ed25519 signers, matching owned policy, actual acquired package SHA and reviewed source/build inputs. The native fixture now requires full owned provenance verification success, specific ownership refusal after protected policy bytes change, and success only after explicit restoration. Missing evidence refusal remains independently required. Portable installer tests, Linux amd64 compilation and diff checks passed; root execution awaits publication. CI 36977267198 at d374fae completed successfully, including corrected owned-policy readback and generated configuration/pinned CLI checks. Complete acquisition enforcement, trust rotation and product acceptance remain unfinished.

### Acquisition enforces owned provenance

Installer-owned acquisition now checks protected provenance policy before network work and authenticates the acquired release against that policy before exposing it. Verification shares the existing engine lock through a locked helper, avoiding recursive locking while retaining package rehash, release-floor and policy readback checks. Verification failure closes the acquired release and returns no package; existing deferred staging/root cleanup still suppresses results on cleanup errors. Portable installer/update tests, Linux amd64 installer compilation and diff checks passed. Native CI 36995084053 for the preceding signed owned-verifier fixture remains in progress; acquisition integration still requires native qualification. SBOM/vulnerability review, trust rotation, owner approval and installation lifecycle remain required; full application acceptance is unfinished.

### Inspection staging enforces provenance admission

Inspection staging now runs the owned provenance verifier under the engine lock before opening publication staging, so direct callers cannot bypass acquisition qualification. Native root fixtures require missing provenance to refuse with no staging entries and altered policy to refuse acquisition with no download entries. The existing independently signed success fixture then exercises staging admission after qualification. Portable installer/update tests, Linux amd64 compilation and diff checks passed; native execution awaits publication. CI 36995084053 remains live, so local commits are retained without superseding that qualification run. This does not implement remaining SBOM/vulnerability, rotation, install/rollback or complete product acceptance gates.

### Document protected provenance setup

Added the policy schema/ownership contract and installer argument guidance for the now-required acquisition provenance policy. Updated release acquisition documentation to distinguish lower-level TUF evidence acquisition from installer-owned authenticated provenance enforcement and remove stale claims that protected repository loading/cache initialization are absent. The guide explicitly records exact source-commit/parameter pinning and unfinished controlled replacement/rotation, preventing an implication that current policy enables arbitrary future updates. Documentation diff checks passed. CI 36995084053 remains in progress; commits stay local until it becomes terminal. Complete product acceptance remains unfinished.

### Reopened inspection repeats provenance qualification

The shared owned inspection reopening path now invokes protected provenance verification before opening staging. Launch preparation and result collection/readback inherit the same admission rather than relying only on earlier staging. Native fixtures remove retained provenance after successful staging and require explicit provenance refusal for reopening and launch preparation, no launch environment publication, then restore evidence for existing success checks. Portable installer/update tests, Linux amd64 compilation and diff checks passed. This requires the retained open acquisition descriptor and evidence; durable restart reacquisition/lifecycle integration remains unfinished. CI 36995741066 at 1e7b81d is live for preceding acquisition/staging changes, so this follow-up remains local pending terminal qualification. Full application acceptance remains unfinished.

### Post-collection provenance requalification

Owned inspection result handling now repeats package/provenance qualification after the collector returns and before exposing evidence, alongside existing repository/service and retained identity checks. This closes the gap where mutable retained provenance could be removed during manager/journal collection after initial reopening admission. Native mutation fixtures now remove provenance inside collection, require ErrProvenanceBinding and zero result, restore evidence and prove stage-lock release through reopening. Portable installer/update tests, Linux amd64 compilation and diff checks passed. CI 36995741066 remains live for preceding acquisition/staging integration; these reopening/post-collection changes remain local awaiting publication. Complete application acceptance remains unfinished.

### Complete CLI provenance preparation coverage

Extended the real parsePreparation fixture, including independently signed catalog and threshold TUF bootstrap, to require exact reviewed provenance bytes in the resulting configuration when full repository/file/pin arguments are supplied. It refuses file-only, pin-only, policy without repository, policy without bootstrap, and wrong digest inputs. This covers CLI argument wiring in addition to the separate no-follow pinned file reader tests. CLI tests and diff checks passed. Native CI 36995741066 remains live for earlier integration; reopening/result requalification and this coverage remain local pending publication. Full product acceptance remains unfinished.

### SBOM primary package binding

Added a bounded CycloneDX 1.6 primary-component binding primitive requiring metadata.component to identify the HomeNode application and independently retained release, with exactly one matching SHA-256 package claim. Exact field lookup and deterministic object parsing refuse casing aliases, duplicated syntax, absent hashes, ambiguous SHA256 claims and package/release substitution. Update tests and diff checks passed. Primary schema research: https://raw.githubusercontent.com/CycloneDX/specification/master/schema/bom-1.6.schema.json. This is not full schema validation, dependency completeness, licensing/vulnerability review or installation authority; production integration and those semantic gates remain required. CI 36995741066 remains live for preceding integration; follow-ups remain local pending publication. Full product acceptance is unfinished.

### Owned acquisition and inspection SBOM binding

Added a shared locked evidence qualification helper combining owned provenance verification with CycloneDX primary-component package/release binding. Acquisition, staging, reopening and post-collection result exposure use it; the explicitly named provenance API retains its narrower contract. Native fixtures require missing SBOM to refuse before staging publication, then provide independently retained package-hash/release SBOM for success, and remove SBOM during collection to require ErrSBOMBinding, zero result and lock release after restoration. Portable installer/update/CLI tests, Linux amd64 compilation and diff checks passed. CI 36995741066 remains live for preceding integration; these follow-ups remain local until publication. This is not full SBOM schema/inventory/licensing/vulnerability review; those gates and full application acceptance remain unfinished.

### SBOM hash entry shape refusal and native qualification

Primary SBOM binding now refuses null/empty/non-object hash entries, missing or non-string algorithm/content, and extra hash fields rather than silently ignoring malformed entries beside a valid package hash. Fixtures cover these refusals and admit an additional well-formed SHA512 claim. This remains package binding, not complete hash-algorithm/schema/inventory qualification. Portable update/installer/CLI tests and diff checks passed. CI 36995741066 at 1e7b81d completed successfully for acquisition/staging provenance enforcement and owned native fixtures. Later reopening/result rechecks, CLI coverage and SBOM binding integration are being published for native qualification; full application acceptance remains unfinished.

### Development package file SBOM generation

Added a Python standard-library CycloneDX emitter invoked after Debian archive creation. It hashes the final archive and staged usr files, emits sorted external package.sbom.json evidence and explicitly declares incomplete composition. It refuses payload links/special files and empty inventories. Local temporary fixtures independently checked known package/payload SHA256 and symlink refusal; build shell syntax and diff checks passed. Actual native Debian generation, archive-to-inventory comparison, dependency/license inventory and release signing/promotion remain unqualified. CI 36996402338 remains live for earlier evidence integration, so this tooling change remains local pending publication. Full application acceptance remains unfinished.

### Independent extracted-package SBOM comparison

Added a separate verifier that does not import the generator. Development package CI now compares archive hash/version and exact claimed file identities/hashes against independently dpkg-extracted usr contents, refusing duplicate JSON keys, duplicate/foreign/missing claims and payload links/special files. Evidence is bounded to8MiB and inventory to4096 files. Local temporary fixtures admitted matching evidence and refused changed extracted bytes and an unclaimed extra file; diff checks passed. Native build/generation/extraction verification awaits publication and execution. This proves development file inventory consistency only, not third-party dependency completeness, licensing, vulnerabilities or promotion. CI 36996402338 remains live for earlier qualification, so changes remain local. Full application acceptance remains unfinished.

### Persistent development SBOM refusal regression suite

Added five portable Python unittest cases exercising matching archive/payload evidence, changed package or extracted payload, missing/foreign/duplicate claims, wrong hashes/version, extra extracted files, payload links and duplicate JSON keys. CI runs them before real Debian generation and independent extracted-package comparison. All five passed locally with bytecode writes disabled; diff checks passed. Native package generation/comparison still awaits publication/execution. CI 36996402338 remains in progress for earlier owned evidence integration; these tooling commits remain local. Full dependency/license/vulnerability and product qualification remain unfinished.

### Development SBOM generation/verification bounds

Aligned development generator and extracted-package verifier with4096-file and240-byte UTF8 path limits. Generator encodes and checks the8MiB evidence limit before writing output. Both independently refuse a symlinked/non-directory usr payload root, avoiding enumeration of a substituted tree. Added real4097-file, overlong-path and symlink-root refusal regressions; all eight Python tests and diff checks passed. Native generation/comparison remains pending publication. CI 36996402338 remains live for earlier evidence qualification, so tooling follow-ups remain local. Dependency/license/vulnerability and full product acceptance remain unfinished.

### Development evidence shape and completeness refusal

The independent development SBOM verifier now explicitly rejects non-object BOM/metadata/file claims and non-string file names with ValueError instead of incidental attribute/hashability exceptions. It also requires the emitted incomplete composition, refusing false complete/missing declarations from file-only evidence. Added malformed-shape/completeness subcases; all nine portable Python tests and diff checks passed. CI 36996402338 at59ddff9 completed successfully for owned provenance/SBOM binding and native mutation fixtures. Development generation/archive comparison tooling is being published for native qualification. Dependency/license/vulnerability and full product acceptance remain unfinished.

### Generated development SBOM through product binding gate

Added opt-in unprivileged Linux build qualification that independently hashes the actual Debian descriptor, reads its no-follow bounded regular external SBOM, requires ValidateSBOMBinding success for the build release, and refuses wrong release and altered package claim. CI runs it alongside actual package content qualification after generation. Portable update tests, Linux amd64 update test compilation and diff checks passed; actual native build execution awaits publication. CI 36996987302 remains in progress for preceding generation/extracted-file verifier tooling, so this follow-up remains local. This proves interoperability/package binding only, not dependency/license/vulnerability completeness or full product acceptance.

### Actual compiled Go dependency evidence

Added development build tooling to record the six staged Go executables' independently hashed bytes, embedded Go version/main module/dependencies/replacements/build settings using debug/buildinfo.Read on the same opened descriptor. Dependencies are sorted and size/mtime are rechecked after hashing. Debian builds emit external .deb.gomodules.json, retained by the existing development artifact pattern, with explicit incomplete status. An actual compiled test executable fixture independently checks SHA256/toolchain identity and refuses non-executable bytes; tool tests, shell syntax and diff checks passed. Source: https://pkg.go.dev/debug/buildinfo. Native six-binary generation awaits qualification; CycloneDX dependency integration, frontend/system dependency inventory, licenses and vulnerability review remain unfinished. CI 36996987302 remains live, so this follow-up stays local. Full product acceptance remains unfinished.

### Extracted-binary module evidence and replacement qualification

Development CI now regenerates module evidence from independently extracted package executables and compares exact output with staged-build evidence, covering actual binary hashes/build information across package assembly. Added a compiled temporary module fixture requiring original dependency identity/version and local replacement identity/version to survive collection. Initial test incorrectly expected empty replacement version; observed Go buildinfo reports (devel), and corrected the test to require preserving that explicit unversioned state. Module tool tests and diff checks passed after correction. Native six-binary generation/comparison awaits publication. CI36996987302 remains live for preceding development SBOM generation/comparison. Full dependency/license/vulnerability and product acceptance remain unfinished.

### Development evidence distribution and review contract

Added an operator/reviewer guide covering actual-binary module identity/replacements/build settings, independent extracted-package comparisons, fourteen-day development artifact retention, checksum limitations and remaining frontend/system/license/vulnerability/promotion qualification. Builds now emit relative-path checksums for both SBOM and module evidence and CI verifies them; existing artifact glob retains the manifest. Shell syntax and diff checks passed; native generation/checksum execution awaits publication. CI36996987302 remains live for earlier development SBOM qualification. This improves reviewable evidence distribution without asserting authenticated release or full application acceptance.

### Emitted frontend bundle dependency evidence

Added a build-only Vite writeBundle hook collecting npm packages represented in emitted chunk modules, checking installed package name/version against exact lockfile entries, recording archive integrity/resolution and declared license, and hashing emitted assets. External evidence is sorted, bounded8MiB and explicitly incomplete; Debian builds copy it to .deb.frontend.json and include its checksum in retained development evidence. Production frontend build passed, with five packages including transitive scheduler and excluding build-only Playwright. Independent local hashing matched all three emitted assets; shell syntax/diff checks passed. Source: https://vite.dev/guide/api-plugin.html. This is build-graph evidence, not installed-package content authentication, complete license review, vulnerability status or CycloneDX integration; independent extracted frontend comparison and refusal regressions remain required. CI36997612077 remains live for preceding module tooling, so changes remain local. Full product acceptance remains unfinished.

### Frontend dependency evidence refusal coverage

Added Node's built-in test-runner coverage requiring scoped package identity/asset SHA retention without absolute host paths, rejecting installed version mismatch, missing lockfile dependency and empty emitted dependency graph before new evidence publication. All four passed locally; CI runs them after npm ci and before build. Diff checks passed. Actual frontend build/hash success was previously observed locally; independent extracted-package frontend comparison remains unfinished. CI36997612077 remains live for preceding module/build qualification, so frontend tooling remains local pending publication. Complete dependency/license/vulnerability and application acceptance remain unfinished.

### Independent extracted frontend comparison

Added a separate Python verifier comparing exact frontend asset set/hashes against extracted package files and recorded npm dependency name/version/integrity/resolution against reviewed lock entries. It bounds evidence8MiB and asset/dependency counts4096, refuses duplicate JSON keys/claims, symlinks/special files, foreign/missing assets and lock identity mismatch. CI invokes it after package extraction. Current actual frontend build evidence passed locally; a copied extracted-tree fixture with changed index.html refused. Diff checks passed. This does not independently reconstruct bundler reachability, authenticate installed npm contents or qualify license/vulnerability status. Persistent refusal regressions and native execution remain pending. CI36997612077 remains live for earlier module qualification; changes remain local. Full product acceptance remains unfinished.

### Persistent extracted frontend refusal regression coverage

Added six portable Python tests covering matching asset/lock evidence, changed/unclaimed assets, duplicate asset/dependency claims, foreign/name/version/integrity/resolution mutations, false completeness, payload/evidence links, duplicate JSON keys and malformed shapes. Explicit schema type checking refuses boolean true instead of treating it as integer1. All six tests and diff checks passed; CI runs them before real package generation. Updated release evidence guide for the third frontend document/checksum and its actual verification limits. CI36997612077 remains live for earlier module qualification; frontend changes remain local pending publication. Complete license/vulnerability/release/application acceptance remains unfinished.

### Module qualification and frontend publication

CI36997612077 at4e1625f completed successfully, including generated package SBOM through the Go product binding gate, actual six-binary module evidence, extracted module comparison and evidence checksum verification. Before frontend publication, all four Node evidence tests, six Python frontend verifier tests, production frontend build and independent verification of its actual emitted assets/lock identities passed; diff checks passed. Frontend collection/refusal/extracted comparison remains subject to the next native CI run. This is development evidence qualification, not complete dependency/license/vulnerability review, production promotion or full application acceptance.

### Frontend installed manifest identity evidence

Frontend build evidence now records SHA256 of the exact installed package manifest used for bundled dependency name/version/license extraction. Independent verification checks canonical lock-rooted package paths, bounded regular manifest shape with final symlinks refused and that retained hash against the installed review/build tree. Added a same-name/same-version changed-manifest regression, requiring refusal rather than accepting version labels alone. All four Node tests, seven Python verifier tests, frontend production build, verification of actual emitted evidence and diff checks passed. This detects manifest drift, not complete installed-package/archive content authentication or license/vulnerability qualification. CI36998293324 remains live for earlier frontend integration; this follow-up remains local. Full product acceptance remains unfinished.

### Bounded descriptor-based npm manifest evidence reads

Frontend collector and independent verifier now open manifests read-only/no-follow/nonblocking, require regular1..1MiB files, bound descriptor reads to1MiB+1 and compare byte count/size/mtime before and after reading. Collector rejects invalid UTF8 rather than replacing bytes. New Node fixture requires symlink/oversize refusal before evidence publication. Five Node tests, seven Python verifier tests, actual production frontend build, verification of emitted evidence and diff checks passed. These are build-tool final-component/descriptor checks, not parent-path ownership or complete npm archive authentication. CI36998293324 remains live for preceding frontend qualification, so follow-ups stay local. Complete dependency/license/vulnerability and full application acceptance remain unfinished.

### npm verifier descriptor read regression coverage

Added Python verifier regressions requiring manifest symlink/oversize refusal, same-size mutation during an actual retained descriptor read to refuse via before/after metadata, and repeated three-byte short reads to preserve exact accepted identity. The mutation fixture modifies its real temporary inode and independently advances mtime; it requires the changed-manifest error rather than incidental hash mismatch. All ten verifier tests and diff checks passed. CI36998293324 remains live for preceding frontend integration; manifest identity/read follow-ups remain local pending publication. Parent-path ownership, npm archive authenticity, complete licensing/vulnerability and full application acceptance remain unfinished.

### Reproducible frontend evidence review documentation

Updated the development release evidence guide for exact installed-manifest hashing, bounded descriptor checks and their final-file/parent-path limits. Added reproducible collector/verifier commands and extracted-package review guidance requiring independently reviewed lockfile plus matching installed dependency tree. Declared licenses remain review evidence, with notices/archive authenticity and complete release qualification explicitly unfinished. Documentation diff checks passed. CI36998293324 remains live for preceding frontend integration; manifest follow-ups and this guide remain local awaiting publication. Full application acceptance remains unfinished.

CI36998293324 at8293542 failed after package/evidence verification, during guest overlay staging's clean committed checkout admission. The new actual frontend verifier imports local verify_sbom and its CI invocation did not disable Python bytecode writes, leaving an untracked packaging/debian/__pycache__ in the runner. Corrected that invocation to set PYTHONDONTWRITEBYTECODE=1, preserving the clean-checkout requirement instead of broadly ignoring generated files. The preceding frontend package/asset checks ran before this failure; the whole run is not successful qualification. Diff checks passed; native correction and manifest follow-ups need requalification.

### Reproduced bytecode dirtiness and immediate CI cleanliness gate

Added an actual CLI subprocess regression using copied verifier/helper files in isolated temporary directories. It proves normal invocation creates helper __pycache__, while PYTHONDONTWRITEBYTECODE=1 produces identical successful verification without it. This confirms the bytecode mechanism inferred from failedCI36998293324 rather than assuming its correction. CI now prints status and requires a clean checkout immediately after release evidence checks, retaining the later guest staging clean-build gate. All eleven Python verifier tests and diff checks passed. ReplacementCI36998828342 remains live; full corrected native workflow and complete application acceptance remain unproven.

### Exact frontend evidence fields and manifest license claim binding

Frontend verifier now requires the exact schema-one root/dependency field sets and parses name/version/declared license from the same bounded retained manifest bytes used for SHA256. It refuses unknown root fields and altered license claims even when manifest hash remains correct, rather than treating declared license as an unchecked annotation. Existing mutation regressions now include license and unknown-field substitution; all eleven verifier tests, verification of actual emitted frontend evidence and diff checks passed. This authenticates consistency with reviewed installed manifest bytes only, not license permission/completeness, package archive authenticity or vulnerability status. CI36998828342 remains live for earlier corrections; follow-ups remain local. Full application acceptance remains unfinished.

### Bundled npm license/notice source artifacts

Frontend evidence now collects package-root LICENSE/COPYING/NOTICE files (including suffixed variants), retaining strict UTF8 text and SHA256 through the same bounded descriptor reader. Independent verification requires the exact matching source notice set and bytes from the installed review/build tree, preventing changed/omitted text claims beside a valid manifest identity. Added collector notice identity coverage and verifier valid/altered notice claim regression. Five Node tests, twelve Python verifier tests, production frontend build, verification of actual emitted evidence and diff checks passed. These are source artifacts for review, not complete bundled subcomponent notices, permission/redistribution approval, archive authentication or vulnerability qualification. CI36998828342 remains live for earlier correction; follow-ups remain local. Full product acceptance remains unfinished.

### npm notice bounds and refusal regressions

Collector and verifier now cap matched source license/notice files at16 per package, retaining1MiB per-file descriptor limits and8MiB whole evidence bound. Added real symlink, oversized, invalidUTF8 and excessive-count source fixtures; collector requires refusal before new evidence publication and verifier refuses unsafe source evidence. All six Node tests, thirteen Python verifier tests, production frontend build, actual evidence verification and diff checks passed. Complete nested/subcomponent notices, permission review, npm archive authenticity and vulnerability qualification remain unfinished. CI36998828342 remains live for preceding correction; follow-ups stay local. Full application acceptance remains unfinished.

### Readable frontend notice review artifact

Added external frontend-notices.txt rendering directly from collected source evidence, with package/version, declared license, notice path/SHA256/text, explicit incomplete status and explicit missing-notice entries. Both JSON and readable artifacts are bounded8MiB before output. Debian builds retain .deb.frontend-notices.txt alongside evidence and include it in the checksum manifest; release guide lists the fourth artifact. Six Node tests passed including collected text/status output, production frontend build and shell syntax/diff checks passed. Independent readable-notice comparison/regressions and native distribution qualification remain pending. This is reviewable source collection, not complete notices or legal/license approval. CI36998828342 remains live; follow-ups stay local. Full application acceptance remains unfinished.

### Independent readable notice binding and corrected native qualification

Python verifier now independently reconstructs readable frontend notices from source-checked evidence and compares exact bounded no-follow descriptor bytes, including package identity, missing-source/incomplete statements and collected notice text/hash. CLI accepts optional notice artifact; CI supplies the retained package artifact. Added explicit literal expected text and extra-text/false-completeness/foreign-package refusal cases. All fourteen verifier tests, comparison of actual generated frontend JSON/notices and diff checks passed. CI36998828342 atd4d524a completed successfully, qualifying bytecode correction and earlier manifest descriptor identity checks; subsequent exact claim/source-notice/readable artifact changes are being published for native qualification. Full license/vulnerability/release/application acceptance remains unfinished.

### Compiled Go dependency source-sum qualification primitive

Added verifyModuleSums for independently retained compiled dependency records and bounded reviewed go.sum bytes. It matches exact module path/version/h1 identity, requires canonical32-byte h1 encoding, rejects duplicate/malformed inventory lines, missing/unversioned module identity and uses replacement module identity rather than allowing original sums to authorize replacement inputs. Tests admit exact ordinary/replacement sums and refuse changed/duplicate records, original-vs-replacement substitution and local (devel) replacement qualification. Module tool tests and diff checks passed. This primitive is not yet wired to actual build/extracted evidence generation and does not establish main-source provenance, checksum database authenticity, licenses or vulnerabilities. CI36999705841 remains live for earlier notice tooling. Full application acceptance remains unfinished.

### Reviewed Go source sums wired into actual build evidence

Module collector accepts an explicit reviewed go.sum argument, verifies every actual compiled third-party dependency/replacement before output, and records sourceSumsVerified plus exact sourceSumsSHA256. Development build and independently extracted module regeneration both supply committed go.sum, retaining exact comparison. Sum input uses a bounded regular no-follow/nonblocking descriptor with size/mtime/close checks; fixtures require exact bytes and refuse symlink/empty/oversized input. Module tool tests, shell syntax and diff checks passed. Native six-binary source-sum qualification awaits publication/execution. Main-source provenance, checksum database authenticity, Go license notices and vulnerability qualification remain unfinished; CI36999705841 stays live for preceding npm notice tooling. Full application acceptance remains unfinished.

### Full compiled collector qualification refusal fixture

Extended the actual compiled local-replacement fixture into all six fixed package executable slots. Complete observational run must retain six records without claiming sourceSumsVerified. A complete reviewed-sum run against those real binaries must refuse specifically unversioned compiled dependency and emit zero bytes, demonstrating the integration cannot expose partially qualified output. Module tool tests and diff checks passed; release evidence guide now explains reviewed sums/hash and observational-vs-qualified consistency limits. Actual production six-binary reviewed sums still await native publication/execution. CI36999705841 remains live for earlier npm source notices; changes remain local. Full application acceptance remains unfinished.

### Bounded complete module evidence output and notice qualification

Module collector now serializes/checks8MiB evidence before output and propagates sink errors or short writes rather than reporting truncated evidence success. Fixtures require explicit output error retention, ErrShortWrite, zero publication for oversized encoded records, and exact complete newline-terminated JSON. Module tool tests and diff checks passed. CI36999705841 at98e6ac9 completed successfully for exact frontend source manifest/license/notice binding, unsafe notice refusal, readable artifact reconstruction, retained checksum and clean-checkout checks. Compiled reviewed-sum integration/full refusal/output follow-ups are being published for native qualification; Go license/vulnerability, full release and application acceptance remain unfinished.

### Compiled module source identity ambiguity/bounds refusal

Reviewed-sum qualification now caps each binary dependency list4096, original/effective path2048 and effective version256, refuses duplicate original dependency paths and nested replacement chains, and requires47-byte h1 encoding before base64 allocation. Reviewed inventory uses line iteration instead of whole-input split and caps line length8192 and retained entries65536. Added nil/duplicate actual identity, oversized sum and nested-replacement refusal fixtures; module tool tests and diff checks passed. Native current six-binary reviewed-sum integration awaits CI37000347832, confirmed live. Source/main provenance, license/vulnerability qualification and full application acceptance remain unfinished; follow-up stays local pending publication.

### Package-wide compiled dependency consistency

Compiled source-sum qualification now retains effective path/version/sum per original dependency across the packaged binaries and refuses conflicting identities, even when both versions/replacements individually match reviewed go.sum. This prevents mixed compilation inputs within the development package from being silently qualified by a multi-version source inventory. Added consistent cross-binary admission and individually authorized mixed version/replacement refusal fixtures; module tool tests and diff checks passed. Current native six-binary qualification remains pending CI37000347832, confirmed live. Go license/source/main provenance, vulnerabilities, full release and application acceptance remain unfinished; follow-up stays local.

### Complete collector real versioned dependency success qualification

Added a compiled fixture using the repository's pinned chacha20poly1305 dependency without network access, placed its actual bytes in all six fixed executable slots, and ran the complete collector against reviewed go.sum. It requires sourceSumsVerified, exact independently computed inventorySHA256, six binary records and nonempty actual dependencies. Changing an actual compiled dependency's source sum in the reviewed inventory must refuse and emit zero bytes. Module tool tests and diff checks passed. This provides local real compiled positive/negative integration evidence, while actual development package six-binary native qualification remains pending CI37000347832. Main-source provenance, Go licenses, vulnerability review, release and full application acceptance remain unfinished; follow-up stays local.

CI37000347832 at66abf41 completed successfully, including actual development six-binary source-sum qualification and independently extracted evidence comparison. Later ambiguity/bounds/package-wide consistency and real compiled fixture follow-ups are being published for native qualification; complete application acceptance remains unfinished.

### Compiled Go module notice source collection primitive

Added a collector requiring qualified compiled source-sum evidence and matching source-directory metadata by effective module path/version/sum before collecting package-root LICENSE/COPYING/NOTICE sources. It refuses ambiguous/mismatched source metadata, source-directory links and final notice links, caps modules4096/notices16 and reuses bounded descriptor reads; output retains source text/SHA256 without absolute host paths and declares incomplete. Three portable tests passed for exact identity, wrong/unqualified/duplicate metadata and notice link refusal; diff checks passed. This primitive is not yet wired to build source metadata/cache integrity/independent verification or readable notices, and cannot establish full licenses/redistribution permission, vulnerability or release acceptance. CI37000898453 remains live for preceding source-sum checks. Full application acceptance remains unfinished.

### Go notice collector bounded command-line inputs

Added CLI input for qualified compiled evidence and Go's concatenated module metadata stream. Both use bounded no-follow regular-file reads; source objects reject duplicate JSON keys, malformed/trailing data, empty streams and more than4096 records. Output is serialized and bounded8MiB before publication and refuses short writes. Boolean schema aliases are refused. Five portable tests passed, including complete multi-object stream admission and zero output on malformed inputs. CI37000898453 at9708248 completed successfully for the preceding compiled dependency consistency and real compiled qualification fixtures. Source cache verification, build integration, independent notice verification and full application acceptance remain unfinished.

### Go notice sidecar integrated into development package build

Development build now checks `go mod verify` before and after collecting notices against actual qualified compiled module evidence plus Go source metadata; only after both checks does it retain `.deb.go-notices.json` and include it in the relative-path evidence checksums. CI invokes five portable collector tests. Local actual six-executable build/compiled-evidence/source-metadata collection retained30 modules and32 notice files with final cache verification passing. Shell syntax and diff checks passed. Documentation states cache-record trust/concurrent-writer limitations and unfinished independent/readable notice verification and license review. Native Debian integration awaits publication after preceding CI37001531587, currently live; full application acceptance remains unfinished.

### Independent Go notice evidence verification

Added separate verification of exact compiled module coverage, effective source path/version/sum and module-root notice names/hash/text. It reconstructs notices without invoking collection, sharing only bounded input/parsing helpers. Regression fixtures admit valid evidence and refuse foreign module/version/sum, missing/duplicated module coverage, changed text/source bytes, boolean schema, unknown inventory fields and notice links. CI runs those fixtures and compares sidecars against independently extracted compiled evidence with cache verification before/after source reads. Portable verifier test and diff checks passed. Native integration remains pending publication; preceding CI37001531587 remains live. Readable Go notices, full license/vulnerability and application acceptance remain unfinished.

### Readable Go source notice artifact

Collector now emits bounded `.deb.go-notices.txt` alongside JSON source records, with effective module version/source sum and collected notice names/hash/text. Build retains/checksums both after cache verification. Independent verifier reconstructs readable contents from verified source records and CI supplies the retained text artifact. Mutation fixture requires altered readable text refusal. Five collector tests, verifier regression, shell syntax and diff checks passed. Preceding CI37001531587 remains live, so follow-ups stay local pending native publication. Complete licenses, nested notices, vulnerability/release and full application acceptance remain unfinished.

### Actual six-executable readable notice qualification

Ran complete local six-command builds, compiled reviewed-sum collection, actual Go source metadata, JSON/readable notice generation and independent verification with cache verification before/after. It verified30 actual modules and150749 readable bytes, then specifically refused appended text in that actual artifact. CI now repeats the changed-readable-artifact refusal against the extracted development package and requires the intended refusal reason. Diff checks passed. CI37001531587 at462f7ad completed successfully for preceding collector input changes and existing package/browser workflows. Accumulated notice build/independent/readable integration is being published for native qualification; full license/vulnerability/release and application acceptance remain unfinished.

### Reachable Go vulnerability CI gate

Added pinned govulncheckv1.8.0 source-context scan of all packages, verbose retained report14days, five-minute timeout and pipefail preserving scanner/tool/database failure. Upload action uses the repository's existing pinned commit. Local scan of29 root packages/31 modules plusGo1.26.8 standard library completed: zero symbol/package findings, one module-onlyGO-2026-5932 for unimported deprecatedx/crypto/openpgp. No blanket vulnerability/release approval is claimed; packaged binaries, platform/tag matrix, npm and host/guest/model checks remain unfinished. Diff checks passed. CI37002107253 remains live for preceding Go notice package integration; follow-up stays local pending publication. Full application acceptance remains unfinished.

### Actual packaged Go binary vulnerability gate

Extended CI vulnerability qualification to all six fixed executables in the independently extracted Linux/amd64 package, using pinned govulncheckv1.8.0 binary mode/text/verbose, ten-minute limit and pipefail. Reports plus adjacent executable SHA256s are retained with the source report14days even on failures. Local Linux/amd64 cross-build and binary scan of all six completed successfully: no symbol/package findings; controller/inspection retain visible module-onlyGO-2026-5932 for unimportedopenpgp. No staged executable was run. Source/binary scanner gates do not complete npm/distribution/guest/model/platform matrix/security review. Diff checks passed; CI37002107253 remains live for earlier package notices, so source/binary gate changes remain local pending publication. Full application acceptance remains unfinished.

### Frontend runtime and build dependency vulnerability gate

Added explicit npm audit JSON gate after locked installation, covering build as well as runtime dependencies. High/critical findings and audit errors fail via pipefail; five-minute limit and always-retained14day JSON/lockSHA256/Node/npm versions preserve review context. Local complete frontend audit reported zero known vulnerabilities; production-only audit also passed. No automatic dependency fixes were applied. Lower-severity findings require release disposition and registry data does not establish package authenticity/bundle reachability/complete security. Diff checks passed. Preceding CI37002107253 remains live for Go notice integration; Go/frontend vulnerability follow-ups remain local pending publication. Full application acceptance remains unfinished.

### Vulnerability report source-context retention

Source vulnerability reports now retain checked Git revision; Go additionally retains go.mod/go.sum SHA256 and scanner-reported version, while frontend retains revision beside existing lock hash/tool versions. This prevents retained source reports lacking the basic review context needed to compare with promoted source; it is not authenticated provenance. Pinned scanner version command and diff checks passed. Reviewed backup control integration: browser currently exposes only durable outcomes; private isolated coordination exists but public approved credential launch/setup remains missing and is the next workflow gap. CI37002107253 remains live for notice integration, so scan follow-ups remain local. Full application acceptance remains unfinished.

CI37002107253 at7769589 failed after actual Go notice generation, six sidecar checksum checks, package binding and independent extracted/source notice verification succeeded: runner lacks rg for the expected readable-substitution error assertion. Changed that CI assertion to ubiquitous grep; scanner gates/source-context follow-ups are being published with the correction. Native complete acceptance still awaits the next run.

### Registered repository backup approval admission

Added strict backup approval resource derivation: exact canonical64hex registered repositoryId plus bounded request-key hash, rejecting unknown/duplicate/case-alias fields, credentials/paths and changed repository identity. Added identity service admission method consuming backup.create approval and creating the durable maintenance job/barrier in the same transaction, returning no authority on failure. Converted existing transactional rollback/replay fixture to exercise this actual method; added request binding/strict input tests. Identity tests and diff checks passed. Browser setup/credential handoff/launch remain unfinished; method does not grant destination or worker dispatch authority. CI37002642560 remains live for prior scanner/notice corrections, so this follow-up remains local. Full application acceptance remains unfinished.

### Controller backup approval initiation

Added administrator-only POST/backups/approval using trusted host-supplied BackupRepositoryID configuration, strict registered repository request parsing and Idempotency-Key binding, existing UV-required passkey ceremony and no-store response. Missing host configuration refuses503; no peer-selected filesystem destination or credential is accepted. Focused HTTP regression covers unauthenticated/non-admin/missing configuration, wrong target/credential injection/missing key, valid UV-required ceremony and unchanged workload admission. Focused control test and diff checks passed after isolating separate session fixtures. Host provisioning of this configuration and browser credential/launch integration remain unfinished; endpoint grants no backup execution authority itself. CI37002642560 remains live for previous scanner/package changes; follow-up stays local. Full application acceptance remains unfinished.

### Trusted controller repository configuration input

Controller serve accepts explicit backup-repository-id only with an admitted private maintenance listener; server construction validates canonical repository identity and rejects development external-backup configuration. Focused control configuration/approval tests and command compilation passed; diff checks passed. This provides the process input for trusted provisioning, not automatic installer repository registration, credential delivery or backup execution. Existing service templates remain disabled for this optional approval configuration until ownership/provisioning integration is complete. CI37002642560 remains live for preceding package/scanner work; follow-up stays local. Full application acceptance remains unfinished.

### Installer-owned backup approval repository identity

Installer configuration/preview and install-prepare input now accept optional registered backup repositoryId. Plan requires the observed isolated maintenance identity and canonical64lowerhex repository identity; generated root-owned controller service passes the fixed ID together with admitted maintenance listener identity. Existing ownership planning retains unit bytes; no untrusted environment substitution or peer destination input is introduced. Focused configuration fixtures verify missing maintenance refusal, exact protected unit/preview identity, invalid case/length/path and unit-text injection refusal; existing maintenance configuration tests and command preparation compilation passed. Drive/repository registration, installed worker target matching, credential delivery and browser launch remain unfinished. CI37002642560 remains live for preceding package/scanner qualification; follow-up stays local. Full application acceptance remains unfinished.

### Backup approval authority substitution qualification

Added race-enabled actual admission regressions substituting registered repository, request key, exact request body, policy generation and session hash after backup.create grant issuance. Every substitution must return no maintenance token/job, leave workload admission open and preserve the unused grant; transactional rollback/replay qualification remains in place. Focused identity backup race tests and diff checks passed. This qualifies the new public-workflow admission primitive before credential launch integration, not completed drive registration/dispatch/browser backup. CI37003238174 remains live for published backup approval/provisioning changes. Full application acceptance remains unfinished.

### Approved backup controller coordinator composition

Added controller RunApprovedBackup joining exact repository/request/session approval admission to RunAdmittedDispatchedMaintenance with controller-owned workload draining and trusted root/delivery bridges. It validates release/catalog/device dispatch metadata and immutable credential contents before consuming approval, clears temporary password bytes, returns job/snapshot only and leaves caller credential ownership intact. Preflight fixtures refuse disk-backed credentials and malformed release before admission/root/delivery effects; focused control/identity/backup tests and diff checks passed. Existing installed socket dispatcher/completion implementation is reused rather than duplicated. HTTP credential intake, server-owned task lifecycle/recovery, provisioning target consistency and browser launch remain unfinished. CI37003238174 remains live for prior backup approval configuration. Full application acceptance remains unfinished.

### Owned password intake coordinator boundary

Added RunApprovedBackupPassword taking ownership of bounded credential input bytes, creating a sealed read-only descriptor before approved admission, immediately clearing input and retaining/closing the descriptor through coordinator completion with close errors propagated. Cancelled/invalid password/release/unissued grant refusal fixtures require zero job/snapshot, cleared input and open admission with no root/delivery effects. Portable race tests passed and Linux control test compilation passed; actual sealed-memory branches await Linux execution. Mac tests exercise the unsupported-platform refusal, not Linux credential creation. HTTP parsing, server-owned background task lifetime, recovery/drive registration and browser launch remain unfinished. CI37003238174 remains live for prior published backup configuration; follow-up stays local. Full application acceptance remains unfinished.

### Bounded backup credential request framing

Added binary credential request reader: four-byte big-endian exact approval-body length, approved JSON bytes and separate raw password. It caps request4096/password8192, verifies configured repository/request key, rejects malformed/truncated/oversized frames, invalid password bytes and read failures, returns no partial data on error and clears its fixed read buffer on every path. Fixed allocation avoids dynamic read-buffer growth retaining password copies; callers own returned password clearing. Race fixtures inspect cleared underlying read buffers for valid/refused/read-error paths. Focused control race tests and diff checks passed. HTTP routing/media admission, task lifecycle and browser encoding/launch remain unfinished. CI37003238174 atffeaff3 completed successfully for preceding published backup identity/approval/provisioning plus package/vulnerability gates. Coordinator/credential/abuse follow-ups are being published for Linux qualification. Full application acceptance remains unfinished.

### Controller-owned backup task lifetime and shutdown

Added single-job server-owned task context with serialized admission/shutdown and worker enrollment, preserving active resources until run callback/coordinator cleanup returns. Failed admission reserves no task slot; overlapping/stopping tasks reject before admission callback. Close cancels work and waits bounded by caller, with timed-out work remaining tracked. CLI drains backup work before store closure; controller source unit stop budget200seconds accommodates190second drain plus existing cleanup. Race fixtures require overlap refusal, cancellation without false completion, eventual drain and retry after admission failure. Focused control race, installer configuration tests and command compilation passed; diff checks passed. Launch handler still needs to transfer credential ownership into task callback; actual service shutdown/HTTP/browser/recovery qualification remains unfinished. CI37003905120 remains live for prior coordinator/credential parsing changes. Full application acceptance remains unfinished.

### Backup-owned runtime acquisition checkpoint

Authority audit found direct controller-to-supervisor maintenance acquisition is correctly denied: root maintenance socket requires isolated backup UID. Public launch must delegate acquisition to that worker rather than broaden controller privileges; existing injected-root coordinator composition is not yet a valid production wiring. Added backup-peer-only private attach-root checkpoint for exact owned token/job/device, strict separate rootToken payload, and state-owned freezing-to-staging transition. Ordinary maintenance operations reject rootToken and unknown/duplicate/null/alias fields. Portable strict maintenance tests passed. Linux kernel-peer fixture now calls actual checkpoint instead of direct state attachment, requiring wrong-phase and repeated-attachment refusal; Linux test compilation passed, execution awaits publication. Worker preliminary launch/acquisition/cleanup protocol and public/browser launch remain unfinished. CI37003905120 remains live for earlier credential/coordinator changes; full application acceptance remains unfinished.

### Worker client for owned runtime checkpoint

Private maintenance client now sends attach-root with exactly version/token/job/device/rootToken, deriving job/device through its existing owned inspection contract and rejecting malformed root tokens before network effects. Shared strict bounded acknowledgement handling remains unchanged. Race fixture verifies exact five-field request and invalid token transport refusal. Linux control kernel-peer fixture now uses the actual client for successful attachment while preserving wrong-phase/repeated refusal. Focused runtimeclient race tests, Linux control compilation and diff checks passed; real peer execution awaits publication. Preliminary worker launch/runtime acquisition/reconciliation and HTTP/browser launch remain unfinished. CI37003905120 remains live for prior credential parsing/coordinator work. Full application acceptance remains unfinished.

### Isolated worker runtime acquisition preflight

Added backup-peer-only verify-freezing checkpoint requiring exact owned job, freezing phase, no attached root and drained inventory. Worker client additionally binds the intended jobID before transport. Added acquisition sequence under backup identity: qualify owned freezing job, acquire supervisor authority, attach root checkpoint; uncertain attachment returns known token for explicit reconciliation and never releases/retries. Race fixtures require ordering, zero acquisition after failed preflight, retained known authority after failed attachment, no automatic release and cancelled-context zero effects. Linux kernel-peer fixture now checks draining/staging refusal and valid freezing client preflight; focused runtimeclient race tests and Linux control compilation passed. CI37003905120 at5ddc8a5 completed successfully for prior coordinator/password/framing qualification. Task/root-checkpoint/acquisition follow-ups are being published; actual worker preliminary launch and HTTP/browser/recovery integration remain unfinished. Full application acceptance remains unfinished.

### Preliminary launch protocol and credential transport

Added strict version2 preliminary Launch naming approved management job/release/catalog without runtime token, destination/executable or password fields. It is disjoint from acquired Dispatch; duplicate/escaped aliases, unknown/missing/null/version/oversized/trailing input refuse. Worker acquisition composition converts to exact acquired dispatch only after runtime acquisition, retaining known dispatch with attachment errors solely for reconciliation. Existing credential transport now shares authenticated bounded send/receive mechanics across explicit launch/dispatch decoders; activated launch sender checks root listener creator and launch receiver requires non-root controller peer/sealed credential. Protocol/acquisition race tests and portable backup tests passed. Added actual Linux sealed descriptor handoff and cross-protocol refusal fixture; Linux backup compilation passed, execution awaits publication. CI37004718146 atec6eadd completed successfully for preceding backup-owned acquisition/checkpoint changes. Worker launch server/completion, HTTP/browser/recovery integration remain unfinished; full application acceptance remains unfinished.

### Preliminary launch server lifecycle and completion

Generalized existing single-job joined credential server across explicit typed dispatch/launch decoders, adding acknowledged preliminary launch serving without permissive format fallback. Added separate launch completion domain/job reply and activated launch send-and-wait with no replay. Credentials close before successful reply; completion does not replace durable publication/recovery qualification. Portable full backup tests and diff checks passed. Linux socket fixtures cover correct/foreign-job/old-dispatch completion domains plus actual launch callback, received credential closure before reply, source descriptor retention and cancellation drain; Linux backup compilation passed, execution awaits publication. CI37032702125 remains live for prior preliminary launch transport. Actual installed worker acquisition/publication/cleanup and HTTP/browser launch remain unfinished. Full application acceptance remains unfinished.

### Preliminary worker acquisition/publication composition

Added RunCredentialedLaunchedBackup for isolated backup identity: validate preliminary protocol against trusted installed worker metadata/paths/target, qualify sealed credential, confirm owned freezing job, acquire/checkpoint root authority and execute existing registered repository/staging/publication pipeline. Returned acquired dispatch is retained on later error for reconciliation. It consumes/closes owned descriptor and clears temporary password, never releases root while publication/callback completion may be active. Shared installed worker input validation serves acquired and preliminary paths. Configuration refusal fixture requires no acquired dispatch/result/root effect and descriptor closure; focused runtimeclient race tests and diff checks passed. Audit confirmed restoration/root release still require acknowledged worker stop; new function is not yet activated by installed worker server until cleanup transport is completed. CI37032702125 remains live for earlier protocol transport. HTTP/browser launch and complete recovery/hardware/application acceptance remain unfinished.

### Backup-peer stopped-job runtime release checkpoints

Added private verify-restoring/ack-root-release operations accepting exact root-token ownership payload from authenticated backup peer. Both require matching owned restoration job and acknowledged stopped worker; release acknowledgement retains state method's transactional rechecks before clearing root token. Worker-side release sequence confirms eligibility, releases supervisor authority, then records exact acknowledgement, never resumes apps/reacquires or retries automatically. Failed preflight/release/checkpoint keeps controller recovery evidence. Linux kernel-peer fixture now uses actual private release client/ack instead of directly clearing state and refuses checkpoint replay. Focused control/runtimeclient tests, release/acquisition race fixtures, Linux control compilation and diff checks passed; live peer execution awaits publication. Cleanup request transport/worker activation and HTTP/browser launch remain unfinished. CI37032702125 remains live for earlier preliminary protocol work; full application acceptance remains unfinished.

### Explicit cleanup protocol and isolated worker release composition

Added strict version3 Cleanup carrying only exact job/device/management/root identity, disjoint from launch/publication dispatch and rejecting commands/paths/unknown/duplicate/alias/null/trailing input. Added authenticated sealed-descriptor cleanup send/receive, distinct cleanup completion domain with bounded wait, and typed single-job acknowledged cleanup server. Registered worker cleanup validates fixed management path, qualifies stopped ownership through backup-only controller checkpoints, releases supervisor token and records acknowledgement without repository access or app resumption. Protocol/release/configuration refusal race tests and Linux backup compilation passed; diff checks passed. CI37032702125 at51e3ea2 completed successfully for preceding preliminary transport. Combined installed worker routing/cleanup client orchestration, HTTP/browser launch and full recovery remain unfinished; follow-ups are being published for native qualification. Full application acceptance remains unfinished.

### Installed worker explicit launch/cleanup routing

Installed registered worker now accepts exactly reviewed preliminary launch/version2 and stopped-job cleanup/version3 schemas on authenticated credential channel, rejecting legacy acquired dispatch and mixed/duplicate/trailing envelopes. It routes launch through backup-owned acquisition/publication and cleanup through stopped/restoring qualification plus root release checkpoint, creating fixed maintenance client per operation. Added trusted maintenance-socket process argument defaulting/run/homenode/maintenance.sock, canonical-path and channel-alias refusals. Full portable backup/runtimeclient/worker-command suites and Linux backup/command compilation passed; dedicated schema-routing and command alias refusal fixtures passed; diff checks passed. Live installed worker protocol, cleanup sequencing, controller launch/cleanup clients and HTTP/browser workflows remain unfinished. CI37033797648 remains live for prior worker/server/release changes. Full application acceptance remains unfinished.

### Controller preliminary launch/cleanup clients and durable intent

Added activated controller clients for typed preliminary launch and stopped-job cleanup, sharing protected socket validation and retaining caller credential ownership. Invalid protocol/configuration and cancelled contexts refuse before delivery. Added transactional ClaimBackupLaunch requiring exact owned freezing job, no runtime token, and empty workload inventory; it records uncertain worker intent before handoff/acquisition and refuses replay. Restart fixture verifies retained uncertainty blocks restoration while permitting worker root attachment; publication is required before completion. Status inspection now represents uncertain pre-acquisition freezing jobs rather than rejecting legitimate launch intent. Full runtimeclient/state race suites and diff checks passed. CI37033797648 atb534817 completed successfully for preceding worker/server/release composition. Controller orchestration, HTTP/browser backup initiation and full recovery/hardware/application acceptance remain unfinished.

### Controller coordinator using isolated launch and cleanup

Added RunAdmittedLaunchedMaintenance for exact approved draining job ownership. It validates sealed credential and claims exclusive runner, drains apps, records preliminary uncertain intent, and launches isolated worker without accepting a controller runtime bridge. Exact completion plus independently inspected owned publishing/root and durable publication evidence is required before worker completion checkpoint. Cleanup uses independent bounded context, typed stopped-job request, and requires durable root clearance before app restoration/admission completion. Failures retain requires-action evidence without transport replay or direct root acquisition/release. Linux fixture covers successful ordered publication/cleanup, lost launch/completion, missing publication, failed cleanup and unrecorded release; Linux test compilation and diff checks passed. Portable dependency refusal fixture and full portable backup race suite passed. Native fixture execution awaits publication after CI37034877995 finishes; HTTP/browser wiring, failure recovery and complete application acceptance remain unfinished.

### Approved backup execution uses worker-owned runtime authority

Replaced controller RunApprovedBackup/Password runtime-bridge/acquired-dispatch parameters with explicit preliminary Launch and Cleanup callbacks. Preflight validates preliminary installed release/catalog/device metadata and requires both worker operations before approval consumption. Approved job runs through RunAdmittedLaunchedMaintenance; controller never acquires/releases runtime authority. Existing preflight/password refusal tests adapted, preserving caller descriptor and clearing owned password on every path. Added missing launch/cleanup refusal fixture requiring no delivery/admission effects. Full portable control race suite, focused approved-backup tests, Linux control test compilation and diff checks passed. CI37034877995 remains active for previously published clients/intent; coordinator and approved-execution changes remain local until it finishes. HTTP asynchronous task/browser/recovery integration and full application acceptance remain unfinished.

### Approved backup server-owned task handoff

Added StartApprovedBackupPassword: clears owned password, creates sealed credential, preflights trusted preliminary configuration, and consumes approval/durable maintenance admission synchronously inside serialized backupTasks admission. Successful admission transfers credential to server-owned coordinator closure and returns only public job ID; request cancellation does not become work cancellation. Refused admission closes descriptor; task closes its descriptor after coordinator completion on all paths. Shared preflight with synchronous helper avoids inconsistent installed metadata qualification. Added password-clearing/no-admission refusal and failed-task owned descriptor closure fixtures. Full portable control race suite, Linux control test compilation and diff checks passed. CI37034877995 remains live; these changes remain local to preserve its native qualification. HTTP routing/installed worker configuration/browser backup and complete recovery/application acceptance remain unfinished.

### Backup HTTP initiation and installed execution configuration

Added administrator-only POST/api/v1/backups using dedicated application/vnd.homenode.backup-credential bounded frame; global middleware accepts that media only for this exact POST route while preserving TLS/origin/authentication/concurrency/security headers and imposing credential-specific body cap. Handler refuses query metadata, validates exact registered repository/request key via frame reader, supplies installed release/catalog/worker callbacks, consumes X-Action-Approval through server-owned admission, and returns202 with public job ID only. BackupExecutionConfig requires production runtime/repository/both worker callbacks and valid preliminary metadata; New copies configuration value to avoid subsequent external pointer edits. Added HTTP fixtures for unauthenticated/unconfigured/cross-origin/wrong media/malformed/oversized/query/cross-route refusals, no secret disclosure/no-store and unchanged admission; configuration refusal matrix added. Full portable control race suite, Linux control test compilation and diff checks passed. CI37034877995 at91411aa completed successfully for prior installed worker routing/controller clients/durable intent. Coordinator/task/HTTP follow-ups are being published for native qualification. CLI installed execution wiring, successful HTTP/browser workflow, complete recovery/hardware/application acceptance remain unfinished.

### Serve command installed backup execution wiring

Added serve --backup-release/--backup-catalog-version and fixed-default --backup-credential-socket configuration. Enabled execution requires production Linux non-root controller, admitted private maintenance listener, configured runtime/repository, isolated system-range controller primary GID and canonical distinct credential path. Typed launch/cleanup callbacks come from protected ActivatedBackupDispatcher and metadata comes only from command configuration. Disabled execution keeps existing serve behavior. Added refusal matrix for unavailable integration, missing repository, invalid release/catalog/GID/path/channel aliases and valid callback mapping. Full command race suite, Linux command test compilation and diff checks passed. Previous push session90447 remains live awaiting Git response; no replacement push started. Installer execution metadata generation, successful HTTP/browser workflow and complete recovery/application acceptance remain unfinished.

### Installer controller backup execution metadata

Added install-prepare --backup-release and Configuration/Preview trusted release field. Optional execution requires registered repository, isolated maintenance identity, controller system-range GID and strict preliminary release validation. Controller unit appends installed release plus catalog version taken from the verified signed catalog, preserving root-owned0644 service file. Added preview/unit matching and command/environment/newline injection refusal fixtures plus missing repository refusal. Fixed test import during verification; full portable installer/command race suites, Linux installer test compilation and diff checks passed. Push session90447 remains live awaiting Git response. Registered-drive qualification, worker backup.env generation, successful browser workflow and complete recovery/application acceptance remain unfinished.

### Installer isolated worker environment

Added install-prepare --backup-drive-uuid and preview field. Optional backup execution now requires validated registered target UUID/repository at fixed/mnt/homenode-backup; drive-only incomplete configuration refuses. Generates root-owned0600/etc/homenode/backup.env with trusted controller UID/GID, registered UUID/repository, installed release, verified signed catalog version and catalog floor. No repository password is stored. Added exact installer path allowlist entry after test detected missing entry. Fixtures check exact metadata/ownership/mode and missing/path/newline/environment/specifier UUID injection refusals. Full portable installer/command race suites, Linux installer compilation and diff checks passed. Drive discovery/actual registration qualification, native installed worker activation, successful browser workflow and complete recovery/application acceptance remain unfinished. Push session90447 remains live with no response; no replacement push started.

### Worker completion-to-cleanup admission ordering

Integration audit found acknowledged worker reply could become visible before single-job slot release, allowing immediate cleanup connection to be discarded as overflow. Added admission mutex spanning successful callback completion send and slot release, so new operation admission waits until reply transition finishes; callback/credential still finish before acknowledgement. Completion-send failure cancels serving before opening admission and failure recording is bounded/nonblocking. Added Linux regression fixture holding completion transition while next socket connects, requiring connection remain queued then process after release and joined shutdown. Full portable backup race suite, Linux backup test compilation and diff checks passed; native regression execution awaits publication. Push session90447 remains live with no response; no duplicate push started. Browser/recovery/physical hardware/security/full application acceptance remain unfinished.

### Browser approved backup initiation

Added administrator-only no-store configuration endpoint returning enabled state and registered repository ID, without installed runtime paths/release. Browser external-backup panel now offers registered-drive password form when enabled, clears DOM field on submission, verifies passkey against exact repository-only approval bytes and sends big-endian metadata/raw credential frame with same request key/action approval on dedicated bounded route. Typed credential buffers clear on success/refusal/passkey cancellation; no automatic retry. Shared approveExactAction helper preserves existing approved-action behavior. UI reports started job separately from publication/restore evidence and explains workload pause/offline drive protection. Configuration endpoint auth/disabled/enabled/no-disclosure/cache fixtures, full control race suite, frontend production build and existing enrollment/pair/revoke/recovery browser regression passed. New successful encrypted-backup browser workflow and credential-frame/passkey-specific browser fixtures remain unverified/unfinished. Prior pending push completed at8d8281c and CI37037070237 is live; browser follow-up kept local. Full recovery/hardware/security/application acceptance remains unfinished.

### Browser backup framing and passkey cancellation fixture

Extended browser regression with mocked registered backup configuration/transport and actual virtual passkey assertion. Requires approval body contain exact repository-only JSON, credential frame preserve exact approved bytes, big-endian length and UTF-8 password, identical request key and action approval; job-start acknowledgement and cleared password DOM are checked. Simulated NotAllowedError cancellation requires cleared field and no further backup POST. Scoped existing status-error selector after cancelled form alert exposed ambiguity. Full browser regression passed. These transport fixtures do not qualify an actual encrypted drive backup. CI37037070237 completed with failure in Linux coordinator lost-launch fixture; fixes underway to preserve the state model's mandatory unacknowledged freezing checkpoint.

### Preserve unacknowledged launch acquisition checkpoint

CI37037070237 at8d8281c failed Linux lost-launch coordinator fixture: coordinator attempted forbidden freezing-to-requires-action transition before acknowledged runtime attachment. State correctly retained freezing/uncertain barrier, but coordinator cleanup attempted invalid transition and fixture expected wrong phase. Coordinator now explicitly preserves freezing checkpoint for worker attachment/reconciliation; other eligible failures still become requires-action. Fixture requires freezing after lost preliminary reply and additionally checks worker-stopped refusal for all uncertain launch/publication/completion paths. Full portable backup race suite, Linux backup fixture compilation and diff checks passed. Native rerun awaits publication; complete recovery and full application acceptance remain unfinished.

### Recovery after acknowledged backup runtime release

Added RecoverReleasedBackupMaintenance under exclusive maintenance runner for exact owned job/device, restoring/requires-action phase, empty retained runtime token, matching durable published outcome and explicit worker-complete observation. It retries app restoration and admission completion only; it never acquires authority or replays publication/uncertain worker cleanup. Unlike general stopped-worker eligibility, missing dispatch record does not qualify because this recovery requires affirmative completion evidence. Portable race fixtures cover successful acknowledged release and refusal for freezing, active worker, retained root, foreign device and missing completion record; Linux backup test compilation and diff checks passed. CI37037597815 remains active for prior coordinator/browser fixes. Public passkey-approved recovery routing, uncertain worker/release reconciliation, new-host restore, hardware/security and full application acceptance remain unfinished.

### Released backup restoration failure checkpoint and restart retry

Recovery now records restoration failure/cancellation under independent bounded checkpoint context, retaining exact job/device, cleared root, completion/publication evidence and requires-action phase without reopening admission. Added app restart failure and cancellation-during-restoration fixtures requiring retained phase/root/admission, closing and reopening the database, then successful qualified restoration retry and reopened admission. Focused recovery race suite, Linux backup test compilation and diff checks passed. CI37037597815 remains live for prior changes; released-recovery changes remain local until native qualification finishes. Public recovery approval/routing, uncertain worker/root reconciliation, new-host restore and complete application acceptance remain unfinished.

### Fresh approval binding for released backup resumption

Added backup.resume approval action and strict exact empty-object resources binding public job ID plus hashed request key. Added transaction-level AuthorizeReleasedBackupRecoveryTx requiring exact active owner device/job, released root, restoration/action phase, affirmative completion marker and matching published outcome; returned maintenance authority remains private and usable only after approval transaction commits. Identity wrapper consumes grant and qualifies owned state in same transaction, exposing no authority on error. Resource tests reject credential/job body fields, invalid IDs/keys and changed bindings; recovery qualification matrix verifies exact private token on eligible state and none for freezing/active/root-retained/missing completion/foreign devices. Full identity/state race suites, focused recovery race tests, Linux identity compilation and diff checks passed. CI37037597815 atae2a51b completed successfully, qualifying prior Linux coordinator fix and browser changes. Explicit approval rollback/replay fixtures, HTTP/task/browser resume wiring and complete recovery/application acceptance remain unfinished.

### Released backup resume approval transaction and HTTP task wiring

Added approval fixture requiring active-worker qualification failure preserve grant/no private authority, subsequent completion/release permit same grant, changed key/body/policy/session/job refuse, and successful grant replay refuse. Added administrator-only backup/{id}/resume/approval and resume JSON routes; fresh backup.resume approval is consumed in serialized server-owned task admission then existing released-job recovery restores apps with exact private authority. API returns public job ID/restoring202 only; no credentials/root acquisition/uncertain worker replay. Unavailable/active/stopping task returns409. Added unauthenticated route, credential-bearing body, unavailable runtime, cancelled initiation and unchanged admission fixtures. Full identity/control race suites, Linux control compilation and diff checks passed. Successful HTTP-to-restoration and browser resume fixtures/control remain unfinished. Prior push session88620 remains live awaiting response; no duplicate push started. Full uncertain-worker/new-host/hardware/security/application acceptance remains unfinished.

### Approved HTTP released-job restoration qualification

Added actual approved resume HTTP fixture with production-shaped owner/session/job IDs and durable published/completed/released state. Exact grant produces202/public job only, initiating request context cancels after handler returns, server-owned task completes restoration for empty workload inventory and reopens admission, shutdown joins task, grant is consumed and repeated request refuses. Fixture does not qualify live VM restart recovery. Test initially exposed shared seedSession short 'device' ID unsuitable for maintenance; added backup-specific valid-ID session fixture to preflight tests so valid-release refusal reaches credential validation instead of being masked by malformed device identity. Full control race suite, Linux control compilation and diff checks passed. Pending push completed at90bcdad, CI37038762050 is live; follow-up stays local. Browser resume controls, live stopped-VM restoration, uncertain worker/root reconciliation, new-host restore and full application acceptance remain unfinished.

### Public resume eligibility and browser restoration control

Backup outcomes now include resumeJobId only after exact owner/job released-runtime/publication/completion qualification and when server task admission is idle/not stopping; private maintenance authority is discarded, never serialized. Qualification failure offers no resume action; unexpected status-read errors return unavailable. Browser offers fresh-passkey approved resume-workloads action bound to job/request key, reports started restoration and refreshes status, with separate action errors and no data-restore claim. HTTP fixture verifies qualified public ID/private token exclusion and suppression while task active. Updated exact missing-evidence contract fixture for new empty field after test caught expected key count. Full control race suite, Linux control compilation, frontend production build and existing browser identity/backup regression passed. Direct resume-button browser fixture and live VM recovery remain unfinished. CI37038762050 remains live; current follow-up stays local. Full uncertain-worker/new-host/hardware/security/application acceptance remains unfinished.

### Direct browser released-job resumption qualification

Extended browser transport fixture with eligible released-job status and actual virtual passkey assertion for resume. Requires exact job URL, empty approval/action bodies, matching request key/action grant/JSON media, one accepted resume, explicit started-restoration message and removal of action after refreshed status suppresses eligibility. Simulated NotAllowedError cancellation requires no resume POST and re-enabled action; subsequent explicit passkey approval succeeds. Full enrollment/pair/revoke/recovery plus backup/resume browser regression and diff checks passed. These mocked HTTP operations do not qualify live VM restart or encrypted backup recovery. CI37038762050 at90bcdad completed successfully for previous released-recovery/approval/task HTTP changes. Status controls and success/browser fixtures are being published for native qualification. Full uncertain-worker/new-host/hardware/security/application acceptance remains unfinished.

### Weekly acknowledged-publication backup reminders

Product-scope audit found reminder requirement absent. Added sampled weekly reminder derived only from last acknowledged publication: never-published, current with next due timestamp, overdue at seven-day boundary, or unavailable for invalid/future/overflow clock evidence. It does not attest repository health or restore success, automate backup, or send external notifications. Browser shows first-backup/overdue/next reminder alongside separate uncertainty and last-observed status. Added boundary/clock/status/overflow fixtures, updated exact missing-evidence API contract, and browser first-backup/overdue-plus-uncertainty assertions. Full control race suite, Linux control compilation, frontend production build, browser enrollment/pair/revoke/recovery/backup/resume regression and diff checks passed. CI37039477030 remains live; reminder follow-up kept local. Configurable reminder preferences, real encrypted-backup/live VM/new-host recovery/hardware/security and full application acceptance remain unfinished.

### Persistent owner-configurable backup reminder interval

Added server-persisted administrator reminder interval1..90days, default7, canonical stored representation and unavailable status on corruption. Configuration changes require open admission and authorized active administrator, refusing maintenance changes. Dedicated JSON endpoint strictly rejects null/duplicates/unknown/aliases/trailing/fraction/string/out-of-range/oversized input; bounded complete-body read prevents prefix truncation from accepting extra input. Browser form saves actual server preference, refreshes sampled status and displays timing using selected interval, without unattended backups/external notifications. Tests qualify database restart persistence, unauthorized refusal, maintenance immutability, corrupt state and selected-interval overdue boundaries. Full control/state race suites, focused reminder race tests, Linux control compilation, frontend build, browser real14-day save/status/navigation persistence plus identity/backup/resume regression and diff checks passed. CI37039477030 remains live; reminder follow-up stays local. Real encrypted-backup/live VM/new-host recovery/hardware/security and full application acceptance remain unfinished.

### Isolate reminder preference corruption from backup recovery evidence

Audit found reminder-read failure returned503 for entire backup status, hiding valid publication and qualified resume while maintenance blocked preference repair. Status now marks only reminder timing unavailable; durable publication/completion and owned released-job resume qualification remain independently readable. UI proposes7-day repair value for invalid timing without persisting/resetting preference automatically. HTTP fixture proves corrupt preference preserves qualified public resume ID without private-token exposure; settings tests add non-admin denial and escaped duplicate-key refusal. Browser fixture requires unavailable timing/visible repair field while resume remains actionable and independent real endpoint still retains saved14-day preference. Full control race suite, Linux control compilation, frontend build, browser identity/backup/resume/reminder regression and diff checks passed. CI37039477030 atff8bd7c completed successfully; reminder follow-ups are being published. Real encrypted-backup/live VM/uncertain-worker/new-host recovery/hardware/security/full application acceptance remain unfinished.

### Qualify and retain repository before worker runtime acquisition

Audit found preliminary worker acquired/checkpointed runtime authority before authenticating registered external destination. Worker now opens/adopts exact registered mount, authenticates repository/password and retains pinned repository kernel lease before any runtime acquisition; temporary password bytes clear immediately after admission. Existing acquired dispatch publication uses that same repository handle through leased staging instead of reopening destination after acquisition. Missing-drive Linux fixture requires no acquired dispatch/runtime calls/result, closed owned credential and no staging entries. Full portable runtimeclient race suite, Linux runtimeclient fixture compilation and diff checks passed; native execution awaits publication. CI37040526899 remains live. Failed preliminary delivery still retains controller admission uncertainty and needs explicit stopped-worker reconciliation; real encrypted-backup/live VM/new-host/hardware/security/full application acceptance remain unfinished.

### Internal pre-acquisition repository refusal classification

Added explicit ErrLaunchRepositoryAdmission category only when registered repository admission fails before runtime acquisition attempt, preserving original cause through joined error. Configuration/credential/acquisition/lost-ack failures are not inferred from empty runtime token. This category is deliberately not a wire stopped-worker acknowledgement or permission to clear barriers. Linux missing-drive fixture now requires category plus original ErrTarget, zero root acquisition, closed credential and no staging; malformed installed configuration fixture requires no repository category. Linux coordinator fixture adds categorized refusal and still requires freezing/uncertain completion barrier with no cleanup, guarding against premature trust in callback error. Full portable runtimeclient race suite, Linux runtimeclient/backup fixture compilation and diff checks passed. CI37040526899 remains live. Authenticated stopped-refusal protocol/durable reconciliation, real encrypted-backup/live VM/new-host/hardware/security/full application acceptance remain unfinished.

### Job-bound stopped repository refusal reply foundation

Added a separate fixed launch-v2 repository refusal packet and client error category, distinct from worker-local admission failure and successful completion. Receipt requires exact job/domain bytes and an uncancelled context; foreign jobs, legacy dispatch/cleanup replies, appended data and invalid launches fail closed. The sender accepts only the pre-acquisition admission category and never includes private failure details. Portable parser tests and Linux socket cancellation/foreign-job fixtures cover these boundaries. Full portable backup race suite, Linux backup fixture compilation and diff checks passed. Installed server emission and durable controller recovery are not enabled yet: callback joining, credential closure, owned-state qualification and workload restoration remain required before this can clear admission barriers. CI37040526899 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Worker emission of joined pre-acquisition refusal

Acknowledged launch and installed launch/cleanup worker entry points now provide an explicit refusal callback. Generic admission invokes it only for categorized repository admission failure after the worker callback returns and received credential closure succeeds. Admission remains locked through reply and slot release; reply errors stop/join the service. Cleanup or mixed requests cannot emit launch refusal. Added portable cleanup/mixed-schema rejection and Linux injected socket fixture requiring credential closure before receipt and service survival after qualified refusal. Full portable backup race suite, focused refusal race tests, Linux backup fixture compilation and diff checks passed; native fixture execution awaits publication. CI37040526899 remains live. Controller still treats delivery errors conservatively: durable stopped-refusal recording and workload restoration are unfinished, as is full application acceptance.

### Atomic owned stopped-refusal checkpoint

Added dedicated RecordBackupLaunchRefused transaction for authenticated stopped-refusal callers: exact owned freezing job, empty root token, active administrator, drained inventory, no publication claim and existing uncertain launch are required. Marker and restoring phase commit together; general freezing transition stays forbidden. Separate refused observation does not imply publication. Worker-stopped checks accept this marker only for matching rootless restoring/requires-action jobs; completion deletes its marker only after existing workload restoration checks. Fixture covers wrong phase, absent attempt, foreign owner/job, forced phase-write rollback retaining uncertainty, replay, post-refusal root attachment denial, restart persistence and completed admission reopening for empty workload. Full state race suite and focused refusal race test passed. Controller invocation/restoration and failed-restoration retry UI remain unfinished; full application acceptance remains unfinished. CI37040526899 remains active; changes stay local.

### Controller restoration after authenticated repository refusal

Launched coordinator recognizes only distinct stopped-refusal reply category, records owned refusal atomically, rechecks worker-stopped authority, restores workloads under independent bounded recovery context and completes admission restoration. It returns refusal with empty snapshot even after successful recovery, never publication success and never root cleanup when no runtime was acquired. Completion flag prevents error-recording defer from attempting to reacquire a removed journal. Ordinary local refusal/lost reply remain freezing/uncertain. Linux coordinator fixtures add successful rootless refusal restoration with no publication/cleanup and restoration-failure retention as requires-action/refused with admission closed. Linux backup fixture compilation and diff checks passed; native execution awaits publication. Failed-refusal explicit retry workflow and full application acceptance remain unfinished. CI37040526899 remains active; follow-up stays local.

### Explicit restoration recovery for stopped repository refusals

Existing approved recovery authority can now qualify rootless stopped-refusal checkpoints, preserving exact device/job/phase ownership. Published-completion recovery retains its publication requirement. Recovery runner independently requalifies authority in a transaction before workload effects, so contradictory publication evidence blocks refusal restoration. Portable fixtures cover refusal recovery success/failure, no fabricated publication, retained requires-action state and contradictory publication claim denial. Audit corrected refusal recording/recovery to allow independently valid earlier published history while rejecting publication for the refused job or unresolved previous publication. Backup/state/identity/control race suites passed before that history correction; repeat suites are running in session98456. Dedicated prior-history fixture and public status/browser recovery presentation remain required. Full application acceptance remains unfinished.

### Refusal recovery status and retained publication history

Regression now executes a successful synthetic publication, completes it, then records and qualifies a separate refused launch; earlier current/last-published records must remain unchanged through refusal completion. Status qualifies the current maintenance journal instead of deriving its job from retained publication history, making refused recovery available without inventing a snapshot. Browser recognizes refused status and explains paused workload restoration/no new publication. HTTP fixture requires exact owned resume ID, refused status, null publication and no private token exposure. Repeat backup/state/identity/control race suites, full state/control race suites, focused history/refusal/status fixtures, frontend TypeScript/build using explicit ARM Node and diff checks passed. npm build initially selected Intel Node with missing Intel TypeScript optional binary; explicit ARM execution succeeded without dependency changes. Browser integration/approval flow for refusal and native Linux execution remain unverified; full application acceptance remains unfinished. CI37040526899 remains active; changes stay local.

### Approved refusal recovery HTTP and browser verification

Expanded actual approved HTTP recovery fixture to both released published and stopped rootless refused jobs. Each requires exact approval/job/request binding, server-owned restoration after request cancellation, consumed grant, no replay, private-token-free acknowledgment and admission reopening only after restoration. Browser virtual-passkey fixture now exercises both outcomes, including explicit refused explanation, cancellation sending zero resume requests, exact JSON/job/request-key/grant on successful retry and refreshed removal of recovery action. Focused control race test, fresh controller build, full browser identity/pair/revoke/backup/reminder/recovery regression and diff checks passed. Browser transport is mocked and HTTP workload empty: these do not prove physical-drive or live-VM restoration. CI37040526899 Linux job remains authoritatively in_progress; follow-ups stay local. Native new fixtures and full application acceptance remain unfinished.

### Refusal evidence consistency and reply failure audit

Refused worker-stop qualification and sampled observation now recheck publication consistency, rejecting same-job publication or unresolved prior outcomes even after stopped-refusal recording. Regression replaces valid prior current/claim with contradictory same-job published evidence and requires stopped-worker check, observation and completion all fail without reopening admission; restoration of prior history then permits normal completion. Linux injected refusal-send failure fixture requires service cancellation/join, propagated send failure and no reply bytes. Full state/backup/control race suites, Linux backup fixture compilation and diff checks passed. Native send-failure fixture execution awaits publication; CI37040526899 remains active. Hardware/live-VM/end-to-end recovery and full application acceptance remain unfinished.

### Installed backup worker protocol contract

Added contracts/backup-worker.md from current implementation: strict disjoint launch/cleanup schemas, authenticated sealed-credential transport, exact job/domain reply bytes, joined credential closure/admission ordering, local versus authenticated refusal categories, bounded no-replay lifecycle, durable transition qualification, prior-history consistency, fresh approved recovery and private/public authority boundaries. Contract explicitly separates portable/mock/empty-workload evidence from native Linux, installed activation, live-VM, external-drive and power-loss acceptance. Corrected generic server comment to describe explicitly configured successful refusal reply exception. Diff checks passed. CI37040526899 remains in_progress in Build and inspect development Debian package step33; no overlapping push was made. Full application acceptance remains unfinished.

### Enforce read-only credential descriptor access

Credential audit found ReadRepositoryPassword validated anonymous regular file, bounds, seals and close-on-exec but omitted declared read-only access requirement. It now checks F_GETFL/O_ACCMODE and rejects writable sealed descriptors before allocating payload bytes. Linux fixtures separately reject valid sealed writable descriptors and read-only descriptors with CLOEXEC removed; invalid sealed-payload fixture now reopens read-only so payload validation remains independently exercised. Linux backup/runtimeclient fixture compilation and diff checks passed. Portable backup/runtimeclient race suites are running in session12554; native Linux execution remains required. No root integration ran on owner Mac. Full application acceptance remains unfinished.

### Publish refused-launch recovery follow-ups for native validation

CI37040526899 at e9f27b4 completed successfully, including development package inspection. Portable backup/runtimeclient race suites from session12554 passed. Repository-before-runtime acquisition, stopped-refusal transport/server, durable qualification/restoration/approved retry/history consistency, HTTP/browser tests, protocol contract and read-only credential changes are now being pushed for native Linux CI. No success claim for the newly added native fixtures is made before their run completes. Unrelated worktree edits remain unstaged. Full application acceptance remains unfinished.

### Reject final-component replacement symlink during target open

Destination audit found Lstat-to-os.Open race: final target could become a symlink between checks. Target now opens using O_DIRECTORY|O_NOFOLLOW|O_CLOEXEC and wraps that descriptor, then retains existing pinned device/read-only filesystem qualification. This closes final-component symlink following; it is not a claim that all mount/parent replacement races or physical removal scenarios are proven. Linux fixture rejects replacement symlink and checks close-on-exec for real directory. Linux backup fixture compilation and diff checks passed; native fixture execution remains required. CI37043553434 for pushed9b02c8d remains queued; new correction stays local. Full application acceptance remains unfinished.

### Bind target descriptor to admitted mount identity

Mountinfo parser now retains nonzero numeric mount ID. After nonfollowing directory open and device verification, target admission requires descriptor statx AT_EMPTY_PATH/STATX_MNT_ID support and exact admitted mount ID; missing support/mismatch fails closed. Semantics checked against https://www.man7.org/linux/man-pages/man2/statx.2.html. Fixtures cover parsed ID, malformed/zero IDs and native descriptor matching/mismatch. Focused portable target race tests, Linux backup fixture compilation and diff checks passed. This narrows mount replacement races but does not establish complete parent-path/same-mount/power-removal safety; native fixtures and supported installed service syscall-filter compatibility remain required. CI37043553434 remains active; correction stays local. Full application acceptance remains unfinished.

### Reject symlink traversal throughout registered target path

Target directory opening now uses openat2 with RESOLVE_NO_SYMLINKS|RESOLVE_NO_MAGICLINKS and directory/read-only/no-follow/CLOEXEC flags, refusing unsupported kernels rather than falling back to weaker path resolution. Added Linux parent-symlink and proc-root magic-link negative fixtures alongside existing final-component fixture. Semantics checked against https://www.man7.org/linux/man-pages/man2/openat2.2.html. Linux backup fixture compilation and diff checks passed; native execution remains required. Packaged backup.service has no SystemCallFilter allowlist; installed sandbox/mount identity behavior still needs native service validation. Same-mount substitution/mount ID reuse/removal and full application acceptance remain unproven. CI37043553434 remains active; follow-up stays local.

### Bind opened target directory to pre-open observed inode

Target now requires opened descriptor identity to match immediately observed Lstat directory via os.SameFile before existing mount/device/filesystem checks. A replacement directory on the same filesystem cannot substitute between observation and open. Mismatch closes the descriptor and fails target admission. Linux fixture first accepts unchanged identity then renames original/recreates target on same filesystem and requires rejection. Linux backup fixture compilation and diff checks passed; native execution remains required. This is bounded consistency checking, not complete proof against privileged mount replacement/ID reuse or physical removal. CI37043553434 is running private backup listener activation step14; new follow-ups stay local. Full application acceptance remains unfinished.

### Probe required kernel backup boundaries during host preflight

Eligibility now directly probes openat2 no-symlink/no-magic-link resolution on read-only root descriptor and descriptor statx mount identity, rather than assuming Ubuntu release implies syscall availability. Missing observations are actionable failing checks and cannot be deferred as storage-only preparation. Portable policy fixtures cover each absent feature and supported facts retain execution-disabled separation. Hostcheck/install race suites, Linux hostcheck fixture compilation and diff checks passed. Probe describes current checker process only; installed service namespace/security policy still needs independent execution evidence. CI37043553434 remains active at package build/inspection; new changes remain local. Full application acceptance remains unfinished.

### Publish target identity and kernel eligibility follow-ups

CI37043553434 at9b02c8d completed successfully, including native refused-launch and credential fixtures. Added explicit preparation regression proving missing safe path resolution or mount identity cannot be deferred with storage-floor exception. Original local test handle58907 was unavailable on resume; fresh hostcheck/install/backup race execution passed. Final/parent/magic symlink rejection, descriptor mount/inode binding and direct kernel preflight probes are being pushed for native validation. Unrelated worktree edits remain unstaged. Installed-service target admission, physical-drive/VM/replacement-host and full application acceptance remain unfinished.

### Describe refusal recovery without claiming runtime release

UI audit found shared resume text claimed released runtime barrier for rootless repository refusal. Resume explanation now distinguishes stopped-before-acquisition refusal from published/released worker completion. General limitation text distinguishes unavailable backup-data/replacement-host restore from available workload resumption. Browser fixture requires correct refusal explanation and absence of release claim. Frontend TypeScript/build, full browser regression and diff checks passed. Initial build invocation used repository root instead of web directory and failed module lookup; rerun from web succeeded. CI37067528058 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Serialize visible backup controls during approved workload recovery

Status audit confirmed request sequence guards already discard superseded refresh results and backend requalifies recovery. Refresh and reminder editing/saving are now disabled during passkey/resume work, and resume handler refuses while status/reminder work is busy, keeping the visible workflow stable during approval. Frontend TypeScript/build, full browser regression and diff checks passed. CI37067528058 remains live in Go/Linux adapters test step; no overlapping push. Full application acceptance remains unfinished.

### Sample backup initiation availability without granting authority

Configuration endpoint now distinguishes not-configured, available and paused based on configured execution, task admission/shutdown and durable host admission. Errors fail to paused rather than implying availability. It exposes no maintenance token, runtime path or secret, and does not assert drive presence; actual job start retains fresh admission/approval qualification. HTTP race fixture covers idle configured availability, active worker pause and maintenance pause/no-token disclosure. Focused configuration race tests and diff checks passed. Browser consumption/refresh of sampled availability remains required. CI37067528058 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Browser availability refresh before backup credential entry

Backup initiation now consumes sampled availability and offers Check backup availability. Checking clears DOM password immediately, removes stale configuration, rejects superseded/unmounted replies and fails closed without available status. Paused admission hides credential entry with work/maintenance explanation; explicit later check restores an empty field. It does not claim drive presence or grant start authority. Browser regression enters secret, samples paused status, requires no password field, returns available with empty field and verifies no extra backup submission. Frontend TypeScript/build, full browser regression and diff checks passed. CI37067528058 remains live; follow-up stays local. Full application acceptance remains unfinished.

### Separate unavailable backup admission observation from active pause

Configuration now reports unavailable for failed durable admission observation instead of inventing work/maintenance; verified ErrMaintenance and task shutdown retain paused. Browser hides credentials for unavailable/unknown states and explains rechecking without guessing cause. Direct handler cancellation fixture verifies unavailable, shutdown fixture verifies paused. Focused control race tests, frontend TypeScript/build, full browser regression and diff checks passed. Dedicated unavailable browser fixture remains to add. CI37067528058 remains active; full application acceptance remains unfinished.

### Availability failure browser evidence and preflight assertion repair

Expanded browser fixture checks sampled unavailable and failed HTTP availability observations clear/remove password field and later available check restores it empty without new backup request. HTTP error fixture uses actual structured error envelope. CI37067528058 failed browser count expecting8 eligibility rows after kernel probes added2; repaired to10 plus explicit kernel check titles, preserving8 recovery-code assertion. Local stale controller binary previously masked eligibility drift; rebuilt controller then full browser regression passed. Initial fixture edit ran from wrong directory, and initial broad count replacement touched recovery code count; both were corrected before successful verification. Full control race suite and diff checks passed. Native Go/Linux adapters step in failed CI completed successfully. Browser follow-ups are now being published; installed service/drive/VM/new-host and full application acceptance remain unfinished.

### Exact restore manifest schema before compatibility validation

Restore audit found DisallowUnknownFields still allowed Go case-insensitive JSON aliases. DecodeManifest now rejects invalid UTF-8 and requires exact non-null top-level and per-file field names, retaining only optional imageSha256 and existing duplicate/depth/size/trailing/type checks. Missing fields cannot silently become zero values. Compatibility/image/schema checks and payload/database validation remain independent requirements. Fixtures cover canonical marshaled manifest, top-level/nested aliases, null metadata, missing catalog field and invalid UTF-8. Full backup race suite, focused manifest race tests, Linux backup fixture compilation and diff checks passed. Replacement-host orchestration and full application acceptance remain unfinished. CI37068365663 remains active; follow-up stays local.

### Verify publisher schema against strict restore decoding

Inspected Snapshot writer: standard json.Marshal(manifest) emits required zero-valued management protocol and omits only optional image metadata. Added round-trip compatibility fixture with management/files/AI entries using that exact representation, independently approved images, valid schemas/protocols, retained image metadata and null-inventory rejection. Focused manifest compatibility/strict-decoder/publisher-schema race tests and diff checks passed. This verifies representation compatibility, not successful encrypted repository publication or actual VM recovery. CI37068365663 remains active; full application acceptance remains unfinished.

### Bind restore payload verification to observed files and cancellation

Payload verification now rejects cancelled context before filesystem work, binds pre-open regular-file identity to opened descriptor, requires the filename still identify that regular descriptor with declared size after hashing, and checks cancellation before reporting success. Existing caller-held staging lease/immutability requirements remain: these point checks do not prove ongoing immutability after return. Cancellation fixture verifies valid staged content cannot produce cancelled integrity success and an independent uncancelled retry remains valid. Focused integrity/cancellation/schema race tests, Linux backup fixture compilation and diff checks passed. Full application acceptance remains unfinished; CI37068365663 remains active and follow-up stays local.

### Owner backup guide and complete portable regression

Added linked owner guide for current configured external-backup flow, sampled availability/password clearing, approval, publication versus acceptance, preserved history, refused versus uncertain recovery, fresh workload resume and reminder semantics. It explicitly identifies absent drive-registration/data-restore/new-host flows and limits of development evidence; it does not present unfinished release acceptance as operational readiness. Full backup/runtimeclient race suites passed for strict manifest/file-identity changes, and diff checks passed. CI37068365663 remains live at development package build/inspection; follow-ups stay local. Full application acceptance remains unfinished.

### Validate sanitized replacement identity representation

Recovery snapshot validator now requires canonical owner identity ID, unclaimed state, positive int64 epoch and SQLite integer representation before accepting exported management snapshot. It previously admitted arbitrary nonempty IDs and positive fractional epochs. Fixture verifies generated sanitized identity, malformed/short ID and fractional epoch refusal, then valid replacement identity acceptance. Initial fixture staging lacked explicit0700 and failed correctly; fixed private staging. Focused snapshot race tests and full backup race suite passed; full corrected state race suite verification recorded in this turn. CI37068365663 at62da21b completed successfully. This validates snapshot representation, not fresh-host identity activation, policy reconstruction, passkey enrollment or live VM recovery. Full application acceptance remains unfinished.

### Prevent malformed or exhausted recovery epoch emission

Snapshot sanitization now verifies copied source identity epoch is a positive SQLite integer below int64 exhaustion before incrementing it. Missing/malformed/fractional/zero/exhausted epochs fail rather than emitting incompatible recovery identity. Negative fixture verifies no returned path, no retained partial snapshot and unchanged claimed live owner. Full state/backup race suites, focused epoch/source-integrity race tests, Linux state fixture compilation and diff checks passed. Replacement-host activation/trust-floor/re-enrollment orchestration and full application acceptance remain unfinished. CI37069186507 remains active; follow-up stays local.

### Reject malformed application revision metadata during recovery

Snapshot sanitization now requires copied application revisions to be nonnegative SQLite integers below int64 exhaustion before incrementing them. Invalid source values fail without changing the live instance or retaining a partial snapshot. Restore validation independently rejects fractional/negative revisions, with a fixture that mutates a valid exported database and verifies rejection and subsequent valid acceptance. Full state and backup race suites, Linux state fixture compilation and diff checks passed. CI37069186507 remains in progress at9c6ec4e; this follow-up stays local to avoid cancelling it. Replacement-host orchestration and full application acceptance remain unfinished.

### Preserve staging reserve during repository restore extraction

Restore now checks the opened staging filesystem capacity before each declared payload and before writing the manifest, using the existing4GiB reserve and overflow-safe block accounting. Capacity refusal uses existing identity-qualified partial-file cleanup. Checks sample available capacity; they do not reserve space against concurrent external writers. Descriptor fixture verifies actual filesystem accounting and missing/closed descriptor refusal. Focused staging/manifest race tests, Linux backup fixture compilation and diff checks passed; Linux real-restic restore execution remains for CI. CI37069186507 completed successfully at9c6ec4e. Pending recovery follow-ups are being published. New-host activation and full application acceptance remain unfinished.

### Real repository capacity-refusal regression fixture

Expanded Linux real-restic round trip with an encrypted snapshot declaring a policy-valid512GiB disk absent from its payload. On staging filesystems that cannot admit that size with reserve, restore must return ErrStagingCapacity before disk retrieval and remove its earlier extracted management database. This distinguishes capacity admission from later missing-payload failure without allocating a huge disk. Hosts with more available capacity log the branch limitation. Linux fixture compilation and diff checks passed; native execution is not yet verified for this follow-up. CI37069859909 remains in progress; follow-up stays local. Full application acceptance remains unfinished.

### Require exact snapshot-success summary fields

Restic summary decoding now reads exact message_type and snapshot_id map keys and rejects case-insensitive aliases, including mixed canonical/alias records. Present snapshot IDs must be non-null strings; message type remains required. Version-dependent progress/statistics fields remain allowed with existing duplicate/depth/size/UTF8 and unambiguous-summary checks. Regression fixtures cover aliases, null and numeric identity fields without replacing process success or recovery validation. Focused summary tests, full backup/runtimeclient race suites, Linux backup fixture compilation and diff checks passed. CI37069859909 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Restore cancellation checkpoints and empty-staging fixture

Repository Restore now returns observed cancellation after taking repository ownership and before filesystem work, between payloads, before manifest creation and after final directory sync. Cancellation after extraction follows existing identity-qualified cleanup; success cannot ignore a cancellation already observable at the final checkpoint. Real-restic fixture invokes pre-cancelled restore into private empty staging, requires context.Canceled and unchanged emptiness, then independently retries normal restore. Linux backup fixture compilation and diff checks passed; this Linux fixture has not yet run natively for this follow-up. CI37069859909 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Recheck restore reserve while streaming payloads

Bounded restore writer now checks the pinned staging filesystem before each output write for remaining declared bytes plus host reserve. If concurrent activity consumes capacity, subsequent writes refuse before consuming declared bytes and existing restore cleanup applies. Missing/closed staging descriptors fail closed. Linux writer fixtures cover no-output/no-byte-consumption refusal and retain exact-size/oversized behavior. Linux backup fixture compilation and diff checks passed; native execution and loaded syscall/performance measurement remain unverified. Checks sample capacity and do not reserve blocks against concurrent writers. CI37069859909 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Execute bounded restore-writer regression on development host

Moved bounded restore writer into portable implementation and passed the restore deadline to it. Each output write now refuses missing context or observed cancellation before capacity observation or destination writes. Moved exact-size/oversized and missing/closed capacity descriptor tests into Linux/macOS fixtures and added cancelled-output/no-byte-consumption test. These writer and staging-descriptor race tests now ran successfully on macOS rather than only cross-compiling. Linux backup fixture compilation and diff checks passed. Actual Linux encrypted restore tests remain pending publication/execution; CI37069859909 remains active and follow-up stays local. Full application acceptance remains unfinished.

### Publish accumulated restore qualification changes

CI37069859909 succeeded at81dbf9b. Full combined backup/state/runtimeclient race suites passed on current local source, including portable bounded-writer behavior. Verified all five pending implementation commits retain requested sole author identity. Publishing encrypted capacity/cancellation fixtures, exact restic summary identity parsing and streaming cancellation/reserve checks for native Linux CI execution. Passing prior CI does not validate these newer fixtures or establish replacement-host recovery/full application acceptance.

### Qualify recovered guest filesystems before restore success

Added QualifyRecoveryDisks after payload/authority/inventory validation. It opens fixed guest disk names read-only, binds observed path identity to the checker descriptor, runs existing non-repairing ext4 header/full-check qualification and rechecks path identity before closing. Repository Restore now requires this gate before reporting success. Caller exclusive staging ownership remains mandatory; no policy reconstruction, enrollment, application-consistency or activation authority is inferred. Real Linux encrypted round-trip fixture now formats a16MiB ext4 guest disk and independently records its hash/size. Portable fixture verifies honestly checksummed non-filesystem data passes metadata validation but fails disk qualification without modification. Initial added fixture lacked bytes import; corrected before passing focused recovery/writer race suites and Linux compilation. Diff checks passed. Actual ext4 encrypted round trip remains pending native CI execution; CI37070514721 remains active and follow-up stays local. Full application acceptance remains unfinished.

### Encrypted restore fixture separates integrity layers

Extended Linux real-restic fixture to corrupt an ext4 root inode while retaining clean superblock state, independently recompute declared disk checksum and verify metadata/payload validation still succeeds. A raw encrypted repository snapshot then exercises Restore: full filesystem qualification must refuse it with ErrManifest and identity-qualified cleanup must remove all extracted files. This prevents hash/metadata rejection from masquerading as filesystem-check evidence. Linux fixture compilation and diff checks passed; actual native execution is not yet verified. CI37070514721 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Combined regression for restored filesystem qualification

Full backup/runtimeclient/state race suites passed on current source after adding recovered-filesystem qualification and encrypted-corruption fixtures. Owner backup guide now explains implemented read-only ext4 gate and its limits without presenting absent owner-facing restore/new-host activation as available. Diff checks passed. Linux native fixtures remain unexecuted for local8f9d5c5/2f35780 while CI37070514721 remains active at guest tool installation; no overlapping push. Full application acceptance remains unfinished.

### Preserve caller cancellation through recovery-set validation

ValidateRecoverySet now refuses observed cancellation before filesystem inspection, preserves caller cancellation if management snapshot validation fails during interruption, and checks it again before certifying the set. Filesystem qualification inherits those checks. Fixture verifies cancelled metadata/filesystem qualification returns context.Canceled, leaves recovery metadata unchanged and permits independent uncancelled metadata retry. Focused recovery/writer race tests, Linux backup fixture compilation and diff checks passed. Internal validation timeouts and native filesystem/process behavior remain separate evidence; no owner-facing restore workflow is inferred. CI37070514721 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Bound isolated backup and filesystem-check resource use

Backup systemd unit now caps CPU at100% (one CPU worth), disables service swap with MemorySwapMax=0, and explicitly uses OOMPolicy=kill/KillMode=control-group. Existing1GiB memory and64-task limits remain. This bounds inherited restic/e2fsck subprocess resource use and requests whole-service teardown on OOM instead of leaving siblings active. Strict unit parsing/directive checks and diff checks passed. Effective Linux systemd/cgroup enforcement, OOM subprocess shutdown and loaded dashboard performance remain unverified; these settings do not prove worker stop. CI37070514721 remains live; follow-up stays local. Full application acceptance remains unfinished.

### Verify backup resource directives in native activation fixture

Packet activation fixture now parses actual backup unit resource directives with strict source syntax, requires declared CPU/memory/swap/tasks/OOM/group policy, applies them to disposable unprivileged socket-activated service and checks loaded systemd property values. Stream maintenance fixture remains outside this resource check. Python compilation and diff checks passed. Native execution remains pending Linux CI; manager properties do not prove actual OOM sibling termination, CPU enforcement under load or dashboard performance. CI37070514721 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Native backup OOM sibling enforcement fixture

Added explicitly gated disposable Linux/root systemd fixture and ordered CI step. It copies required backup CPU/memory/swap/task/OOM/group directives into an unprivileged test service, forks a sleeping sibling, allocates and touches up to2GiB within the1GiB service ceiling, requires systemd oom-kill result and disappearance of the sibling before bounded cleanup. Unique fixture paths must be vacant; no owner installation execution occurred. Python syntax and diff checks passed. Native execution, portability across the supported systemd/cgroup matrix and actual production subprocess behavior remain unverified. CI37070514721 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Observe live sibling before OOM pressure

OOM fixture now waits for the harness sibling before releasing allocation, verifies its live proc entry and cgroup-v2 membership against systemd service ControlGroup, and checks loaded1GiB memory ceiling. Harness waits on bounded root-controlled allocation gate. Cleanup removes both known marker files. This prevents an already-failed sibling/harness from masquerading as OOM group shutdown evidence. Python syntax and diff checks passed; native execution remains pending. CI37070514721 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Inspect kernel controls before backup OOM exercise

Before allocation gate release, native fixture now requires cgroup-v2 memory.max=1GiB, memory.swap.max=0, memory.oom.group=1 and pids.max=64, plus finite CPU quota equal to its period (one CPU). Primary [kernel cgroup-v2 documentation](https://docs.kernel.org/admin-guide/cgroup-v2.html) confirms quota/period and group OOM semantics; systemd hosted manual fetch returned403, so no successful hosted-manual verification is claimed. These are kernel readback assertions, not a substitute for the actual pending OOM exercise. Python syntax and diff checks passed; CI37070514721 remains active and follow-up stays local. Full application acceptance remains unfinished.

### Preserve bounded OOM fixture failure evidence

Disposable OOM fixture now collects selected systemd result/resource/phase properties and at most50 fixture journal entries before cleanup on failure. Each diagnostic call is bounded to10seconds and diagnostic subprocess failures do not replace the original exception. No production journal or owner data is queried. Python syntax and diff checks passed; native OOM execution remains pending. CI37070514721 remains live in package inspection; follow-up stays local. Full application acceptance remains unfinished.

### Require complete worker exit in OOM fixture

OOM fixture success now requires observed oom-kill result followed by sibling proc disappearance, systemd MainPID=0 and failed service phase within bounded shutdown wait. Result notification alone no longer passes while a worker remains active. Python syntax and diff checks passed; actual Linux execution remains pending. CI37070514721 remains active in package inspection; follow-up stays local. Full application acceptance remains unfinished.

### Publish filesystem recovery and native resource enforcement gates

CI37070514721 succeeded atc8b7e88, including published real-restic capacity/cancellation and portable writer changes. Current pending fixture syntax and diff checks passed; full backup/state/runtimeclient race suites and macOS/Linux command builds previously passed for recovered-filesystem gate. Publishing recovered ext4 checks, checksummed corruption fixture, caller cancellation propagation, backup service resource limits, loaded systemd/kernel control assertions and bounded OOM sibling/main-exit exercise. These newer native assertions remain unverified until this push completes Linux CI. Requested sole author identity remains in every pending commit. Full application acceptance remains unfinished.

### Bounded repository snapshot candidates for restore selection

Added authenticated pinned-repository Snapshots operation with2-minute deadline, no cache, HomeNode tag filter and1MiB output ceiling. Portable parser returns only canonical snapshot ID/time references, rejects duplicate identities/JSON fields, case aliases, malformed/untagged/future entries, invalidUTF8/trailing data and more than1000 entries. Unknown version-dependent restic metadata remains input-only; stored host names/paths/tags are not returned. Empty inventory remains valid. Candidate selection does not certify recoverability or grant restore/start authority. Focused inventory race test, Linux backup fixture compilation and diff checks passed. Actual restic listing and owner-facing selection remain to integrate. CI37072361478 is queued; follow-up stays local. Full application acceptance remains unfinished.

### Snapshot selection native and boundary fixtures

Real-restic encrypted round trip now checks empty candidate inventory, pre-cancelled listing returning no candidates, exact published HomeNode snapshot identity/timestamp, and exclusion of a separately published untagged snapshot. Portable boundary fixture admits1000 distinct candidates and rejects1001 as a whole without partial results. Focused inventory race suite, Linux backup fixture compilation and diff checks passed. Native listing remains unexecuted for this local follow-up; CI37072361478 remains queued. No owner-facing selection or new-host restore activation is inferred. Full application acceptance remains unfinished.

### Selected snapshot compatibility preview

Added InspectSnapshot on retained authenticated repository with exact full snapshot ID, bounded2-minute dump/64KiB manifest and existing strict decoder plus independently supplied installed policy validation. It extracts no payload and exposes no source host paths; success certifies metadata compatibility only, not recoverability/start authority. Real encrypted fixture checks selected release/time/file-count preview, stricter catalog-floor refusal and cancellation. Linux backup fixture compilation and diff checks passed; native preview/listing remains unverified. Primary [restic manual](https://restic.readthedocs.io/en/stable/manual_rest.html?highlight=json) confirms JSON snapshot listing and [restore documentation](https://restic.readthedocs.io/en/stable/050_restore.html) documents exact snapshot selection/dump. CI37072361478 remains queued; follow-up stays local. Owner-facing workflow and full application acceptance remain unfinished.

### Stable redacted snapshot selector ordering

Candidate inventories now sort newest first with lexical full-ID tie break for equal instants. Ordering never chooses or approves restore automatically. Fixture uses equal instants with different timezone representations and verifies serialized results exclude private source paths/host names/tags. Focused inventory race tests and diff checks passed. Full backup/runtimeclient/state race suites passed for listing/preview changes before this ordering follow-up. Native restic listing/preview remains unverified; CI37072361478 remains queued and follow-up stays local. Owner-facing selection and full application acceptance remain unfinished.

### Recheck snapshot selector cancellation after parsing

Snapshot listing now rechecks its deadline after bounded parsing/sorting and returns no candidates if cancellation is observable before the final response. Existing pre-process and post-process checks remain. Linux backup fixture compilation and diff checks passed; deterministic cancellation-during-parse fixture has not been added. CI37072361478 was last observed queued; current GitHub read timed out at API connection, so no terminal/stopped state is inferred and no overlapping push occurred. Full application acceptance remains unfinished.

### Publish snapshot selection and preview prerequisites

Retried GitHub observation succeeded: CI37072361478 completed successfully at901dc24, including native ext4 encrypted recovery/corruption and backup loaded-resource/kernel/OOM fixtures. Publishing bounded tagged snapshot inventory, stable redacted ordering, metadata preview and cancellation gates with native selection fixtures. Local focused inventory tests, full backup/runtimeclient/state race suites before ordering, Linux backup fixture compilation and diff checks passed; native listing/preview still needs this new CI run. No owner-facing restore wizard or replacement-host activation is claimed. Full application acceptance remains unfinished.

### Distinguish selected metadata preview from recoverability

Real encrypted corruption fixture now requires compatible preview of the honestly checksummed corrupt disk without extracting any recovery files, followed by actual Restore filesystem rejection and cleanup. This explicitly separates compatibility metadata evidence from recoverability instead of letting a successful preview imply a successful restore test. Linux backup fixture compilation and diff checks passed; new native boundary fixture remains unexecuted. CI37072981616 remains in progress; follow-up stays local. Full owner-facing restore workflow and application acceptance remain unfinished.

### Native snapshot listing and preview evidence

CI37072981616 at published a0332b0 completed Test Go and Linux adapters successfully with real-restic/filesystem integration flags enabled. This covers empty/tag-filtered encrypted inventory, cancelled listing, selected compatible preview/catalog-floor refusal and the published recovery/OOM source prerequisites. The later compatible-preview-versus-corrupt-filesystem assertion at local22da2ff has not run natively. Overall CI remains active in systemd activation checks, so no overlapping push. Dedicated owner-facing metadata worker protocol, restore installation/re-enrollment and complete application acceptance remain unfinished.

### Bound snapshot selector pages for private packet transport

Added SnapshotPage operation with25 candidates, full-ID continuation cursor, explicit malformed/stale-cursor refusal, copied page values and final cancellation check. Inventory still uses authenticated bounded listing and newest-first ordering; pagination does not authorize recovery or provide a durable inventory view across repository changes. Fixture traverses52 candidates without omission/repetition and checks serialized page plus256-byte prospective envelope fits existing4KiB packet limit, including nanosecond/offset timestamps; empty inventory remains a non-null empty page. Focused page/inventory race tests, Linux backup fixture compilation and diff checks passed. Worker response/owner-facing integration remains required. CI37072981616 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Native repository page selection fixture

Real encrypted restic round trip now exercises first page, terminal full-ID cursor returning non-null empty page, missing/stale cursor refusal and cancellation through SnapshotPage. Linux backup fixture compilation and diff checks passed. This new native fixture remains unexecuted until publication; existing portable52-item traversal/wire-bound tests passed previously. CI37072981616 remains active; follow-up stays local. Dedicated metadata worker response authorization and complete owner-facing recovery remain unfinished. Full application acceptance remains unfinished.

### Request-bound snapshot page response schema

Published local page/native-preview boundary prerequisites through3c2f7ac after CI37072981616 succeeded; remote exact SHA verified and CI37073661410 is active. Added portable version/kind/request-bound page response codec limited to existing4KiB transport. It requires exact non-null outer/page/reference fields, unique JSON keys, canonical IDs, valid timestamps, sorted distinct candidates and continuation only from a full page last ID. Foreign request IDs, aliases, duplicate fields, wrong operation domain and trailing data refuse. Focused page/inventory/response race tests and diff checks passed. This codec is not wired to worker admission or owner HTTP/UI and supplies no peer authentication or recovery authority. Full application acceptance remains unfinished.

### Bound selector receive and verify actual full response envelope

Added private2-minute packet response receiver with valid request-ID qualification, cancellation checkpoints before receive/after receive/after decode and no-descriptor transport. Caller still must independently authenticate/authorize the configured worker. Actual25-entry continuation envelope with64-character request ID/nanosecond/offset timestamps round-trips below4KiB. Linux socket fixture covers exact request response, foreign request refusal and queued response under cancellation. Portable focused page/inventory/response race tests, Linux fixture compilation and diff checks passed; new native socket fixture has not executed. CI37073661410 remains active; follow-up stays local. Worker routing/admission and owner-facing selector remain unfinished. Full application acceptance remains unfinished.

### Snapshot selector request and credential transport

CI37073661410 completed successfully at3c2f7ac, including published native page selection and metadata-preview/corrupt-filesystem separation. Added exact version4 snapshot-page request schema containing only request/device IDs and full-ID cursor. Duplicate/aliased/null/extra fields, wrong operation versions and trailing data refuse; the maintenance launch/cleanup decoder does not admit it. Dedicated credential sender authenticates the configured root-created listener, and receiver reuses controller peer authentication plus sealed read-only descriptor qualification. The device remains a claimed identity requiring independent authorization before repository access. Native socket fixture covers transfer, foreign peer, maintenance-domain refusal and unprivileged listener creator refusal. Focused portable race tests, Linux backup test compilation and diff checks passed; new Linux socket fixtures remain unexecuted until CI. Worker routing, owner-facing selection and complete application acceptance remain unfinished.

### Controller snapshot page delivery bridge

Added send-and-wait operation that authenticates the installed listener before credential handoff and accepts only the request-bound page response. ActivatedBackupDispatcher now exposes SnapshotPage through its existing installed-path/group checks and returns no candidates on any delivery error. Portable controller fixture checks unqualified paths/groups, pre-cancellation, maintenance-domain requests and caller descriptor retention. Focused backup/runtimeclient race tests, Linux runtimeclient test compilation and diff checks passed. CI37074415371 at2fd969c remains in progress; this follow-up stays local until that run finishes. Independent owner/device authorization, worker routing, HTTP/UI and full application acceptance remain unfinished.

### Registered snapshot worker authorization boundaries

Added RunCredentialedSnapshotPage using only the installed registered target, authenticated pinned repository and existing bounded pagination. It consumes/closes the credential on every path, clears password copies, performs no runtime acquisition/staging/app mutations, and returns no page on repository or credential closure failure. A required SnapshotPageAuthority bridge must verify exact pending request plus current owner/session authority before credential reading/repository access and again after listing. The interface explicitly does not consider a device ID sufficient authorization. Portable tests prove authorization refusal precedes invalid credential qualification, descriptor closure, exact cursor forwarding, post-listing denial/cancellation and repository error returning no candidates. Focused runtimeclient race tests, Linux test compilation and diff checks passed. The concrete protected management authority endpoint, pending request/session binding, worker routing and HTTP/UI remain to implement; this function is not yet connected to the installed worker entry point. CI37074415371 remains active and follow-up stays local. Full application acceptance remains unfinished.

### Revalidate snapshot selector owner session

Added VerifyBackupSnapshotSession for controller-retained actors: requires current fresh admin access, exact session verification/expiry/epoch/capabilities, nonrevoked device, active session and claimed current identity. It supplies no runtime/restore authority and cannot derive an actor from a worker-provided device ID; exact pending request binding remains the caller's responsibility. Race fixture verifies valid session acceptance and logout, revocation, epoch change, capability change, verification change, expiration, idle timeout, unclaimed identity, stale verification and forged session refusal. Focused identity race test and diff checks passed. Pending controller request registry, protected management endpoint, installed worker routing and HTTP/UI remain unfinished. CI37074415371 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Bind pending selector requests to retained sessions

CI37074415371 completed successfully at2fd969c. Added controller-local pending snapshot request registry binding exact version/kind/request/device/cursor to a retained session, with copied capability slice, two-minute operation lifetime, caller release/cancellation withdrawal and four-request concurrency ceiling. Restart withdraws all entries; session hashes never enter worker packets. Verification rechecks current owner session and rechecks entry lifetime after database access. Tests cover exact binding, repeat verification, release/cancellation, logout/revocation, bounded concurrency, slot reuse and capability copy isolation. Full control/identity/runtimeclient race suites, Linux command builds and diff checks passed. Publishing accumulated selector worker/client/session prerequisites; new native credential/response fixtures still require this publication's CI. Protected management endpoint, installed worker routing, owner HTTP/UI and full application acceptance remain unfinished.

### Protected snapshot authorization endpoint

CI37075063601 completed successfully atf1993f7. Protected management handler now accepts a separate exact snapshot-page verification schema only from the configured kernel-authenticated backup peer, verifies the retained pending session/request under a five-second deadline and returns a no-store version acknowledgement. It does not require or create maintenance admission. Portable header-spoof refusal and pending request race tests passed; Linux socket fixture compiles and covers repeated exact verification, foreign cursor refusal, released request refusal and unchanged admission. Native execution awaits publication CI. Host default Git/Go became unusable after environment change; bundled Git and an isolated ARM Go1.26.8 archive verified against official download SHA256 restored local verification without accepting system license terms. Concrete worker management client, installed worker routing and owner HTTP/UI remain unfinished. Full application acceptance remains unfinished.

### Worker snapshot authority management client

Added a dedicated SnapshotPageAuthorityClient exposing only verification and close, reusing existing installed-path/listener-peer authentication and redirect refusal. It sends the exact selector schema, uses a five-second context deadline, accepts only strict bounded version acknowledgements and preserves cancellation. Portable race fixtures cover invalid configuration, cancellation and foreign operation domain. Extended native controller socket fixture to exercise the actual client, foreign listener creator refusal and withdrawn request refusal. Focused runtimeclient race tests, Linux control fixture compilation and diff checks passed; native endpoint/client execution remains for CI. Installed credential worker routing and owner HTTP/UI remain unfinished. Full application acceptance remains unfinished.

### Route metadata requests through installed credential worker

Added separate service request union retaining unchanged maintenance decoder and distinct snapshot schema/response slot. Installed registered worker now routes metadata through the concrete protected authority client and pinned registered repository listing; launch/cleanup callbacks stay separate. Shared single-operation admission remains held until successful callback completion, credential closure and response delivery. Metadata operation has a two-minute deadline and bounded request-correlated response; it acquires no root runtime authority. Portable union/race fixtures and focused backup/runtimeclient tests passed, Linux backup socket fixture compilation and Linux command builds passed. New native fixture checks actual credential callback routing, page response and received descriptor closed before reply, with caller descriptor retained; native execution remains for CI. CI37382005528 remains active and this follow-up stays local. Owner HTTP/UI and full application acceptance remain unfinished.

### Owner snapshot listing API

Added POST /api/v1/backups/snapshots with existing admin/origin boundary and dedicated credential media type. Installed controller configuration wires SnapshotPage to the authenticated dispatcher. Shared bounded binary credential envelope now accepts an operation-specific exact metadata validator while preserving backup approval validation. Selector metadata admits only exact registered repository ID and canonical/empty cursor; no credential enters metadata/URL/header. Handler requires fresh current owner session, registers a controller-generated request, creates a sealed password descriptor, dispatches and closes it before returning, then rechecks request/session and validates the response page. Portable envelope fixtures cover aliases, duplicates, null/foreign/path cursors, extra command fields and trailing data. Full control and command race suites, Linux command builds and diff checks passed. Successful Linux HTTP-to-worker execution and owner UI remain to verify/implement. CI37382005528 remains active and follow-up stays local. Full application acceptance remains unfinished.

### Native snapshot HTTP credential and withdrawal fixture

Added Linux handler fixture with controlled worker callback covering sealed credential qualification, exact cursor/controller-generated request/session binding, successful candidate response, revocation during dispatch, worker failure and malformed page refusal. It checks no secret reflection/caching, closure of controller credential before response, removal of pending worker authority after HTTP completion and unchanged maintenance admission. Linux control fixture compilation and diff checks passed. This fixture is not a real repository-to-browser test and has not executed natively yet. CI37382005528 remains active; follow-up stays local. Owner selector UI and full application acceptance remain unfinished.

### Owner snapshot browser interface

Added snapshot browser beneath available backup configuration with dates/full snapshot IDs, explicit metadata-only explanation, newest-page and older-page navigation. Each page requires credential reentry; input/byte/frame buffers are cleared and pending requests abort on unmount. Client uses dedicated same-origin no-store credential endpoint, validates bounded candidate identity/time/uniqueness and continuation before rendering. Synchronous pending guard prevents duplicate submission. TypeScript checks, production Vite build and diff checks passed before final pending guard; browser interaction and real drive-to-browser acceptance remain unverified. CI37382005528 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Snapshot browser interaction evidence

Extended real identity/passkey browser suite with controlled snapshot transport verifying UTF8 credential frame/media, no password URL, full25-candidate page, exact continuation cursor, terminal empty page and input clearing after each request. Snapshot input label now stays distinct from backup creation input. Rebuilt ARM backend after host toolchain change, then full Playwright identity suite passed (one scenario), TypeScript/production build and diff checks passed. Snapshot transport is mocked and does not prove installed drive-to-browser operation. Initial browser launch raced the backend rebuild and found the prior incompatible binary; retry after successful same build passed. Native HTTP/service fixtures and full application acceptance remain unfinished.

### Snapshot browsing instructions and narrow display layout

Documented reachable owner snapshot browsing, recent admin session requirement, password reentry/clearing, bounded pagination and stale cursor restart. Clarified that dates/IDs do not prove recoverability and backup-data restore is not exposed. Full snapshot IDs now use the existing wrapping checksum style so long identifiers fit narrow layouts. TypeScript, production build and diff checks passed; no new browser run was required for reuse of existing display styling. CI37382755401 remains in progress. Full application acceptance remains unfinished.

### Withdraw selector authority during controller shutdown

CloseBackupWork now closes snapshot request admission, cancels all pending operation contexts and removes retained authority before existing backup coordinator shutdown. Snapshot HTTP dispatch uses its registry-owned operation context, so request expiry/shutdown reaches socket work rather than only later authorization checks. Fixture proves dispatch context cancellation, request/context withdrawal and refusal of new selector admission after shutdown. Full control race suite, Linux control test compilation and diff checks passed. Cancellation does not independently prove isolated worker process termination; metadata worker acquires no runtime authority. CI37382755401 remains active. Full application acceptance remains unfinished.

### Redacted selected-snapshot compatibility preview

Added PreviewSnapshot over authenticated exact-ID manifest inspection, returning only selected ID/date/release/catalog and declared workload sizes with explicit metadata-compatible classification. It performs no extraction or payload/app certification and omits filenames, hashes and host details. Portable race fixture verifies declared summary, redaction, catalog-floor refusal and ambiguous selection refusal. Focused preview race test and diff checks passed. The preview is not yet exposed through isolated worker/owner UI; replacement-host restore and full application acceptance remain unfinished. CI37382755401 remains active.

### Exact selected-snapshot preview request

Added separate version5 snapshot-preview request requiring canonical full selected snapshot ID and request/device identity. It carries no cursor, paths, commands, runtime tokens or caller-defined compatibility policy, and remains excluded from both list and maintenance decoders. Strict codec rejects duplicate/aliased/null/extra fields, ambiguous/empty selection and trailing data. Focused preview race tests and diff checks passed. Authorization registry, protected management/client/service routing and owner preview UI remain to connect. CI37382755401 remains active. Full application acceptance remains unfinished.

### Request and selected-ID bound preview response

Added strict4KiB preview response codec correlating both request ID and selected snapshot ID. Exact outer/preview/file fields, canonical identities, valid release/catalog/date, bounded unique known workloads and management entry are required; only metadata-compatible classification is accepted. Duplicate/alias/extra-path fields, foreign request/selection, substituted restore-tested classification and trailing data refuse. Focused preview response/request race tests and diff checks passed. Peer authorization, transport routing and owner preview UI remain to connect. CI37382755401 remains active. Full application acceptance remains unfinished.

### Preview credential transport and controller delivery

Added dedicated authenticated credential preview send/receive and bounded response receiver correlating request and selected snapshot. Activated dispatcher exposes preview through existing installed socket/path/group qualification, returning no preview after transport errors. Linux response fixture covers own request/selection, foreign request, foreign selected snapshot and queued response cancellation. Focused portable backup/runtimeclient race tests, Linux backup fixture compilation and diff checks passed; native fixture execution awaits CI. Pending preview authorization, management endpoint/client, service routing and owner UI remain unfinished. CI37382755401 remains active. Full application acceptance remains unfinished.

### Pending selected-preview session binding

Added pending preview registration and verification in the shared controller registry with distinct preview domain fields, exact selected snapshot ID, retained current owner session, two-minute deadline, shared concurrency ceiling and shutdown cancellation. Listing checks cannot adopt preview entries. Focused controller race tests prove exact preview verification, foreign selection refusal, cross-domain listing refusal, canceled dispatch context and post-shutdown admission refusal. Diff checks passed. Concrete protected preview endpoint/client, worker routing and owner preview UI remain unfinished. CI37382755401 remains active. Full application acceptance remains unfinished.

### Protected preview authority endpoint and client

CI37382755401 completed successfully at9bb15e5, covering published native service and snapshot HTTP fixtures. Added distinct preview verification endpoint behind configured kernel backup peer authentication, exact preview decoder and retained session/selection verification. Dedicated preview authority client reuses installed listener authentication, five-second deadline and strict bounded acknowledgement. Native fixture compiles and checks exact repeated verification, foreign selection/listener and withdrawn request refusal through actual client. Full control/runtimeclient race suites, Linux control fixture compilation and diff checks passed. Preview worker service routing and owner UI remain unfinished. Full application acceptance remains unfinished.

### Registered selected-preview worker operation

Added credential-consuming preview worker function requiring exact pending authority before credential qualification/repository access and after selected manifest inspection. It uses only installed target/policy, clears temporary password, returns no preview on credential/repository closure failure and acquires no runtime/staging/app authority. Fixture checks exact selected-ID and installed catalog-floor forwarding, post-inspection withdrawal and pre-credential denial plus closure. Focused runtimeclient race tests, Linux runtimeclient compilation and diff checks passed. Installed service request routing and owner HTTP/UI remain unfinished. CI37383603183 remains active. Full application acceptance remains unfinished.

### Installed preview service routing

Credential service union now admits distinct preview request/response slots with optional explicitly configured callback. Registered installed worker supplies the concrete preview authority client and credential-consuming inspection operation. Domain guards prevent preview adoption by listing or maintenance handlers; exact selected ID and bounded preview response are validated before retaining a result, and shared completion sends only after callback and credential closure. Focused backup/runtimeclient race tests, separate service-union test, Linux command builds and diff checks passed. Native preview service response fixture and owner HTTP/UI remain unfinished. CI37383603183 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Owner selected compatibility preview API

Added POST /api/v1/backups/preview using fresh admin session, same-origin credential media, bounded binary envelope and exact registered repository/full selected snapshot metadata. Installed controller configuration wires preview dispatcher. Handler creates a pending preview, forwards its cancellable operation context and sealed credential, closes credential before response, rechecks owner/request authority and validates exact selected-ID metadata response. No restore or runtime action is granted. Exact envelope fixtures reject aliases/duplicates/null/paths/foreign repository/extra commands/trailing data. Full control/command race suites, Linux command builds and diff checks passed. Native preview service/HTTP success coverage and owner preview UI remain unfinished. CI37383603183 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Native selected-preview HTTP boundary fixture

Added Linux controlled-callback HTTP fixture covering exact selected snapshot/session binding, sealed credential qualification, metadata-compatible response, revocation during dispatch, worker failure and foreign selected-ID response refusal. It verifies no credential reflection/cache, controller credential closure, pending authority removal and unchanged maintenance admission. Linux control fixture compilation and diff checks passed; new fixture has not executed natively. It does not prove real repository-to-browser recovery. CI37383603183 remains active; follow-up stays local. Owner preview UI and full application acceptance remain unfinished.

### Owner selected compatibility preview interface

Snapshot browser now offers per-ID compatibility inspection with credential reentry, immediate input clearing, abort on unmount and shared duplicate-operation guard. Client verifies exact selected ID, metadata-compatible classification, release/catalog/date and bounded unique known workload sizes before rendering. UI shows release/catalog/declared bytes and explicitly distinguishes metadata compatibility from payload integrity/application recovery; no restore action is exposed. TypeScript, production build and diff checks passed. Initial typecheck command used repository root; corrected web-directory invocation passed. Browser interaction and real drive-to-browser preview remain unverified. CI37383603183 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Selected-preview browser verification

Extended real identity/passkey browser suite with controlled preview endpoint checking selected snapshot and credential frame, metadata-compatible display/explanation, credential clearing and foreign selected-ID response refusal with stale preview removed. Rebuilt current backend; full Playwright scenario, TypeScript and diff checks passed. Documented reachable compatibility inspection and distinction from extraction/integrity/app health. Preview transport remains mocked in browser test and real repository-to-browser acceptance remains unverified. CI37383603183 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Native credential service selected-preview fixture

Added Linux packet service fixture exercising actual sealed credential transfer, distinct preview callback routing (listing/maintenance callbacks refuse), selected-ID response correlation and duplicate descriptor closure before reply with caller credential retained. Linux backup fixture compilation and diff checks passed; native execution remains for CI. This uses controlled metadata callback rather than real repository inspection. CI37383603183 remains active; follow-up stays local. Full application acceptance remains unfinished.

### Encrypted restic compatibility summary fixture

CI37383603183 completed successfully at653f05a. Real encrypted restic round trip now exercises selected redacted preview summary identity/classification/release/declared workload sizes and incompatible catalog-floor refusal. Honestly checksummed corrupt ext4 snapshot must still summarize metadata-compatible before actual Restore rejects filesystem corruption and removes extraction output. Linux backup fixture compilation passed after fixing a local variable collision; new assertions remain unexecuted natively. Publishing accumulated preview routing/API/UI/native fixtures for CI. Browser preview transport tests passed with controlled responses; real repository-to-browser and full application acceptance remain unfinished.

### Encrypted preview wire round trip

Real restic fixture now encodes the selected redacted compatibility summary through the4KiB response codec and decodes it under exact request/selected-ID correlation, checking release/catalog/file-count preservation. Linux backup test compilation passed after replacing an undeclared fixture identity helper with a fixed valid request ID; diff checks passed. New wire assertion remains unexecuted natively. CI37384358035 remains active; follow-up stays local. Disconnected restored-disk installation, replacement-host policy reconstruction and full application acceptance remain unfinished.

### Qualified recovered-disk installation inventory

Added RecoveryInstallInventory requiring recovered payload/database authority validation plus ext4 qualification before producing disconnected data-disk inventory. It includes fixed staging filenames/declared sizes and approved image mappings from trusted policy, assigns fresh target instance identities and carries no runtime/UID/device authority. Caller must retain exclusive staging and journal inventory before installation effects; this is not a durable install authorization. Portable race fixture verifies honestly checksummed nonfilesystem refusal and cancellation returning no inventory; diff checks passed. Successful native inventory fixture, actual disk installation, management-state import/policy reconstruction and full application acceptance remain unfinished. CI37384358035 remains active; follow-up stays local.

### Restored encrypted disk inventory native fixture

Real restic recovery fixture now builds installation inventory from restored validated ext4 staging, checks fixed workload/source filename/size and installed approved image mapping, and requires distinct fresh instance IDs across repeated transient planning. Linux backup compilation passed after correcting the fixture to open private staging as os.Root. Native execution remains pending; no installed disk or runtime activation is claimed. CI37384358035 remains active; follow-up stays local. Actual installation/policy reconstruction and full application acceptance remain unfinished.

### Recovery payload identity and native preview verification

Recovered-disk inventory now retains the validated payload SHA256 separately from its approved boot-image SHA256 so later journaled publication can verify the recovered bytes. The real encrypted recovery fixture checks that mapping. Backup race tests and Linux fixture compilation passed. CI37384358035 completed successfully at4f86dbf, verifying the published native preview fixtures; the later encrypted preview wire and recovered inventory assertions are being published for native execution. Actual disconnected disk installation, fresh management-state import, policy reconstruction and replacement-host acceptance remain unfinished.

### Private recovered-disk copy primitive

Added a private disconnected recovery copy primitive for a future journaled installer. It requires exclusive private roots and fixed instance staging names, refuses occupied targets, bounds bytes, retains the 4GiB filesystem reserve during copying, verifies the source descriptor/name and copied SHA256, syncs file/directory and removes only its own staging inode on failure. Portable race tests cover byte preservation, occupied-target refusal, digest-mismatch cleanup and cancellation; Linux compilation passed. This primitive does not qualify a filesystem or grant installation/runtime authority. Durable journal/publication orchestration and fresh-host policy reconstruction remain unfinished. CI37385089713 is checking the previously published inventory changes; this follow-up remains local.

### Encrypted recovered filesystem copy fixture

CI37385089713 completed successfully at0e7f1c7, verifying the encrypted preview wire and qualified recovery inventory assertions natively. Extended the real encrypted restic recovery fixture to copy its recovered ext4 disk into separate private staging, verify copied payload identity, requalify the copied filesystem and refuse a repeated occupied-target copy. Linux compilation and diff checks passed; the new copy assertions require native CI execution. Publishing the copy primitive and fixture. This remains staging only: durable installation journal/publication, management-state import and replacement-host acceptance are unfinished.

### Immutable recovery installation plan creation

Added private journal creation for selected full snapshot ID and bounded unique workload/target identity inventory. Creation requires exclusive private storage, validates inventory and syncs the new file/directory before returning; occupied journals are refused and owned partial journals are removed on failure. Inventory now uses explicit JSON field names. Portable race tests verify retained target identities, occupied-journal preservation, duplicate refusal and cancellation. This is the initial immutable plan only; strict reopening/reconciliation, installation publication and fresh-host policy/state reconstruction remain unfinished. Full application acceptance is not achieved.

### Strict recovery plan reopening

Added bounded private regular-file journal reopening with descriptor/name identity checks, cancellation handling and zero result on failure. Decoder requires exact nonnull outer/nested fields, valid UTF8, unique JSON keys, full snapshot IDs and unique bounded disk mappings; alternate-case aliases, unknown fields, traversal source names and trailing objects are refused. Race tests verify retained disk identities after reopening, malformed/ambiguous input rejection, cancellation and insecure journal permissions. Reopening supplies recorded intent only; trusted source requalification, crash reconciliation, disk publication and replacement-host policy/bootstrap remain unfinished.

### Reopened plan source requalification

Added reopened-plan reconciliation against restored manifest disk count/names/sizes/payload hashes and currently approved image policy, followed by full recovered database/payload/ext4 qualification without generating new target IDs. Portable race tests reject honestly checksummed nonfilesystem data and cancellation. Real encrypted recovery fixture now journals/reopens target identities before copying, requalifies the source and refuses altered payload mapping. Linux compilation passed after adding the fixture strings import; native assertions remain for CI. This retains exclusive-staging ownership requirements and does not grant runtime authority. Publication, complete crash reconciliation, fresh-host management/policy/bootstrap and full application acceptance remain unfinished.

### Journaled disconnected disk publication primitive

Added private publication requiring an exact journaled disk entry, copied payload verification and ext4 qualification. It creates a non-overwriting hard link under the new instance identity and syncs the directory, retaining staging as inode proof; repeated publication accepts only that same regular inode and refuses an unrelated occupied final file. Portable race tests refuse unjournaled and honestly checksummed nonfilesystem staging without a final disk. Linux compilation passed; real encrypted fixture now exercises publication, same-inode reconciliation and foreign final-file refusal but awaits native execution. This private primitive grants no UID/runtime/device authority; full installed-layout ownership, orchestration/checkpoints, management import and fresh-host policy/bootstrap remain unfinished.

### Disconnected disk preparation orchestration

Joined qualification, immutable plan creation/reopening, current-policy requalification, bounded disk copying and same-inode publication into private preparation orchestration. It preserves journaled target identities on retry, rejects foreign selected snapshots, refuses occupied final names without staging inode proof and leaves incomplete/conflicting stages for explicit repair. Portable race tests show unqualified source refusal before journal/disk effects and cancellation. Linux compilation passed; encrypted native fixture covers both fresh preparation and recorded-identity retry but awaits CI execution. This prepares disconnected private files only; installed-layout ownership, complete partial-copy repair/checkpoints, management import, policy/bootstrap and full end-to-end application acceptance remain unfinished.

### Recovery source/destination separation

Preparation now rejects source and destination roots identifying the same directory before journal effects, retaining restored source separately from publication state. Focused race fixture verifies this refusal; the complete portable backup race suite passed before this added guard and the focused preparation test passed afterward. CI37388435556 remains in progress at6b2d67d; subsequent journal/orchestration changes stay local until it finishes. Full installed recovery and application acceptance remain unfinished.

### Full recovery manifest journal binding

Immutable plans now require SHA256 of the complete deterministic manifest encoding, including management payload identity and compatibility fields. Creation records this digest; reopening requires its exact field and requalification rejects any substituted manifest before source effects. Updated native encrypted fixture rejects a changed release with identical disk mappings. Focused recovery race tests, Linux compilation and diff checks passed. CI37388435556 completed successfully at6b2d67d, verifying the published private-copy fixture. Publishing the accumulated journal/requalification/publication/orchestration changes for native execution. Installed ownership, management import, fresh-host policy/bootstrap and full application acceptance remain unfinished.

### Recovery host-operation namespace exclusion

Management recovery review found sanitation and validation enumerated current host-operation setting keys. Changed snapshot export to remove the entire host.* namespace and recovery validation to refuse any retained host.* setting, avoiding future authority markers surviving through a missed individual key. Race fixtures verify an unknown live host marker is removed on export and a restored unknown marker is rejected. Snapshot race tests and diff checks passed. Metadata import/rebinding and full replacement-host acceptance remain unfinished.

### Transaction-capable recovery database validation

Separated already-open recovery database validation from immutable descriptor opening. The shared compiled-query validator accepts either a database or transaction, preserving schema identity checks before recovery data reads, so import can validate/rebind in one consistent transaction. Snapshot race tests passed; an added transaction fixture verifies app inventory and rejects authority inserted within that transaction, then rolls back. Its targeted race test passed after adding the required errors import. No metadata import or identity rebinding is implemented by this refactor; replacement-host acceptance remains unfinished.

### Transactional recovered management identity rebinding

Added RebindRecoveryTx for an exclusively owned disconnected recovery database. It validates schema/absence of restored authority inside the transaction, requires exact app coverage, distinct fresh disk identities, new owner identity and increasing recovery epoch, updates stopped app bindings/revisions and revalidates before returning. Exact identity/epoch/app retries preserve revisions; contradictory retries, reused old disk IDs and unchanged epochs are refused. Caller must journal intended identities before effects and roll back on error. Recovery rebinding and snapshot race tests passed. This supplies the management mutation primitive only; private copied-database import/publication, journaled owner/epoch intent, installed ownership and trusted bootstrap/policy remain unfinished.

### Validated source recovery identity metadata

Added ReadRecoveryMetadata returning historical owner/epoch/app inventory from the same immutable validated descriptor with bounded deadline, cancellation and zero metadata on failure. Existing snapshot validation delegates to this path. Rebinding fixture verifies the exported recovery epoch/app identity and cancellation without returned identity. Snapshot/rebinding race tests and targeted metadata race tests passed. These historical identities grant no authority; journaling replacement owner/epoch intent, management database import/publication and installed bootstrap remain unfinished.

### Journaled replacement management identity

Recovery plans now require a fresh owner identity and recovery epoch derived from validated source metadata before journal creation. Requalification requires a different owner and exactly the next source epoch, refusing epoch overflow; plan decoding requires exact new fields. Preparation records this intent alongside disk identities, preserving it on reopening/retry. Portable recovery race tests verify construction and durable identity/epoch retention; Linux compilation passed. CI37389167610 completed successfully at9a1ca4d, verifying journal/disk orchestration natively before these management additions. Publishing accumulated management validation/rebinding/journal changes. Private database import/publication, installed ownership, trusted bootstrap/policy and complete application acceptance remain unfinished.

### Private sanitized management database copying

Extracted bounded verified payload copying shared by data disks and a new private management-copy operation. Management copy requires loaded journal intent and full current-policy/source requalification, caps its fixed snapshot.db payload at256MiB, refuses occupied staging, preserves reserve/durable bytes and validates the copied sanitized database. Portable recovery race tests passed; encrypted native fixture now checks byte-identical separate management staging and occupied-stage refusal, with Linux compilation passed and native execution pending. Identity rebinding/publication is still separate; full installed import/bootstrap/policy and application acceptance remain unfinished.

### Linux private management-stage rebinding

Added Linux management-stage rebinding under a30-second deadline using retained directory-descriptor addressing, exclusive ownership, exact original payload verification and validated historical metadata. It applies journaled owner/epoch/disk intent through the validated transaction, closes SQLite and syncs the private file/directory with inode checks. Encrypted native fixture verifies resulting owner/epoch/app identity and refuses treating the changed database as the original payload on a repeated initial rebind. Linux compilation and diff checks passed; native SQLite path/journal behavior and assertions remain unexecuted pending CI. Changed database output tracking/crash reconciliation and publication are still unfinished, as are installed ownership/bootstrap/policy and full acceptance.

### Rebound management output identity inspection

Added private rebound-stage inspection requiring exact journaled owner/epoch/app identities, validated sanitized schema/authority and private inode/size checks before context-aware hashing. It returns resulting database bytes/SHA256 and full plan digest for later durable output recording, with zero result on failure. Portable race fixture refuses the original historical database, verifies changed rebound output and cancellation; Linux compilation passed with encrypted fixture output checks awaiting native execution. Returned output is transient, not a durable receipt. Durable output recording, crash reconciliation/publication and full installed recovery remain unfinished.

### Durable management output receipt creation

Added immutable management-output receipt creation from newly validated rebound bytes, sharing bounded private journal creation/sync/failure cleanup with the installation plan. Receipt codec requires exact nonnull fields, unique JSON keys, valid digest/size bounds and version. Portable race tests verify persisted output equality, occupied-receipt refusal and ambiguous/trailing JSON rejection; existing plan tests passed after journal helper extraction. Receipt reopening/digest reconciliation and database publication remain unfinished, along with installed bootstrap/policy and full application acceptance.

### Management receipt reopening and output reconciliation

Added bounded private regular-file receipt reopening with inode/permission/context checks and fresh rebound output inspection. Recorded full plan digest, bytes and checksum must all match; no changed output is adopted. Portable race tests verify valid reconciliation, cancellation and refusal after an otherwise valid database setting mutation changes bytes. Linux compilation passed and real encrypted fixture now records/reconciles output receipts but awaits native execution. CI37389817175 completed successfully at4f18b2c, verifying prior identity-journal/rebinding state primitives; publishing accumulated management-copy/Linux-rebind/output/receipt changes. Database publication, complete interrupted-import orchestration, installed ownership/bootstrap/policy and full acceptance remain unfinished.

### Private verified management database publication

Added private management.db publication only after receipt/output reconciliation. It creates a non-overwriting hard link, retains stage inode proof, syncs/checks publication and accepts repeated publication only for that same inode. Portable race fixture verifies publication, same-inode retry and unrelated occupied database refusal; Linux compilation passed with encrypted native publication assertion pending execution. This database remains disconnected and must not be opened by a live controller before installed ownership, disk policy and trusted bootstrap reconstruction. Complete import interruption/repair orchestration and full application acceptance remain unfinished.

### Joined disconnected disk and management preparation

Added Linux orchestration joining qualified journaled disks, verified management copying, transactional identity rebinding, immutable output receipt and private publication. It verifies existing receipts on retry, resumes an original complete copy or exactly journaled committed rebound identities, and refuses contradictory/uncertain partial state or an occupied database without receipt. Real encrypted native fixture covers fresh joined preparation and repeated receipt-preserving retry; Linux compilation and diff checks passed, native execution pending. Hot-journal/partial-copy repair, installed ownership/bootstrap/policy and complete application acceptance remain unfinished; this operation activates no controller or workloads.

### Management completed-boundary retry fixtures

Extended encrypted native recovery fixture with separate private destinations paused after completed management copy and after committed rebinding before receipt. Orchestration must resume/publish/reconcile without changing immutable journal identities. An incomplete management-copy fixture must fail, preserve its partial bytes and publish neither database nor receipt. Linux compilation and diff checks passed; native assertions remain pending publication/execution. These emulate completed-operation boundaries, not process-kill/power-loss/hot-journal recovery. Explicit partial repair and full installed recovery/acceptance remain unfinished.

### Uncertain management-stage quarantine primitive

Added private repair quarantine preserving uncertain stage bytes via a durable hard link before removing the active stage name. It requires matching journal/manifest intent, refuses completed original/rebound databases, output receipts/publications and SQLite journal/WAL/shared-memory sidecars, and never overwrites an unrelated quarantine inode. Unexpected inspection failures/cancellation do not trigger quarantine. Portable race tests verify preserved partial bytes, sidecar refusal and completed-copy refusal. This is a repair primitive for explicit guided workflow selection, not automatic repair or a claim of hot-journal recovery. Native repair orchestration, installed bootstrap/policy and full acceptance remain unfinished.

### Frontend dependency advisory remediation

CI37418019043 failed at the frontend vulnerability gate for source-map-js1.2.1 (GHSA-68fv-2mgg-jv7q), rather than establishing a successful complete run. Updated only that transitive lockfile entry to1.2.2, the patched release identified by the GitHub advisory. npm audit now reports zero vulnerabilities; TypeScript, production build, six dependency-evidence tests and diff checks passed. Publishing the remediation with accumulated recovery publication/orchestration/quarantine changes for another native CI run. Full installed recovery and application acceptance remain unfinished.

### Restart after preserved management quarantine fixture

Encrypted native partial-copy fixture now selects quarantine explicitly, reruns joined disk/management preparation from validated restored source, reconciles the repaired output receipt and verifies quarantined uncertain bytes remain intact. Immutable journal identities must remain unchanged through refusal/quarantine/restart. Linux compilation and diff checks passed; the new assertion remains unexecuted natively. CI37418510830 is active at ecda02d. Guided owner repair UI, hot-journal/power-loss recovery, installed bootstrap/policy and complete acceptance remain unfinished.

### Exclusive private recovery preparation entry point

Added Linux PreparePrivateRecovery retaining persistent kernel maintenance leases on both private runner-owned source/destination roots through preparation and publication. Nonblocking overlap or same-root use is refused; destination refusal releases the already-acquired source lease, and close errors clear returned names. Unsupported hosts refuse preparation. Portable race tests verify overlap exclusion, destination refusal/source release and subsequent acquisition; Linux compilation passed and encrypted fresh/retry fixtures now call the leased entry point. Leases coordinate participating HomeNode writers, not independent privileged writers. Installed ownership/bootstrap/policy, guided owner workflow and full acceptance remain unfinished.

### Encrypted extraction through private recovery preparation

Added Linux RestoreAndPreparePrivateRecovery joining exact encrypted Repository.Restore with journaled disconnected disk/management preparation under a two-hour deadline. Empty extraction uses the retained JobStaging parent lease; destination lease spans extraction through publication, and source maintenance lease is acquired after extraction. Preparation failure retains validated source for explicit leased retry; no live controller activates. Real encrypted fixture now exercises the joined operation and prepared-source retry with output reconciliation. Linux compilation and diff checks passed; native execution pending. Caller must retain JobStaging for the operation. Installed ownership/bootstrap/policy, guided recovery and full acceptance remain unfinished.

### Recovery staging operation lifetime guard

JobStaging now serializes owned operations with Close, preventing concurrent closure from dropping the parent lease/root during joined recovery. A competing operation refuses immediately and closed staging refuses future operations; the restore/preparation entry point retains this guard through handle cleanup. Race fixture verifies retained root access/parent overlap exclusion while Close waits, release afterward and closed-object refusal. Full portable backup race suite, Linux compilation and diff checks passed. Root() callers still must retain ordinary staging lifetime; installed recovery/bootstrap/policy and complete acceptance remain unfinished.

### Installer-provisioned private recovery destination parent

Maintenance configuration now provisions a separate var/lib/homenode-backup/recovery parent with0700 and the observed distinct maintenance UID/GID. Plan admission and journal validation apply the same strict private system-account bounds as backup staging and reject duplicate/foreign recovery ownership. Existing journals without the newly generated path remain readable; they do not gain a recovery directory implicitly. Installer race suite and diff checks passed, including generated ownership/admission fixtures. This provisions private preparation storage only; recovery service authority, installed data handoff/bootstrap/policy and full application acceptance remain unfinished.

### Verified prepared recovery handoff inventory

Added InspectPreparedRecovery rechecking full source manifest/current trusted image policy, disk mapping/checksum/ext4 integrity, publication-stage inode identity and rebound management receipt/identity before exposing bounded handoff inventory. It returns no UID/runtime/client activation authority and requires retained exclusive prepared-root ownership through later handoff. Portable race fixture refuses management-only state without prepared disks; Linux compilation passed with encrypted valid-inventory and raised catalog-floor refusal assertions pending native execution. CI37418510830 completed successfully at ecda02d, verifying prior recovery publication/orchestration/quarantine and advisory remediation. Publishing accumulated leased extraction/preparation, storage provisioning and inventory changes. Installed handoff/bootstrap/policy and full acceptance remain unfinished.

### Pinned prepared recovery inspection lease

Added OpenPreparedRecovery for installer inspection of private maintenance-owned output. It verifies an expected UID supplied by trusted installed account configuration, requires the existing private single-link preparation lock without creating one, retains nonblocking exclusion and independently pins the directory. Inventory serializes inspection with Close and rechecks current manifest/policy through prepared-output qualification; closed or overlapping operations refuse. Portable backup race suite passed, including missing-lock/no-mutation, owner mismatch, preparation overlap, independent root lifetime, inspection overlap and release assertions. Linux compilation passed; the encrypted restore fixture now exercises leased inventory and trusted catalog-floor refusal, with native execution pending. CI37419382760 remains active at93db3c3; this change is not yet pushed to avoid interrupting that run. Installed data transfer/bootstrap/policy, guided recovery and full acceptance remain unfinished.

### Replacement configuration joined to prepared recovery

Added RecoveryConfigurationPlan joining replacement-host ConfigurationPlan with leased prepared-output inspection. It derives approved files/AI image hashes from the independently verified replacement catalog, preserves its current minimum catalog floor and requires recovered disk sizes to match current catalog data sizes. InventoryForOwner binds the retained lease to the replacement maintenance UID rather than accepting caller-opened ownership implicitly. Preview retains installer pending gates and adds journaled ownership transfer, stopped runtime reconstruction/fresh UID leases, new owner reenrollment and workload verification before activation. Full installer/backup race suites, Linux installer compilation and diff checks passed. Refusal tests cover incompatible sizes, raised publisher catalog floor, absent prepared state/foreign owner and cancellation; they do not establish successful installed restore. CI37419382760 completed successfully at93db3c3. Installed data handoff/bootstrap/policy, guided recovery and complete application acceptance remain unfinished.

### Scoped prepared recovery descriptor handoff

Added PreparedRecoveryLease.WithFiles retaining operation exclusion, pinned root and maintenance lock through full current-policy inventory inspection, read-only descriptor opening, consumer callback and joined descriptor cleanup. Each published file is checked against its preparation-stage inode, exact size and private mode before opening; callbacks receive verified inventory and borrowed bounded file descriptors. Expected maintenance UID remains independently supplied. Callback must journal destination intent before effects and must not close/reenter the same lease. Portable backup race suite and focused refusal tests passed; Linux compilation passed with encrypted native fixture assertions for read-only files, overlap exclusion and descriptor closure pending CI execution. This provides source lifetime for installer copying; destination ownership journal/copy/bootstrap/runtime reconstruction and full acceptance remain unfinished.

### Immutable installer recovery intent

Added internal prepareRecoveryIntent requiring installed replacement configuration, exact independently reconstructed configuration digest and current owned-path matches before joined trusted-catalog/leased source qualification. It records bounded canonical inventory with configuration ID/digest in private recovery.json before future transfer effects. Exact byte-for-byte retry preserves the committed inode and resyncs the journal directory; conflicting, ambiguous or partial existing bytes are preserved/refused. Uncertain new writes remain blocking rather than being silently adopted. Installer race suite passed with exact retry, foreign intent, torn-byte preservation and cancellation tests; Linux compilation and diff checks passed. Production account/capacity observation, destination copy progress/ownership, bootstrap/runtime reconstruction and full restore/application acceptance remain unfinished. CI37419978389 remains active at e94abe5, so accumulated changes remain local pending that run.

### Bounded disconnected recovery copy primitive

Added installer copyRecoveryFile for borrowed read-only prepared descriptors into private installer-owned staging under committed intent/retained WithFiles prerequisites. It allows only fixed management or validated fresh-instance copy-stage names, requires exact source size and SHA256, preserves descriptor offset via section reads, checks cancellation and filesystem remaining-copy capacity plus4GiB reserve at every write, and syncs file/directory on success. Occupied stage names refuse; uncertain partial/checksum-failed bytes remain for explicit repair, with no live publication or ownership assignment. Installer race suite, Linux compilation and diff checks passed; fixtures verify exact bytes, unchanged borrowed offset, occupied-stage refusal, checksum failure preservation and path refusal. Integration with installer copy progress/publication, ownership/bootstrap/runtime reconstruction and full application acceptance remain unfinished. CI37419978389 remains active, so this change stays local without cancelling its run.

### Installer recovery copy staging orchestration

Added verifyRecoveryCopy rehashing private owner/mode/size/inode-bound staging and internal stageRecoveryCopies joining installed configuration checks, immutable intent, current trusted catalog and retained WithFiles scope under installer exclusion. Stages stay in the private installer journal directory under installer ownership. Exact completed bytes are reverified on retry; only absent stages are copied, while contradictory/partial stages refuse. Inventory must remain byte-identical to committed intent qualification. Installer race suite, Linux compilation and diff checks passed; copy fixtures add completed-copy verification and permissive/checksum-invalid refusal. Successful joined installer staging has not yet been exercised end-to-end natively; live publication/ownership/bootstrap/runtime reconstruction and full acceptance remain unfinished. CI37419978389 remains active and accumulated changes remain local.

### Durable recovery copy retry and configuration refusal fixtures

Completed staging verification now syncs the verified file and destination directory before accepting retry, covering completed-copy bytes with lost acknowledgement before the original durability boundary. Added joined staging refusal fixture ensuring absent installed configuration produces neither intent nor copy bytes and cancellation is preserved. Added temporary root-only fixture refusing changed runtime policy against installed configuration before recovery intent is written; existing CI root installer step includes it. Installer race suite, Linux compilation and diff checks passed; native root assertion remains pending publication/execution. CI37419978389 remains confirmed live at browser installation. Successful joined restore staging/publication/ownership/bootstrap/runtime reconstruction and full application acceptance remain unfinished.

### Durable immutable intent retry and accumulated native publication

Exact recovery-intent retry now syncs the verified recovery.json file before directory sync, covering initial completed bytes interrupted before file durability. Installer race suite, Linux compilation and diff checks passed. CI37419978389 completed successfully at e94abe5, verifying the prior prepared lease/replacement configuration changes. Publishing accumulated scoped descriptor handoff, immutable intent, bounded copy/verification and staging/refusal changes for native execution. Successful full installed restore/publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished.

### Recovery copy cancellation and occupied-path regression evidence

Extended installer copy/verification fixture to prove pre-cancelled copy creates no stage, occupied symlink is refused by verification and never replaced by copying, its source target remains unchanged, and partial staging bytes remain intact after refusal. Installer race suite, Linux compilation and diff checks passed. CI37420643271 is still live at38f4529; this regression change remains local. Successful joined installed recovery, publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Durable disconnected staging completion receipt

Installer staging now writes immutable recovery-staged.json only after all source-scoped copies verify/sync and borrowed descriptor cleanup succeeds. Receipt binds version, exact canonical configuration/recovery intent SHA256 and expected management-plus-disk count. Shared immutable record writer accepts only the intent/completion fixed filenames, preserves conflicting bytes, and requires exact durable retry. Installer race suite, Linux compilation and diff checks passed; receipt regression tests cover exact retry, foreign receipt refusal and journal path allowlist. Successful full joined native restore remains unverified; live publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished. CI37420643271 remains active, so this change stays local.

### Joined native management staging replay fixture

Added recovery-intent and recovery-copy checkpoints under retained installer/source exclusion for interrupted-boundary testing. New Linux temporary-root fixture builds a sanitized management-only snapshot, prepares/rebinds it, assigns private prepared output to the configured maintenance UID, interrupts after verified installer copy before completion receipt, reopens installer and requires unchanged copy inode plus successful receipt on retry. Existing Linux root installer CI step will execute it; Linux compilation, portable installer race suite and diff checks passed, but the new native fixture is unexecuted until publication. This covers empty application state and injected acknowledgement loss, not app disk recovery, process kill or power failure. Full publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished. CI37420643271 remains live, so changes remain local.

### Intent-before-effects replay boundary fixture

Extended Linux joined management staging fixture to interrupt immediately after immutable intent, require no copied stage/completion receipt, reopen installer and proceed through the separate copy interruption/reopen. Final replay must preserve exact original intent bytes and completed copy inode. Linux compilation and diff checks passed; the native assertion remains pending publication/execution. CI37420643271 is confirmed live in Go/Linux adapter testing. Successful live installed restore/publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished.

### Current-byte staging reconciliation gate

Added reconcileRecoveryStaging binding expected independently qualified inventory to installed configuration ID/digest, exact immutable intent and completion receipt, then rehashing/syncing every current copy. matchRecoveryRecord reads only the two fixed private record names and refuses absent, permissive, replaced or contradictory records without creating them. Joined staging calls reconciliation before returning success. Portable installer race suite, Linux compilation and diff checks passed; matching/foreign record tests passed. Linux joined fixture now requires altered management copy to fail reconciliation despite a previously completed receipt; native execution remains pending publication. CI37420643271 remains live. Full installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Reconciliation against current owned host configuration

Staging reconciliation now verifies every installed ownership record against current host files/directories before accepting intent, receipt or copied data. Configuration ID/digest alone does not establish unchanged runtime/network/service policy. Native joined fixture changes installed runtime-policy bytes after staging and requires refusal, restores exact bytes and requires reconciliation again before the existing corrupted-copy refusal. Portable installer race suite, Linux compilation and diff checks passed; native fixture execution remains pending publication. CI37420643271 remains live; changes stay local. Successful live installed restore/publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished.

### Missing completion record is never synthesized by reconciliation

Portable immutable-record regression now removes staging receipt and requires read-only matching to refuse with not-exist while leaving the record absent. Linux joined fixture likewise removes a completed receipt, requires staging reconciliation refusal/no recreation, and uses the explicit record writer to restore test state for later corruption checks. Installer race suite, Linux compilation and diff checks passed; native joined assertion remains pending publication. CI37420643271 remains live. Successful publication/ownership/bootstrap/runtime reconstruction and full application acceptance remain unfinished.

### Whole installer recovery staging deadline and native publication

Joined installer staging now shares one two-hour deadline across configuration/intent qualification, scoped source inspection/copy, descriptor cleanup, completion receipt and final current-byte reconciliation. Repeated phases no longer receive independently reset full deadlines. Installer race suite, Linux compilation and diff checks passed. CI37420643271 completed successfully at38f4529, verifying earlier scoped handoff/intent/copy staging changes; publishing accumulated completion receipt, current configuration/byte reconciliation and joined interruption fixtures for native execution. Full installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Strict recovery service dormancy observation parser

Added bounded UTF8 exact-property parser requiring loaded/inactive/dead service state and zero main/control process IDs. It refuses missing, duplicate, unknown, oversized, carriage-return ambiguous, active/stopping or live-process observations. Installer race suite and diff checks passed. This is an observation parser only: native manager querying, activation exclusion, guest/cgroup reconciliation and retained publication barrier remain unfinished; it does not grant ownership/publication authority. CI37421253738 remains active. Full installed recovery/bootstrap/runtime reconstruction and application acceptance remain unfinished.

### Bounded native recovery service observations

Added root-only Linux ObserveRecoveryServicesDormant querying fixed control/transfer/supervisor/backup units through absolute systemctl path, restricted environment, five-second per-query/twenty-second overall deadlines, bounded wait and1KiB output. It applies exact dormancy parsing and stops at first failure; unsupported hosts refuse. Linux command fixture verifies fixed arguments/service set, unsafe-observation short circuit and output cap; fixture execution remains pending Linux CI. Portable installer race suite, Linux compilation and diff checks passed. This read-only observation API does not mask activation or establish guest/cgroup emptiness/publication authority. Retained activation barrier, installed publication/ownership/bootstrap/runtime reconstruction and full application acceptance remain unfinished.

### Dormancy bound to installed manager unit identity

Native recovery queries now require exact fixed unit ID, /etc/systemd/system fragment, empty drop-in list, no pending daemon reload and non-transient state in addition to loaded/inactive/dead/zero-process observations. Parser refuses foreign IDs/paths/overrides and arbitrary unit selection. Command fixture includes these identity fields and fixed property argument. Installer race suite, Linux compilation and diff checks passed; Linux command execution remains pending publication. This still observes state only: activation exclusion, effective unit/cgroup enforcement and installed publication/bootstrap/runtime reconstruction remain unfinished. CI37421253738 remains live; full application acceptance is not complete.

### Recovery manager observation cancellation/failure boundaries

Native observation loop now checks its shared deadline before constructing every fixed-unit query. Linux command fixture requires pre-cancelled observation to launch zero queries, command failure to short-circuit after one, and missing adapter refusal. Linux compilation and diff checks passed; these Linux assertions remain pending publication/execution. Retained activation barrier, cgroup reconciliation, installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Persistent recovery activation marker and installed unit conditions

Installer staging now durably creates private owner-bound zero-byte recovery-blocked after immutable intent and before copies. Reviewed installed control/transfer/supervisor/backup service and credential-socket templates require this fixed marker to be absent before activation. Marker survives exact retry, refuses foreign/malformed occupants and is never automatically released; tests cover cancellation/no creation, retained inode and condition coverage. Installer race suite, focused marker/template tests, Linux compilation and diff checks passed. Native manager enforcement/reload and queued/running service/cgroup reconciliation still need execution and retained handoff checks: marker alone does not stop already-running units or prove safe publication. Bootstrap/acceptance-gated release, installed publication/ownership/runtime reconstruction and complete application acceptance remain unfinished. CI37421253738 remains active for earlier work.

### Reconciliation requires retained activation marker

Split activation-marker creation from existing-marker validation. Current staging reconciliation now requires the existing private owner-bound zero-byte marker and refuses absence without synthesizing it. Portable regression verifies existing marker admission, absent marker refusal/no recreation and explicit recreation through the blocking operation. Installer race suite, Linux compilation and diff checks passed. CI37421253738 completed successfully atad80d13, verifying joined native management staging intent/copy replay and current configuration/receipt/copy refusal fixtures. Publishing accumulated bounded service dormancy/identity queries, unit activation conditions and marker admission for native checks. Live manager condition enforcement/queued-running guest exclusion, installed publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished.

### Joined staging refusal after activation-marker loss

Linux joined replay fixture now removes persistent recovery-blocked after successful staging, requires reconciliation to refuse with not-exist and leave the marker absent, then restores it only through the explicit blocking operation before remaining policy/receipt/copy corruption assertions. Linux compilation and diff checks passed; native execution remains pending publication. CI37421831478 is confirmed live at19b5315. Live manager enforcement/quiescence, installed publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished.

### Installer-owned recovery service observation preflight

Added root/system-host ObserveRecoveryQuiescence joining installed ownership-record checks, existing durable activation marker, exact reviewed blocking condition in all five owned service/socket files and bounded loaded-unit dormancy observation. Marker is revalidated after manager queries without creation. Portable refusal fixture ensures uninstalled, missing conditioned-unit configuration and cancelled preflight never reach manager observation. Installer race suite, focused refusal test, Linux compilation and diff checks passed. Successful native manager preflight and queued-job/guest-cgroup emptiness still need verification; this observation does not authorize publication by itself. Installed publication/ownership/bootstrap/runtime reconstruction and full application acceptance remain unfinished.

### Credential activation socket included in dormancy observation

Recovery native observation now queries the fixed backup credential socket as well as four services, requiring owned loaded identity and inactive/dead socket state. Socket query uses common unit properties rather than service-specific main/control process fields; services retain zero-process checks. Shared maximum rises to25seconds for five bounded queries. Portable parser test accepts inactive socket and refuses listening state; Linux command fixture verifies all five fixed units/property sets. Installer race suite, focused socket test, Linux compilation and diff checks passed; Linux command fixture execution remains pending publication. Queued activation/cgroup reconciliation, live condition enforcement, installed publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished.

### Activation marker hard-link alias refusal

Persistent recovery activation-marker validation now checks descriptor link count equals one, preventing acceptance/adoption of an aliased marker inode. Portable regression creates a hard-link alias and requires both validation and blocking retry refusal, removes alias and requires single-link admission, then broadens mode and requires refusal. Installer race suite, Linux compilation and diff checks passed. CI37421831478 remains live. Native manager/cgroup/queued-job enforcement, installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Systemd repeatable-condition fixture parsing repair

CI37421831478 failed in packet socket activation fixture because generic strict INI parsing rejected valid repeated ConditionPathExists directives in the updated backup unit. Both activation/OOM resource fixtures now parse only the unique Service section, retaining strict duplicate resource refusal without treating repeatable Unit conditions as singleton INI options. Local current-source resource parsing, duplicate Service-resource rejection, Python syntax and diff checks passed. Native activation/OOM execution still requires another CI run; no complete-success claim for failed run. Publishing accumulated marker-loss/preflight/socket/alias changes with fixture repair. Full installed publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished.

### Activation marker inode pinned through manager observation

Recovery preflight now opens/pins the activation marker through owned configuration and manager observation, joins descriptor close errors and requires final path identity to match the retained inode. Replacing a valid marker during queries can no longer pass solely through shape validation or inode reuse after unlink. Temporary root fixture constructs full backup configuration, accepts unchanged injected observation and refuses marker removal/recreation during observation. Installer race suite, Linux compilation and diff checks passed; root fixture execution remains pending native CI. This does not establish live manager/cgroup/queued-job exclusion or publication readiness. Installed publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished.

### Whole recovery observation preflight deadline and overlap refusal

Recovery observation preflight now carries one thirty-second cooperative deadline through bounded configuration-record checks, manager query and final marker recheck. Cancellation is checked before each configuration/unit read; native manager commands retain their shorter deadlines. Public production entry refuses immediately if installer exclusion is occupied rather than waiting behind long staging work. Installer race suite, Linux compilation and diff checks passed. Filesystem syscalls themselves are not made preemptible by context. Native manager/cgroup/queued-job enforcement, installed publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished. CI37422229911 remains active.

### Host configuration drift during recovery observation refusal

Recovery preflight now reloads installed journal after manager observation and requires unchanged ID/digest/installed phase, then revalidates every original owned record under the shared deadline before final marker check. Temporary root fixture mutates runtime-policy bytes during injected observation and requires refusal, restores them and proceeds to existing marker replacement refusal. Installer race suite, Linux compilation and diff checks passed; native root assertion remains pending publication/execution. CI37422229911 remains live. Live manager/cgroup/queued-job exclusion, installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Manager observation failure preserves activation exclusion fixture

Temporary root preflight fixture now injects unavailable manager observation, requires the original error to propagate and verifies the persistent activation marker inode remains unchanged. Linux compilation and diff checks passed; native assertion remains pending publication/execution. CI37422229911 remains confirmed live at3d5415f. Live manager/cgroup/queued-job exclusion, installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Unit-file drift during manager observation fixture

Temporary root recovery preflight fixture now replaces owned control unit bytes during injected observation and requires refusal, restores exact owned bytes, then proceeds to marker replacement refusal. This exercises the final owned-record check on activation-critical unit source rather than runtime policy only. Linux compilation and diff checks passed; native execution remains pending publication. CI37422229911 remains live. Live manager/cgroup/queued-job exclusion, installed publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished.

### Installation journal identity drift fixture

Temporary root preflight fixture now changes the valid installation journal ID during injected manager observation and requires refusal despite unchanged owned file bytes. It restores original journal bytes before the marker replacement case. Linux compilation and diff checks passed; native execution remains pending publication. CI37422229911 remains live. Live manager/cgroup/queued-job exclusion, installed publication/ownership/bootstrap/runtime reconstruction and complete acceptance remain unfinished.

### Fixed kernel guest-cgroup emptiness observation

Added root-only Linux ObserveRecoveryGuestsEmpty reading bounded cgroup.events beneath fixed homenode.slice, checking actual cgroup-v2 filesystem magic and a regular no-follow descriptor. Strict parser requires populated0/frozen0 and refuses unknown/duplicate/incomplete/malformed state. Production preflight now observes services/socket first then kernel guest hierarchy before final owned configuration/marker rechecks. Missing hierarchy and unsupported hosts refuse. Installer race suite, Linux compilation and diff checks passed; actual kernel observation remains unexecuted natively and needs disposable fixture coverage. An empty snapshot does not exclude queued/future activation or establish retained publication authority. Installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Kernel guest hierarchy identity retained through observation

Native guest-cgroup observation now refuses symlink/non-directory fixed slice paths, compares pre-open path identity with retained directory descriptor and rechecks current fixed-path identity after reading events. A replaced hierarchy cannot supply stale empty evidence from an older pinned root. Linux compilation and diff checks passed; actual kernel fixture remains pending implementation/execution. CI37422229911 remains live. Queued/future activation exclusion, installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Accumulated recovery preflight native validation publication

CI37422229911 completed successfully at3d5415f, verifying the repaired repeatable-condition activation/OOM fixtures and preceding activation-marker/preflight changes. Revalidated current installer race suite, Linux compilation and diff checks before publishing accumulated marker pinning, whole-preflight deadline, manager failure/configuration/unit/journal drift fixtures and fixed guest-cgroup observation/identity checks. Cached portable installer results are reused by Go for unchanged inputs; native newly added fixtures still require this publication run. Actual kernel empty-hierarchy fixture, queued/future activation exclusion, installed publication/ownership/bootstrap/runtime reconstruction and full application acceptance remain unfinished.

### Kernel cgroup events descriptor/path identity check

Guest-cgroup observation now rechecks cgroup.events as a regular current path matching the retained opened descriptor after parsing, in addition to fixed hierarchy identity and cgroup-v2 filesystem checks. Replaced events-file evidence is refused. Linux compilation and diff checks passed; actual kernel fixture remains pending implementation/execution. CI37479528801 remains live at0f525a4. Queued/future activation exclusion, installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Disposable actual kernel guest-cgroup fixture

Added explicit disposable Linux root fixture guarding fixed unit/hierarchy vacancy, creating a temporary bounded homenode.slice, observing empty state, starting a harmless sleeping descendant and requiring populated refusal, then stopping it and requiring empty admission. Cleanup only targets successfully created service/unit paths. CI invokes the gated fixture separately after installer root checks. Linux compilation and diff checks passed; the new kernel fixture has not executed yet and is not run on the owner Mac. CI37479528801 remains live. Queued/future activation exclusion, installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.

### Native kernel fixture cleanup failures remain visible

Disposable cgroup fixture now reports service/slice stop, created-unit removal and daemon reload cleanup failures through the test result rather than silently ignoring them. It retains vacancy guards and only attempts service cleanup after successful service creation. Linux compilation and diff checks passed; fixture execution remains pending publication. CI37479528801 remains live. Queued/future activation exclusion, installed publication/ownership/bootstrap/runtime reconstruction and complete application acceptance remain unfinished.
