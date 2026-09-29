# Implementation record

HomeNode's end-to-end implementation is in progress. A completed code path is not a passed release gate.

## Delivered foundation

- Host prerequisite report, loopback diagnostics API, responsive dashboard.
- Go tests, vet, Linux cross-build and browser smoke checks passed for the initial slice.
- Persistent SQLite identity; WebAuthn enrollment/login; capability-scoped pairing, sessions, revocation, recovery and redacted diagnostics.
- Browser end-to-end test passed with a Chromium virtual authenticator: setup, pairing, denied admin access, revoke, sign out/in, recovery invalidation and mobile width.
- Production requires private-network binding and TLS; explicit development mode is loopback-only.

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
