# Go certificate verification security patch

CI run 39545958506 at `ee9181d` rejected Go 1.26.9 because the Linux source
scan found reachable advisory [GO-2026-6809](https://vuln.go.dev/ID/GO-2026-6809.json)
through `networkcheck.Validate` and `crypto/x509.Certificate.Verify`.
The official advisory identifies Go 1.26.10 and Go 1.27.3 as fixed versions
for their respective release lines. HomeNode remains on the 1.26 release line.

The module's required Go version and all four build/qualification workflows
now pin 1.26.10. Adapter metadata continues to record the actual compiler.
Historical evidence and synthetic metadata fixtures retain their original
versions; they are not claims that older binaries have received this fix.
No vulnerability check or certificate validation is disabled.

Local verification used Go 1.26.10, downloaded from the official Go release
index and checked against its published SHA-256. The full local race suite
passed. A host-native govulncheck v1.8.0 executable analyzed the Linux/amd64
target with CGO disabled and reported zero reachable vulnerabilities. It still
reports the uncalled `golang.org/x/crypto/openpgp` module advisory; this result
is not a general safety claim about every package in that module.

Native Linux CI, packaged-binary scans and rebuilt guest qualification remain
required at the updated toolchain revision. Older successful development
workflows remain component evidence for their exact source and compiler;
they do not qualify newly rebuilt or release-published artifacts.

## Frontend dependency patch

The next main run, 39546828950 at `c6aeffb`, passed the Go scan but failed
`npm audit` on Vite 8.3.1's bundled launch-editor advisories. The high-severity
[upstream advisory](https://github.com/vitejs/launch-editor/security/advisories/GHSA-587h-7wmj-h5pm)
describes Windows-specific command-injection prerequisites; it is not evidence
of a Linux production gateway exploit. The development toolchain is still in
scope for maintenance. Vite is now pinned to 8.3.4 with its resolved bundler
transitives, rather than suppressing the audit or changing its threshold.
A clean `npm ci`, audit, all six dependency-evidence tests, frontend build
and all 15 browser tests pass locally. Native delivery, package inventory and
CI must qualify the new assets.
