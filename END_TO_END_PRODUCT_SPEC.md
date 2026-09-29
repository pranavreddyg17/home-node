# Personal compute server end to end product specification

Revision 1 — 28 September 2026. Product name: HomeNode. Status: design for implementation; no complete application or security guarantee has been delivered yet.

This specification completes the product decisions in [the security research](SECURITY_RESEARCH_AND_PLAN.md). The [implementation plan](IMPLEMENTATION_PLAN.md) defines delivery order and acceptance evidence. This document is authoritative for version-one scope and user behavior; the security research remains authoritative for threat assumptions and required protections. Later features must meet those protections rather than quietly bypass them.

**Product promise and release scope.** An owner installs HomeNode on a supported spare laptop, connects their other devices, and uses private files, local AI, and background processing through one interface. The product includes installation, administration, workload execution, updates, data export, backup, recovery, diagnostics, and removal. The complete lifecycle is the unit of delivery.

Version one supports one dedicated Linux server, one owner, multiple paired owner devices, and three maintained workloads. It provides a responsive browser application with an optional installable shell. Clients use the existing Tailscale client; no separate HomeNode mobile app is required. This dependency must be visible before setup. Family accounts, arbitrary app stores, public URLs, third-party web interfaces, LAN-only certificate provisioning, GPU passthrough, multiple hosts, and AI administration are later releases.

