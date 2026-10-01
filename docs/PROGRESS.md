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
