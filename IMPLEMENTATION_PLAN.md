# Personal compute server implementation and release plan

Revision 1 — 28 September 2026. This is the build sequence for the [end-to-end specification](END_TO_END_PRODUCT_SPEC.md), under the controls in [the security research](SECURITY_RESEARCH_AND_PLAN.md). The repository contains an initial control plane and interface. See [implementation progress](docs/PROGRESS.md) for verified functionality. Milestones below are release acceptance gates, not claims of completion.

**Architecture decisions to hold stable.** Use Go for host services, React/TypeScript for the first-party interface, optional Three.js for device visualization, SQLite for local metadata, QEMU/KVM with libvirt for app isolation, a restrictive Tailscale deployment for client access, verified release metadata for updates, and restic for encrypted external-drive backup. Start on one supported x86-64 Linux release. Do not add Kubernetes, Redis, PostgreSQL, a public cloud control plane, a custom VPN, or a second backend language unless evidence requires it.

**Resolve the expensive uncertainties before building the dashboard.** The first milestone must prove host confinement, firewall ordering, VM memory overhead, HTTPS/passkey bootstrap, package maintenance coverage, and a viable CPU inference profile. Failure changes the supported scope or runtime choice; it must never trigger automatic fallback to weaker security. Keep a decision log containing tested versions, hardware, results, and why alternatives were rejected.

| Milestone | Deliverables | Required demonstration | Planning range |
|---|---|---|---|
| M0 — Feasibility and contracts | Hardware checks, two minimal VMs, process/privilege map, signed image spike, resource measurements | Compromised guest cannot reach host secrets, another disk, LAN, or management; control survives overload | 1–2 weeks |
| M1 — Installation and identity | Signed package, resumable install, local bootstrap, private HTTPS, passkeys, pairing, revocation, recovery codes | Fresh host and phone complete enrollment; unauthenticated peers fail; revoked stream terminates | 2–3 weeks |
| M2 — First complete compute workflow | Supervisor journal, durable state, transfers, one conversion template, basic UI, event streaming, cancellation | Upload → isolated execution → verified output → download; interrupt/retry without duplicate effects | 3–4 weeks |
| M3 — Complete daily product | Private Files, CPU AI, app lifecycle, capacity admission, mobile UI, accessibility, optional map | Two devices use all three workflows under competing resource demands with honest status | 3–5 weeks |
| M4 — Maintenance and recovery | Verified updates/catalog, migration policies, encrypted drive backup, restore wizard, exports, uninstall | Failed update recovery and complete restore to another eligible host; old sessions remain invalid | 2–3 weeks |
| M5 — Security assessment | Abuse/failure suites, fuzzing, dependency/release checks, external review, fixes | Findings retested; security gate passes on supported matrix | 2–4 weeks |
| M6 — Pilot and release | Installer/docs refinement, diagnostic exports, 72-hour soak, support and advisory process | New tester completes installation through restore/export without developer intervention in normal flows | 2–3 weeks |

The planning range is approximately 15–24 engineer-weeks for an experienced full-time systems engineer, plus external review scheduling and unplanned remediation. It replaces the earlier 12–19 week baseline because it explicitly includes three usable workflows, a complete interface, packaging, migration, and removal. It excludes a custom OS installer, general app ecosystem, family accounts, multi-host operation, GPU qualification, and AI administration. Re-estimate after M0/M1 rather than treating this range as a commitment. Security work runs throughout; M5 is assessment of already implemented controls, not the first hardening phase.

**Build by complete workflows.** M2 is the first usable vertical slice and should deploy through the real installer, authenticate through the real session path, execute inside the real VM boundary, and persist real job/artifact state. Mock hardware is appropriate for fast tests but cannot satisfy the release demonstration. Show actual error, pending, interrupted, and retry states in the UI alongside success.

| Workstream | Concrete first tasks | Dependency |
|---|---|---|
| Host/runtime | Eligibility checker, safe libvirt template, confinement assertion, policy-before-start ordering, stop enforcement | M0 |
| Identity | Bootstrap state machine, maintained WebAuthn integration, scoped sessions, device registry, recovery epoch | Trusted hostname and TLS |
| Supervisor protocol | Typed requests, peer authentication, resource IDs, request deduplication, protected policy/journal | Runtime prototype |
| Metadata | Schema and migrations, operation/attempt invariants, event ordering, queue leasing, reconciliation | API contract |
| Transfers | Chunk offsets/hashes, quota reservation, finalize, expiration, immutable input handles | Authentication and storage policy |
| Jobs | Preset manifest, attempt lifecycle, timeouts, guest adapter, output commit, cancellation | Supervisor and transfers |
| Files/AI | File lifecycle/trash/export; tested model profile, bounded inference, history, cancellation | VM lifecycle and core UI |
| UI | Setup wizard, status/error system, six screens, keyboard/touch accessibility | Stable typed API fixtures |
| Updates | Pinned trust roots, metadata verification, release staging, migration compatibility, recovery | Package ownership inventory |
| Backup | Registered drive, maintenance orchestration, consistency manifest, restic integration, restore | Stable schemas and app storage |
| Release/support | Signed deliverables, CI evidence, known limits, diagnostics, reporting and response | Every milestone contributes |