The initial host candidate is Ubuntu Server 24.04 LTS on x86-64 with working KVM, an SSD, maintained firmware, and enforceable host confinement. This is a product selection to validate on physical machines, not universal laptop compatibility. Canonical lists standard support through May 2029; support coverage varies between Main and Universe packages, so the release inventory must identify maintenance ownership for each dependency. [Ubuntu maintenance policy](https://ubuntu.com/security/esm).

| Capability | Complete version-one workflow | Explicit boundary |
|---|---|---|
| Private files | Upload with progress/resume, browse, download, rename, move to trash, restore, export | No network-drive protocol or public sharing links |
| Private AI | Choose a tested model profile, see download/storage requirements, chat, cancel generation, delete/export history | Local CPU inference; no cloud fallback or administrative tools |
| Processing jobs | Upload a video, select an approved conversion preset, queue, observe, cancel, download result | One maintained processing template; no arbitrary commands or images |
| Device access | Pair, name, inspect sessions, restrict capabilities, revoke | Owner devices only; no household multi-user authorization yet |
| Maintenance | Health details, verified update, restart, encrypted external backup, restore, diagnostics, uninstall | No claim of high availability or immutable connected-drive backups |

**Installation and first use.** Distribute a signed Debian package and documented installer workflow for the supported OS. The installer checks prerequisites and changes only the configuration it owns. It records an installation journal so interrupted setup can resume or roll back its own changes. A separate guide covers preparing a clean Linux installation. HomeNode must never silently repartition or erase an existing laptop; a future appliance image requires a distinct, explicit disk-erasure workflow.

1. Run a read-only eligibility check: virtualization, kernel and OS support, confinement, firmware status where discoverable, disk health/free space, time accuracy, network, and power behavior. Report unknown firmware eligibility honestly. Missing enforcement prerequisites block supported mode.
2. Show planned packages, service accounts, paths, networking changes, and disk reservations. Install approved dependencies and services; do not create a globally available administrator page.
3. Use a local-console bootstrap to enroll the host in the owner's private network. Provide a guided owner step for restrictive tailnet policy configuration and validation. Never retain a general tailnet-admin token on the host merely for convenience.
4. Obtain a valid certificate for a stable non-identifying host name and verify the HTTPS endpoint. Certificate/private-network setup failure returns an actionable diagnosis; it does not fall back to unauthenticated HTTP.
5. Present a short-lived enrollment code locally. Through the trusted HTTPS endpoint, the owner redeems it, registers a passkey, and prepares a second authenticator or recovery codes. Expired/replayed enrollment codes are rejected.
6. Pair a phone through the private network and issue a distinct revocable app session. Present app-use and administrative capabilities separately. Pairing a device is not equivalent to granting every session permanent owner authority.
7. Set a measured system reserve, workload budget, retention settings, and backup reminder. Recommend one workload at a time initially. Show unsupported model choices as unavailable with a reason.
8. Complete a sample processing task and download its result from the phone. Show any unfinished protection tasks, including an external backup and recovery drill.

Users may defer attaching a backup drive, but the dashboard must then say that data is not externally backed up. The product release itself cannot pass acceptance without the backup/restore workflow tested. A copied install command or a successful login alone does not count as onboarding completion.

**Interface and daily operation.** Use React and TypeScript for a single first-party interface. The primary navigation is Overview, Files, AI, Jobs, Devices, and Settings. App installation and resource settings live within the relevant workload and Settings. At phone widths, navigation becomes an accessible compact menu; the same actions remain available by keyboard and touch.

The Overview answers four questions: is the server reachable, what is running, what is waiting, and what needs the owner's action? Show specific conditions such as disk pressure, backup overdue, update awaiting restart, or device revoked. Do not collapse them into an unexplained security score. Telemetry is sampled and bounded; a disconnected client displays a last-observed timestamp rather than stale readings as live data.

Three.js is an optional, lazy-loaded map of the host and paired devices. It must not be required to pair devices or operate workloads. Show observed connection status and granted access distinctly; a grant does not prove a current connection. Support reduced motion and a list fallback. Bundle dependencies locally and render on changes rather than continuously when idle.

The first-party interface can share one HTTPS origin because it never serves guest-supplied executable markup. AI text and logs are rendered safely; downloads are attachments with safe content headers; active HTML/SVG previews and embedded third-party pages are excluded. Guest APIs are mediated by typed product adapters, not exposed through a general URL proxy. Introducing third-party web UIs later requires the separate-hostname/session design in the security research before launch. This resolves the per-app certificate dependency for the first release without weakening that future boundary.

**Concrete component ownership.** Implement Go services with explicit OS identities and a small shared contract package. Avoid splitting every feature into a network microservice: process separation follows privilege boundaries.

| Component | Owns | Must not have |
|---|---|---|
| HTTPS gateway | TLS, connection budgets, routing to fixed handlers | Runtime sockets, guest disk access, backup keys |
| API/controller | User/session authorization, workload intent, metadata, durable queue, user-facing events | Root shell, arbitrary libvirt/QMP access, signing keys |
| Privileged supervisor | Bounded VM lifecycle, enforceable resource and network profiles, protected operation journal | Internet-facing listener, arbitrary caller-provided commands or VM XML |
| Transfer service | Bounded opaque input/output streams and staging allocation | File parsing, arbitrary host paths, runtime control |
| Guest workload adapter | Fixed application protocol, readiness, bounded job execution and outputs | Host credentials or cross-app storage |
| Update service | Independently verified product/catalog packages and controlled activation | Unverified URLs from app requests |
| Backup service | Registered external destination, consistent snapshots, encrypted backup and restore | General runtime administration or unrestricted user-selected commands |

SQLite stores management metadata. Workload data resides in separate virtual disks. Use QEMU/KVM and libvirt confinement as selected in the security research. Host commands are typed library/API operations with bounded arguments. Narrow wrappers around existing tools may be necessary, but must never interpolate user input into shell strings.

The supervisor verifies caller identity, operation scope, catalog digest, resource limits, policy version, and ownership independently of API validation. Its operation journal and enforcement policy are protected from API writes. Guest acknowledgments are untrusted observations, not authorization. The supervisor can cap or terminate a guest even if the adapter is unresponsive.

**Packaging the three workloads.** Every release contains a signed, versioned catalog with tested guest images, image digests, adapter protocol versions, required resources, data schema, health probes, network grants, and backup/update procedures. No floating image tags. Prefer immutable guest system images and separate writable data disks. Do not build images on the user's laptop during normal installation.

Private Files gets a persistent VM and its own data disk. Private AI gets a separate persistent VM, explicit model cache quota, bounded context/concurrency, and local-only inference configuration. Video conversion gets an ephemeral job VM per attempt with a fixed preset manifest. Share no mutable disks between them. A file selected as input is copied or streamed through an authorized transfer; it does not create a host-mounted shared folder.

Each model entry needs a verified distribution source, integrity metadata, redistribution/license review, a supported CPU/runtime profile, maximum context, and measured memory envelope. The final model SKU is selected during the hardware milestone. Do not hard-code an unbenchmarked large model as a default. Model downloads and image pulls happen through the controlled download path; ordinary workloads have no internet access by default.

Network enforcement is installed before a VM is permitted to start. A firewall or confinement verification failure blocks new execution. Existing affected VMs are disconnected or stopped according to the local safety policy. On host reboot, the supervisor reconstructs policy and validates disks/identities before restoring approved persistent apps. It never starts guest networking first and hardens it afterward.

**Durable operations and state.** Store desired state, observed state, operation ID, attempt ID, policy generation, and timestamps. User actions that outlive an HTTP request return an operation identifier. Transitions and their events are persisted transactionally; log streams are bounded and can drop verbose output with an explicit truncation event.

| Object | Proposed states and rules |
|---|---|
| Host | Setup, ready, degraded, maintenance, recovery-required; offline is an observation by a client |
| App | Absent, downloading, verifying, installing, stopped, starting, running, stopping, updating, failed, removing |
| Transfer | Created, uploading, verifying, ready, failed, expired; only ready inputs can be scheduled |
| Job attempt | Queued, preparing, running, finalizing, succeeded, failed, cancelled, interrupted |
| Operation | Pending, executing, succeeded, failed, requires-action; retain reconciliation evidence |
| Backup | Preparing, copying, checking, completed, failed; completed is distinct from restore-tested |

The API first records intent. The supervisor journals the operation before causing a side effect and associates the created VM/resource with that ID. On retry, it returns the observed existing result rather than repeating execution. Reconciliation repairs missing acknowledgments. Do not promise exactly-once execution across crashes. An uncertain non-idempotent job becomes interrupted and requires explicit retry; a retry creates a new attempt.

Job completion requires process exit status, finalized artifact metadata, integrity checks, and durable event commit. A disconnected browser does not stop a job. Cancelling uploads cleans staged data after a grace period; cancelling compute uses bounded graceful shutdown followed by forced termination. Partial artifacts are labeled incomplete and never reported as successful output. Admission accounts for guest RAM, host overhead, input/output staging, logs, and disk reserve.

**Data and API contract.** Core entities are Owner, Device, Session, Host, CatalogVersion, AppInstance, Grant, Volume, Transfer, Job, Attempt, Artifact, Operation, Event, BackupSet, UpdatePlan, and RecoveryEpoch. Identity and secrets are separate from ordinary mutable app metadata. Every foreign-key relationship and object access carries the owner/device/app scope even while the product is single-owner.

| API group | Representative versioned contract | Behavior |
|---|---|---|
| Setup/auth | `/v1/setup`, `/v1/auth/passkeys/*`, `/v1/sessions` | Bootstrap only when unclaimed; use a maintained WebAuthn implementation |
| Devices | `/v1/devices`, `/v1/devices/{id}/revoke` | Enrollment, capability view, revoke sessions and active streams |
| Host | `/v1/host`, `/v1/host/health` | Eligibility, power, runtime status, resource budget |
| Apps | `/v1/catalog`, `/v1/apps`, `/v1/apps/{id}/actions` | Validated install/start/stop/update/remove actions return operations |
| Transfers/files | `/v1/transfers`, `/v1/transfers/{id}/chunks`, `/v1/files` | Offset/checksum validation, resumable bounded transfer, object authorization |
| Jobs | `/v1/jobs`, `/v1/jobs/{id}/cancel`, `/v1/jobs/{id}/artifacts` | Preset-only job spec, separate retry attempts, bounded outputs |
| AI | `/v1/ai/conversations`, `/v1/ai/generations`, `/v1/ai/generations/{id}/cancel` | Bounded generation and streaming; no management tools |
| Events | `/v1/events`, `/v1/operations/{id}` | SSE with cursor and reconnect; polling fallback |
| Recovery/maintenance | `/v1/backups`, `/v1/updates`, `/v1/diagnostics` | Fresh verification for sensitive actions; encrypted data export |

Generate and review OpenAPI before parallel frontend/backend implementation. Mutation retries use owner-scoped idempotency keys plus request-body hashes; changed content with the same key is rejected. Bind sensitive approvals to action, resource IDs, policy generation, expiry, and recovery epoch. Use stable error codes such as `HOST_UNSUPPORTED`, `CAPACITY_UNAVAILABLE`, `POLICY_DENIED`, `INPUT_INCOMPLETE`, `UPDATE_BLOCKED`, and `RECOVERY_REQUIRED`, each with an actionable UI explanation. Never put secrets into URLs or error details.

Authentication protects every resource and stream. Concurrency limits apply to connections, uploads, model generations, and jobs. UI progress measures actual phases/bytes; no invented completion percentages. Report queue position only when it is meaningful and stable enough to explain.

**Backup and replacement-host recovery.** Version one supports an encrypted restic repository on a separately registered external drive. This is a specific integration, not a new encryption format. The owner supplies the repository password when initiating backup/restore and stores recovery material elsewhere; automatic unattended backups are deferred until credential and destination-retention policies are implemented. Reminders are supported from the start. Restic supports local repositories and requires the repository password for recovery. [Restic repository documentation](https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html).

A registered drive is a host-managed backup target only, never guest USB passthrough. Match its identity and expected repository before writing; reject a missing drive rather than accidentally filling a directory on the host disk. The UI reports that a connected writable drive is vulnerable to host compromise; offline protection begins after safe removal. It is not described as immutable storage.

For the first implementation, announce a maintenance window, block configuration changes and new jobs, drain finite work, and stop persistent VMs cleanly. Back up their stable disks plus a consistent management snapshot and version manifest. Restart services in a guaranteed cleanup path even when the backup fails. This deliberately trades backup duration for a simpler consistency model; live incremental app backups are a later optimization. Use SQLite's backup API rather than copying a live database file alone. [SQLite backup API](https://www.sqlite.org/backup.html).

A backup set includes app data, model/catalog version references, required encrypted app secrets, and recovery metadata. It excludes active sessions, old overlay enrollment state, active job leases, and trust that could resurrect a revoked device. Set bounded cache behavior and protect root-filesystem reserve during backup. Show last completed backup, validation status, bytes, and last successful restore test separately. Repository checks help detect corruption but do not establish application recoverability. [Restic repository checking](https://restic.readthedocs.io/en/stable/045_working_with_repos.html).

On replacement hardware: pass eligibility checks, install a trusted release, attach the backup, provide recovery material locally, validate manifests and compatibility, restore app disks while disconnected from clients, create a new host identity/recovery epoch, and re-enroll passkeys and devices through the trusted bootstrap. The owner revokes the old host in the private-network account. Restored apps resume only after policy reconstruction and health checks. A changed host name may change passkey relying-party identity; do not assume passkeys transfer automatically to a new origin. A recovery wizard must explicitly handle this.

File trash has a documented retention period and quota, initially proposed as seven days. Expiration is disclosed before enabling it; trash remains part of quota accounting. Removing an app defaults to retaining its data for export. Destructive purge and backup-retention changes require separate confirmation and fresh verification. Ordinary package uninstall does not erase app data or backups. Data wiping must state its limitations on SSDs rather than promising forensic erasure.

**Updates, outages, and support.** Host OS packages follow maintained distribution update channels; HomeNode binaries and catalog metadata follow the verified release mechanism in the security research. Version one does not promise whole-OS atomic rollback. Before incompatible data migrations, take a verified backup and test forward recovery; a binary downgrade is allowed only when both security policy and data schema permit it.

The owner chooses a maintenance window. Notify about interrupted workloads and reboot requirements. A revoked or critically vulnerable app version is blocked from new starts; active instances follow a signed remediation policy with explanation and preserved data. Invalid update signatures never become an “ignore and continue” path. Failed updates retain diagnostics and a documented recovery command.

| Failure | User-visible result | Recovery behavior |
|---|---|---|
| Internet or overlay coordination outage | Reachability uncertain; last-seen timestamp | Existing workloads continue locally; no promise of new enrollment or guaranteed reconnection; local console remains available |
| Certificate expired or time invalid | Secure connection unavailable | Repair clock/certificate through local console; never bypass validation |
| Disk low | New uploads/jobs blocked with cleanup choices | Preserve control reserve; do not silently delete user data |
| Host restart with disk encryption | Server awaits local unlock if configured | Honest availability status; remote browser cannot unlock an unbooted service |
| VM or adapter crash | Workload failed/interrupted | Bound restarts with backoff; preserve evidence and avoid crash loops |
| Backup drive missing/full | Backup failed or awaiting drive | Leave last valid backup intact; restart any stopped apps |
| Lost phone | Device revoked | Terminate live sessions/streams and rotate affected credentials |

Health checks, bounded audit logs, and a redacted diagnostics export are included. Support bundles omit secrets, file contents, chats, and personally identifying names by default and are previewable before export. No remote support shell or automatic telemetry upload. The owner sees relevant software versions and support status. Release infrastructure needs a vulnerability-reporting address, triage ownership, signed emergency releases, advisory communication, and end-of-support policy.

**Measurable completion.** All targets below are proposed release tests on a documented reference host/network, not current performance claims. Dashboard interactions should remain usable while a job is resource-limited; measure ordinary API p95 under load with a target below 500 ms excluding transfer/model execution. Local session revocation should terminate access within 30 seconds; separately measure overlay revocation. Use a 72-hour mixed-workload soak plus reboot, disconnect, disk-pressure, invalid-update, and restore exercises. Validate a 1 GB resumable transfer and larger files up to a separately published tested limit.

End-to-end acceptance is: a new owner installs on supported hardware, pairs two devices, uploads/downloads a file with matching integrity, chats with a tested local model, runs and cancels conversion jobs, revokes a device, upgrades safely, creates an encrypted external backup, restores onto another eligible host, exports data, and removes the application without unintended data loss. Security tests in the research remain mandatory. Publish the supported matrix and known limits with the release.

**Product dependencies and costs.** Local compute/data do not require a HomeNode-operated cloud control plane. Installation, model acquisition, updates, and the initial overlay/certificate setup need internet access. Tailscale account terms, device limits, redistribution requirements, and licensing for models/media components need review before distribution; do not promise a permanently free service without that review. Operating costs include power, replacement storage, backup media, signing/release infrastructure, security review, and ongoing maintenance. These are release-planning dependencies, not reasons to expand the initial architecture.
