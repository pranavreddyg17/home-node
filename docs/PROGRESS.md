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
