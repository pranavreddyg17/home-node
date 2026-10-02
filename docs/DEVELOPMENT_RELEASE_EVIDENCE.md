# Development release evidence

`packaging/debian/build.sh VERSION` emits these unsigned artifacts:

| Artifact suffix | Content |
| --- | --- |
| `.deb` | Script-free development package |
| `.deb.sha256` | Package transport checksum |
| `.deb.sbom.json` | CycloneDX 1.6 package binding and staged file inventory |
| `.deb.gomodules.json` | Build information from the six actual packaged Go binaries |
| `.deb.frontend.json` | Emitted asset hashes and bundled npm package lock identities |
| `.deb.evidence.sha256` | Relative-path checksums for all three evidence documents |

The module record includes executable SHA256, embedded Go version, main module,
dependencies, replacement modules and build settings. It reads binary build
information without executing the staged binaries. Module versions refer to the
compiled inputs, rather than every dependency in the repository's module file.
Local replacements remain explicit, including their `(devel)` version; they
must not be silently reported as the original upstream module/version.

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

Frontend evidence records packages represented in emitted chunks and compares installed names/versions with lock entries. CI separately compares recorded asset hashes and exact file set with the extracted package, and dependency identities with the reviewed npm lockfile. This does not independently reconstruct bundler reachability or authenticate installed npm package contents.

The file SBOM, module and frontend evidence explicitly remain incomplete. Frontend bundle
and host/guest system dependencies, licenses/notices, vulnerability assessment,
full CycloneDX dependency integration, authenticated provenance, signing,
controlled promotion, installation/rollback and hardware qualification still
need completed evidence before distribution as a supported release. The updater's
package-binding gates alone do not authorize installation.
