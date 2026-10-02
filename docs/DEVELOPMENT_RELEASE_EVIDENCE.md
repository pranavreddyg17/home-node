# Development release evidence

`packaging/debian/build.sh VERSION` emits these unsigned artifacts:

| Artifact suffix | Content |
| --- | --- |
| `.deb` | Script-free development package |
| `.deb.sha256` | Package transport checksum |
| `.deb.sbom.json` | CycloneDX 1.6 package binding and staged file inventory |
| `.deb.gomodules.json` | Build information from the six actual packaged Go binaries |
| `.deb.go-notices.json` | Collected Go module root notice sources and hashes, marked incomplete |
| `.deb.go-notices.txt` | Readable Go notice source collection, marked incomplete |
| `.deb.frontend.json` | Emitted asset hashes and bundled npm package lock identities |
| `.deb.frontend-notices.txt` | Readable collected npm license/notice sources, marked incomplete |
| `.deb.evidence.sha256` | Relative-path checksums for all six evidence artifacts |

The module record includes executable SHA256, embedded Go version, main module,
dependencies, replacement modules and build settings. It reads binary build
information without executing the staged binaries. Module versions refer to the
compiled inputs, rather than every dependency in the repository's module file.
Local replacements remain explicit, including their `(devel)` version; they
must not be silently reported as the original upstream module/version.

Build and extracted-package regeneration supply reviewed `go.sum` to the collector.
Compiled third-party module path/version/source sums must match it, using replacement
identity when present. Local/unversioned replacements refuse that qualification.
Evidence records `sourceSumsVerified` and the exact `sourceSumsSHA256`; omission of
the optional reviewed inventory is observational collection and records false.
This checks source-sum consistency, not main-source provenance, checksum database
authenticity, licenses or vulnerability status.

Go notice collection matches effective compiled module path/version/sum with
`go list -m -json all` source metadata, then reads bounded regular module-root
LICENSE/COPYING/NOTICE files. The build runs `go mod verify` before and after
collection and only then copies the notice sidecar into the artifact directory.
This detects cache changes against Go's downloaded module records. It does not
authenticate those records independently or exclude a concurrent writer that
changes and restores cache contents between checks. Build hosts must isolate
their source cache from untrusted writers. Nested notices, full license review,
license approval remains unfinished. CI independently
compares the notice sidecar with compiled identities regenerated from extracted
executables and source notice bytes, checking cache integrity before and after.
The verifier shares bounded input parsing helpers, but reconstructs the source
notice inventory without invoking the collector. It also requires the readable
Go notice artifact to exactly match the source-verified records.

For review, first verify downloaded artifact checksums. These detect corruption;
they do not authenticate a publisher when downloaded alongside untrusted bytes.
Check executable hashes against the extracted package, inspect replacements and
build settings, and confirm that evidence belongs to the reviewed source/build.
A local replacement or modified source tree needs independent qualification.
Do not infer license permission or absence of vulnerabilities from module hashes.

CI runs the product's SBOM package-binding gate against generated evidence,
extracts the archive, independently compares file hashes and the complete `usr`
file set, then regenerates module evidence from extracted executables and requires
an exact comparison. The development package artifact upload retains these files
for 14 days. Passing these checks proves the checked identities and inventories;
it does not make the build a production release.

Frontend evidence records packages represented in emitted chunks, compares installed
names/versions with lock entries, and hashes the installed package manifests used
to obtain identity and declared licenses. The collector and verifier read regular
manifests through bounded no-follow descriptors and refuse size/mtime changes.
These checks protect the final opened file; they do not establish trusted parent
directory ownership. CI separately compares asset hashes and exact file set with
the extracted package, dependency identities with the reviewed npm lockfile, and
manifest hashes with the review/build installation. It does not independently
reconstruct bundler reachability or authenticate complete npm archive contents.
Declared licenses are evidence for review, not an approval or complete notices set.

To reproduce frontend evidence checks on a development checkout with installed
locked dependencies:

```sh
npm --prefix web run test:evidence
npm --prefix web run build
PYTHONDONTWRITEBYTECODE=1 python3 packaging/debian/verify_frontend_test.py
PYTHONDONTWRITEBYTECODE=1 python3 packaging/debian/verify_frontend.py \
  web/dist web/build-evidence/dependencies.json web/package-lock.json \
  web/build-evidence/frontend-notices.txt
```

For extracted package review, replace `web/dist` in the last command with
`EXTRACTED_ROOT/usr/share/homenode/web` and use the package's `.deb.frontend.json`.
Supply the package's `.deb.frontend-notices.txt` as the fourth argument to verify
its exact readable contents against source-checked evidence. Retain the independently reviewed lockfile and matching installed npm tree beside
it: verification checks those manifest bytes. Downloaded evidence alone cannot
supply trusted dependency identities. CI also runs the file SBOM and compiled
module tests; all checks must pass for the scope of development evidence claimed.


The file SBOM, module and frontend evidence explicitly remain incomplete. Frontend bundle
and host/guest system dependencies, licenses/notices, vulnerability assessment,
full CycloneDX dependency integration, authenticated provenance, signing,
controlled promotion, installation/rollback and hardware qualification still
need completed evidence before distribution as a supported release. The updater's
package-binding gates alone do not authorize installation.

CI runs pinned `golang.org/x/vuln/cmd/govulncheck@v1.8.0` against `./...`
with verbose text output and a five-minute limit, retaining the report for14 days.
Text mode preserves scanner failure for reachable findings; pipeline failure
also blocks on tool/database errors. A second gate scans all six independently extracted Linux/amd64 Go package
executables in binary mode with a ten-minute limit. Each retained binary report
has an adjacent SHA256 identifying the scanned bytes. Binary scanning reads build
information/symbols and does not execute these files. These checks cover the CI
source and package build contexts, not every platform/build-tag combination, npm,
distribution/guest image and model dependency inventory. Module-only findings
remain visible for review; a zero reachable-symbol result is not release approval.
See [official govulncheck documentation](https://pkg.go.dev/golang.org/x/vuln@v1.8.0/cmd/govulncheck).

After locked frontend installation, CI runs `npm --prefix web audit
--audit-level=high --json` across runtime and build dependencies with a five-minute
limit. Pipeline failure blocks high/critical findings and audit errors. The JSON
report, exact package-lock SHA256 and Node/npm versions are retained14days, also
on failure. Lower-severity findings remain visible and need reviewed disposition
before release. This queries registry advisory metadata; it does not prove bundle
reachability, package authenticity or absence of unreported vulnerabilities.
See [npm audit documentation](https://docs.npmjs.com/cli/audit/).

Retained source-scan evidence also identifies the checked Git revision. Go source
reports include exact go.mod/go.sum hashes and the scanner's reported version;
frontend reports include the revision beside the lock hash and tool versions.
These context files aid review and do not cryptographically authenticate reports.
