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
