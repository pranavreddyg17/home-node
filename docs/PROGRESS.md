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
