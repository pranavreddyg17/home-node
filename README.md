# HomeNode

Design for turning a supported spare Linux laptop into a private server for the owner's other devices. The first release includes private files, CPU-based local AI, isolated processing jobs, device access, verified updates, encrypted external backup, recovery, export, and removal.

The first implementation slice contains a Go host preflight, a loopback-only diagnostics API, and a React diagnostics screen. Workload execution is disabled. The application has not been security-audited.

To run the local development build, start the Go service and web development server in separate terminals:

```sh
go run ./cmd/homenode serve
```

```sh
cd web
npm ci
npm run dev
```

Open the local address printed by Vite. For a machine-readable host report, run `go run ./cmd/homenode doctor`. A nonzero exit code means prerequisites were not met. A passing report is only a prerequisite check; this build still cannot run apps or jobs. The supported host target is Ubuntu Server 24.04 LTS on x86-64. macOS is useful for building and testing the UI but will correctly fail the host check.

Run `go test ./...`, `go vet ./...`, and `npm --prefix web run build` before committing. The web build is not yet served by the Go binary; the production TLS, enrollment, installer, VM runtime, and backup workflow remain in the implementation plan.

- [End-to-end product specification](END_TO_END_PRODUCT_SPEC.md) defines scope, user journeys, components, state, API responsibilities, and completion criteria.
- [Implementation and release plan](IMPLEMENTATION_PLAN.md) defines milestones, dependencies, proposed code layout, estimates, and acceptance gates.
- [Security research and rationale](SECURITY_RESEARCH_AND_PLAN.md) documents threats, isolation/network decisions, primary sources, and required security evidence.

Next, validate two confined VMs on real supported hardware and record the isolation and resource results. Do not treat passing diagnostics as proof that VM isolation is safe.
