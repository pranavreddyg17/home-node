# HomeNode

Design for turning a supported spare Linux laptop into a private server for the owner's other devices. The first release includes private files, CPU-based local AI, isolated processing jobs, device access, verified updates, encrypted external backup, recovery, export, and removal.

The current implementation includes passkey identity, scoped devices, recovery, isolated runtime services, resumable files, video jobs, private AI conversations, and a responsive interface. Production execution requires separately provisioned verified guest images and a qualified Linux host. See the implementation record for outstanding release gates.

Build and run a local development instance (Go 1.26+, Node.js 22.12+):

```sh
npm --prefix web ci
npm --prefix web run build
go run ./cmd/homenode serve --dev --state-dir .homenode
```

In a second terminal, issue the single-use enrollment code:

```sh
go run ./cmd/homenode setup-code --state-dir .homenode
```

Open `http://localhost:8787`, enter the code, create a passkey, and save the recovery codes outside the server. HTTP is allowed only in explicit loopback development mode. Production verifies an exact HTTPS origin, a trusted matching certificate/key in protected root-owned files, and a Tailscale IPv4 address assigned to this host. Protected certificate replacements are revalidated for new TLS handshakes; certificate issuance and renewal still require owner action. See [private HTTPS setup](docs/NETWORK_SETUP.md). A state directory pins its original origin; changing a passkey relying-party identity requires recovery.

Run `go run ./cmd/homenode doctor` for machine-readable host diagnostics. Ubuntu Server 24.04 LTS on x86-64 is the initial host target; macOS can develop the interface but cannot qualify KVM/AppArmor isolation.

Supported-host local release preparation is available through `homenode install-prepare`; see [installation preparation](docs/INSTALL_PREPARATION.md). It creates verified service identities, owned configuration and signed image placements while retaining resumable journals. Service activation and complete onboarding remain pending.

Verification:

```sh
go test ./...
go vet ./...
npm --prefix web run build
# Once per machine, install Playwright's Chromium:
cd web && npx playwright install chromium && npm run test:e2e
```

The browser test uses a virtual authenticator to exercise actual WebAuthn signatures, device scope, revocation, login and recovery against a temporary SQLite database. It does not replace real-device testing. To use an installed Chrome, set `PLAYWRIGHT_CHROMIUM_EXECUTABLE` to its executable path. See [implementation progress](docs/PROGRESS.md) for outstanding work and release gates.

- [End-to-end product specification](END_TO_END_PRODUCT_SPEC.md) defines scope, user journeys, components, state, API responsibilities, and completion criteria.
- [Implementation and release plan](IMPLEMENTATION_PLAN.md) defines milestones, dependencies, proposed code layout, estimates, and acceptance gates.
- [Security research and rationale](SECURITY_RESEARCH_AND_PLAN.md) documents threats, isolation/network decisions, primary sources, and required security evidence.

Next, validate two confined VMs on real supported hardware and record the isolation and resource results. Do not treat passing diagnostics as proof that VM isolation is safe.

To build an unsigned development Debian package on Ubuntu/Debian after installing web dependencies, run `packaging/debian/build.sh`. It packages Linux amd64 binaries and built web assets under `artifacts/`. It does not provision or start services. [Package ownership](packaging/debian/OWNERSHIP.md) documents installed paths and limits; production signing and the resumable installer are still outstanding.