**Repository structure for implementation.** This is the target layout; the implementation record tracks created components.

```text
cmd/
  homenoded/               API, scheduler, reconciler
  homenode-supervisor/     narrow privileged runtime interface
  homenode-transfer/       bounded byte transfer service
  homenode-maintenance/    verified update and backup entry points
  homenodectl/             installation, local recovery, diagnostics
internal/
  auth/ policy/ state/ jobs/ apps/ transfers/ catalog/
  supervisor/ runtime/ updates/ backup/ audit/
web/                      first-party React interface
contracts/                OpenAPI and versioned local/guest protocols
guest/                    minimal images and workload adapters
catalog/                  signed manifest sources and tests
packaging/                Debian package, service units, owned configuration
tests/                    unit, integration, physical-host, abuse, recovery
docs/                     setup, ownership, troubleshooting, release evidence
```

The maintenance executable can provide shared code with separately privileged service entry points; do not give a web-accessible backup endpoint automatic update authority. Keep the wire format and supervisor API small. Generated clients may share schema definitions, but authorization remains enforced server-side and supervisor-side.

**Contract freeze before broad implementation.** Review OpenAPI, local supervisor protocol, guest-adapter protocol, catalog manifest schema, database invariants, update envelope, and backup manifest. Define maximum message sizes, deadlines, version negotiation, error codes, idempotency scope, authentication context, and cancellation behavior. An incompatible client or guest version fails explicitly; it never ignores unrecognized security fields.

**Release artifacts.** Ship a signed package/repository, pinned trust bootstrap, immutable guest/catalog artifacts, dependency inventory and licenses, recoverable configuration ownership record, setup guide, local recovery guide, backup/restore guide, and known limitations. CI signing credentials are separated from ordinary build jobs and releases require a reviewed promotion path. Include a provenance chain and regression evidence for all supported combinations. Development images and signing roots must be distinct from release roots.

**Acceptance ownership.** The maintained [product acceptance evidence matrix](docs/PRODUCT_ACCEPTANCE_MATRIX.md) tracks release demonstrations and remaining evidence boundaries. Each security requirement maps to a threat, enforcing component, test, and retained result. Each user journey maps to an interface flow, API operation, stored state, failure behavior, and end-to-end test. This matrix must be maintained as code changes; passing only a frontend test cannot establish that a firewall or storage boundary works.

| Gate | Evidence needed before release |
|---|---|
| Functional | Fresh install, all three workloads, restart, revoke, update, backup/restore, export, uninstall |
| Security | Research-plan negative tests, supervisor/protocol fuzzing, guest isolation, browser boundaries, malicious releases |
| Data integrity | Checksummed transfer, no premature success, consistent app/metadata backup, replacement-host restore |
| Reliability | Interrupted operations, bounded retries/logs/queues, resource exhaustion, power loss, 72-hour soak |
| Usability | Nondeveloper tester completes ordinary flows; errors explain recovery; accessible keyboard/mobile controls |
| Supply chain | Verified artifacts and publisher identity, package maintenance coverage, licenses, rotation drill |
| Operations | Redacted diagnostics, advisory/reporting path, release ownership, end-of-support process |

The security gate requires no unresolved critical/high findings in the supported threat model and an independent review of identity, runtime authority, network policy, update trust, and recovery. This is not a certification or proof that no vulnerabilities exist. Lower-severity findings have documented disposition and owners. Any unsupported configuration is clearly outside the published claim.

**Pilot constraints and expansion.** Begin with a small number of consenting owners on the tested matrix. Do not collect their content to diagnose problems; use opt-in redacted support bundles. Compare setup completion, successful jobs, recovery success, resource use, and support effort. Expand only when maintenance and incident response can support the new surface. GPU, third-party app origins, remote immutable backups, and household accounts each need their own design review and acceptance tests.

The first implementation task is M0: produce the Linux eligibility report and a two-VM isolation/resource test with saved evidence. The first product demo is M2: a paired phone completes a real isolated conversion job and retrieves its result.
