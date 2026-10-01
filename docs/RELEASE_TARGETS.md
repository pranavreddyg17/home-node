# Release target contract

HomeNode package targets use TUF length/hashes and the following signed `custom` metadata. Target trust comes from the protected TUF root and role workflow; this object has no separate signature format.

```json
{
  "schema": 1,
  "release": "0.1.0",
  "sequence": 5,
  "platform": "ubuntu-24.04-amd64",
  "catalogVersion": 3,
  "minimumSourceSchema": 3,
  "maximumSourceSchema": 4,
  "resultSchema": 4,
  "sbomTarget": "releases/0.1.0/sbom.json",
  "provenanceTarget": "releases/0.1.0/provenance.json"
}
```

Release sequence and minimum catalog version are checked against independently trusted host policy. A higher Debian version string does not override the sequence floor. The host state schema must fall within the declared source range; the resulting schema cannot be lower than that range. These declarations do not prove migration correctness or grant installation authority.

Platform is currently fixed to the product's initial Ubuntu 24.04 amd64 candidate. This is a compatibility filter; physical host qualification remains required. Custom metadata is bounded to 8 KiB, uses exact recognized field names, and rejects duplicate decoded keys, unknown fields and trailing values. Release labels cannot contain shell or unit syntax.

Package paths are relative canonical `.deb` target names. Acquisition requires SHA256, verifies any SHA512 too, rejects unsupported hashes and caps package bytes at 512 MiB. SBOM and provenance paths are distinct relative canonical JSON target names. Both must resolve through verified TUF metadata, declare SHA256 and have lengths from one byte through 8 MiB before package acquisition starts.

The current code acquires both evidence documents, verifies their exact TUF length/hashes and JSON syntax, and returns their bytes with the verified read-only package for later review. It does not establish SBOM completeness, vulnerability status, an authorized build identity or release promotion approval. Semantic evidence review, install journals, migrations, permitted rollback, trusted launch configuration and owner approval remain separate required gates before a release can be installed or claimed qualified.

## Installer trust bootstrap

`install-prepare` accepts optional paired `--update-root <local-root.json>` and `--update-root-sha256 <independently-verified-digest>` flags. The pin must come from independently trusted release setup, not a digest downloaded beside an untrusted root. Input is bounded, must satisfy upstream TUF root signature verification, uses a root role threshold of at least two, and keeps keys distinct across the four top-level roles.

Preparation journals an immutable root-owned 0400 `/etc/homenode/update-root.json` and root-owned 0700 update, metadata and download directories under `/var/lib/homenode-update`. It does not journal `metadata/root.json` as immutable configuration, initialize a current cache root, download packages, or activate updates. The current cache bootstrap/rotation ownership lifecycle and trusted release-policy configuration still require implementation.

After preparation has completed with update bootstrap configuration, run `sudo homenode update-trust-initialize` locally. This command requires Linux/root and an existing completed ownership journal. It accepts only an optional canonical `--journal-dir`; root bytes and pins come from journaled configuration, never from this command's request. It verifies all owned configuration before initializing current trust with recorded resumable intent. It does not activate updates, acquire a release, or install a package. A changed bootstrap or missing previously initialized cache root is refused for explicit recovery.

Installer repository policy can additionally be supplied through
`install.Configuration.UpdateRepository`. It requires a pinned bootstrap and
writes `etc/homenode/update-repository.json` as root-owned mode `0400` within the
configuration ownership journal. Schema one fixes HTTPS metadata and target
repository scopes on the same host, plus positive release-sequence and catalog
floors. Its catalog floor cannot be below the installation's catalog floor.
The current database schema is measured for each acquisition, rather than saved
as an immutable installation-time value. The installer CLI accepts this configuration through the complete set
`--update-metadata-url`, `--update-targets-url`, and
`--update-sequence-floor`, together with the pinned bootstrap flags. The
catalog floor comes from `--catalog-floor`. No updater service is activated.

On Linux, `updates.AcquireRelease` joins metadata refresh, signed compatibility
checks, evidence downloads, and the verified read-only package under one cache
lock. It rejects invalid repository configuration before changing the cache and
closes the package if final session cleanup or cancellation fails. It does not
install a package or establish vulnerability, provenance-identity, migration,
or owner-approval qualification. Protected repository policy loading, updating
persistent release floors after successful installation, and approved service
activation remain required before enabling product updates.

`install.Engine.ReadUpdateRepository` provides the privileged policy-loading
boundary on the real Linux host. It requires a completed ownership journal with
pinned bootstrap and private updater directories, verifies all journaled items,
and admits only the created root-owned mode `0400` repository policy entry with
matching content digest. Parsing is bounded to 8 KiB and rejects missing,
unknown, duplicated, case-aliased fields, trailing JSON, and incompatible values.
This loader does not activate an updater or authorize installing a release.

The Linux `install.Engine.AcquireUpdateRelease` method keeps installer ownership
locked while loading repository policy, checking the observed schema,
initializing or resuming the protected cache, and acquiring the release through
its owned download directory. It accepts neither repository URL nor bootstrap
bytes from its caller. Returned bytes still have no install authority; this
method is not yet connected to a maintenance command or owner-approved updater
service. The actual signed repository acquisition fixture tests the underlying
TUF/download chain; the complete installer-to-network chain remains to qualify.

Acquisition observes the live management database schema before and after the
network operation, without migrating it. A changed schema or failed final
observation closes the acquired package and returns a conflict. This is a
consistency check for acquisition, not an admission barrier for installation;
the eventual approved migration must retain exclusive maintenance admission
and revalidate compatibility at its execution boundary.

The proposed HomeNode Debian control-content gate,
`updates.ValidateControlArchive`, accepts only a bounded decompressed control
tar with one regular root-owned mode `0644` control file (and optional `.`
directory). It rejects maintainer scripts, triggers, links, unknown archive
entries, duplicate/unknown control fields, package or architecture mismatch,
and versions different from signed release metadata. Dependencies must exactly
match the maintained HomeNode distribution dependency set. This format matches
the current script-free development packaging contract. It is deliberately a
HomeNode format constraint rather than acceptance of every legal Debian format.
See [Debian binary package format](https://manpages.debian.org/bookworm/dpkg-dev/deb.5.en.html).
The parser does not yet decompress or verify the outer `.deb`, inspect the data
payload, or run in the eventual constrained package-inspection worker; it is not
currently an installation admission gate. Those integrations remain required.

`updates.InspectDebianArchive` reads the already-open package using bounded
sections without changing its descriptor offset. HomeNode's supported outer
layout is exactly `debian-binary` with `2.0`, then `control.tar` and `data.tar`,
optionally compressed using gzip, xz, or zstd. The structural check refuses
unsupported members/order, length overruns, bad ar padding/headers, extra trailing
members, packages above 512 MiB, and compressed control above 1 MiB. Returned
sections still contain potentially compressed bytes. They must undergo bounded,
constrained decompression and content validation before installation. The
acquisition path does not yet invoke this inspection gate; production activation
remains blocked on its full integration and real package qualification.

`updates.ValidateCompressedControl` currently supports bounded uncompressed,
gzip, and zstd control sections, verifies the complete compressed stream before
parsing, and rejects more than 1 MiB of compressed or decompressed control.
The zstd decoder uses one decoder worker with a 16 MiB window/memory limit;
the implementation is pinned to `github.com/klauspost/compress v1.20.1`
([upstream](https://github.com/klauspost/compress)). Context checks surround
input and output reads. These cooperative checks do not replace process CPU
limits. xz control decoding is still refused by this API: the evaluated Go xz
reader dictionary option is not a hard maximum, so xz support requires the
planned constrained inspection worker. The existing development package uses
xz and therefore is not yet qualified by this compression/content chain.

`updates.ValidateDebianControl` joins outer structure and compressed control
inspection on the same opened descriptor. A pathname replacement does not
redirect inspection, and the descriptor's current offset is unchanged. The
joined fixture validates a gzip control section within an ar container, rejects
version mismatch, hooks and truncation, and explicitly uses an unqualified data
payload: passing this control check says nothing about payload contents. Real
build output inspection and constrained xz support still require qualification.

`updates.ValidatePayloadArchive` inspects an uncompressed data tar without
extracting files. It bounds input/regular data to 2 GiB, each regular file to
256 MiB, entry count to 4096 and paths to 240 bytes. It requires root ownership,
normal directory/file modes, the five packaged executables and web index, refuses
links/special files and files outside HomeNode distribution paths, and hashes
all regular files against a complete, duplicate-free `SHA256SUMS` inventory.
The inventory proves internal consistency only, not authenticity or executable
qualification. Tests currently use small synthetic payloads; compressed payload
integration, real build inspection, process resource enforcement and release
qualification remain required before activating updates.

`updates.ValidateCompressedPayload` streams gzip, zstd or uncompressed data to
the payload validator, bounds compressed input to 512 MiB and preserves its
2 GiB output limit. Zstd uses one decoder worker, at most a 32 MiB window and
64 MiB decoder memory. Complete-stream/checksum errors reject the result.
Tests cover valid compressed inventory, corruption, truncation, duplicate tar
entries, trailing bytes and cancellation. Xz, isolated process enforcement,
real distribution-package inspection and installation admission remain pending.

`updates.ValidateDebianContent` joins archive structure, release-bound control
and complete payload inventory validation on one opened descriptor. Synthetic
whole-package tests cover uncompressed, gzip and zstd sections, rejecting
corrupt payload inventory even when control is valid, installation hooks and
release mismatch. This API is not yet executed by the acquisition/installation
path or a constrained worker. Xz support and actual distribution-package
qualification remain required; success does not establish provenance identity,
vulnerability clearance or owner approval.

Required executable files additionally undergo streaming ELF header sanity:
64-bit little-endian AMD64, executable or shared-object type, current ELF
version, expected header sizes and a nonempty program-header table contained in
the declared file length. Header bytes remain included in the inventory hash.
Tests reject scripts and a different architecture. This is format compatibility,
not proof of executable behavior, linkage, build identity or runtime safety.

The opt-in Linux `TestNativeBuiltPackageContent` consumes the actual CI
`0.1.0~ci` package through a pinned descriptor. Distribution `dpkg-deb` streams
control and data tar sections (including the development build's xz sections)
to the validators without extracting or installing during this test. Execution
is refused as root and bounded by a two-minute command context. This qualifies
build-output compatibility only; it does not implement the production inspection
sandbox or change the production xz refusal. CI must execute this new fixture
before real build compatibility can be claimed.

The Linux `updates.ValidateDistributionPackage` API centralizes distribution
xz decoding and complete content validation on a read-only descriptor. It
refuses root and writable descriptors, uses fixed `/usr/bin/dpkg-deb` and fixed
arguments/environment, and bounds the entire inspection to two minutes.
Cancellation terminates the decoder process group, including descendants;
complete decoder exit status is required. The real-package CI fixture now uses
this API. Dedicated worker UID, network/filesystem restrictions, cgroup limits,
privileged descriptor handoff and activation remain mandatory and unimplemented;
the API is not connected to root acquisition or installation.

`cmd/homenode-inspect` is the initial Linux inspection worker executable. It
accepts only expected release identity, consumes descriptor 3, requires
non-root UID/GID without supplementary authority and `NoNewPrivileges`, and
requires an inherited private root-owned mode `0400` regular package with one
link. Successful output reports content validity and explicitly no installation
authority. It currently is not packaged or launched by the updater. Dedicated
service identity, full capability/network/filesystem/cgroup enforcement,
trusted descriptor/result handoff and native cross-identity execution tests
remain required before production activation.

Worker startup additionally requires matching real/effective/saved UID and GID
and empty effective, permitted, inheritable, bounding and ambient Linux
capability sets. Missing, malformed or duplicate capability fields fail closed.
These startup checks still do not establish the pending filesystem, network or
cgroup sandbox. Actual build-content CI at d47a61a (run 36928911181) completed
successfully; the centralized decoder and worker changes await their own CI.

Distribution decoding now reads bounded sections of the inherited descriptor
through stdin (`dpkg-deb ... -`), rather than reopening `/proc/self/fd/3` under
the worker UID. Reopening can fail DAC checks on a root-owned mode `0400` file,
even when the worker possesses a readable descriptor. The new opt-in Linux CI
entry fixture copies the real built package into a root-private directory,
passes its read-only descriptor across UID/GID 804 with empty supplementary and
capability sets plus `NoNewPrivileges`, then requires content success with no
installation authority. Execution still awaits CI. This fixture does not prove
the pending filesystem/network/cgroup service sandbox.

The development package now includes `homenode-inspect`, and payload inventory
requires six executable entries including that worker. The inactive
`homenode-inspect.service` template uses systemd `OpenFile` to pass a read-only
private package to a dynamic identity, with no capabilities, private networking,
restricted address families, hidden HomeNode state/secrets, protected filesystem
and kernel settings, 256 MiB memory/no swap, 50% CPU, 32 tasks and bounded
startup/shutdown with cgroup termination. See the
[systemd 255 OpenFile contract](https://raw.githubusercontent.com/systemd/systemd/v255/man/systemd.service.xml).
No launch environment, private staging slot or service activation is generated
by the installer yet. Source-template and development build checks do not prove
runtime isolation; a native service fixture, trusted operation/result handoff
and privileged installation integration remain required.

The opt-in `packaging/inspection/systemd_fixture.py` copies all reviewed service
restrictions into a transient disposable Linux unit. It substitutes only the
test executable, expected release environment and an exclusively created private
package slot, preserving systemd `OpenFile` descriptor handoff to a dynamic UID.
The child checks worker startup/content validation and inability to reach a
confirmed listening host TCP endpoint. Source completeness, Python syntax and
Linux test compilation pass locally; native service execution awaits CI. This
fixture does not yet activate the product updater or qualify its root result
binding and staging lifecycle.

The inspection service child fixture additionally reads its unified cgroup
limits and requires effective 256 MiB memory, no swap, 32 tasks, OOM group kill,
and 50% CPU quota. It must also fail to read an otherwise world-readable host
`/tmp` marker, testing private temporary storage. These new runtime assertions
await native CI execution. HomeNode CI 36929627382 at fbaf582 completed
successfully, including centralized distribution decoding of real build output;
the subsequent cross-identity/service fixture changes are not covered by that
older run.

The worker's schema-one result now includes inspected release identity, package
SHA256 and byte length, with content validity and no install authority. It
hashes the descriptor before and after content inspection and refuses changed
identity. PackageIdentity is bounded, cancellation-aware and preserves file
offset. Native entry/service fixtures compare result identity with their actual
inherited descriptor. AcquiredRelease additionally retains the verified signed
target digest and length for the future privileged result comparison. Root
operation/result binding and activation still require integration and testing.

`updates.ValidateInspectionResult` requires every schema-one field exactly once,
with no unknown/case-aliased fields, nulls or trailing JSON, within 2 KiB. It
compares release, SHA256 and length against independently retained signed target
identity and requires content validity with installation authority false. Native
worker fixtures consume the emitted result through this parser. This parser
checks result content, not worker identity or successful protected execution;
its privileged operation binding and service completion evidence remain pending.

The inspection protocol additionally requires a canonical 20–64 character
maintenance operation ID. Worker output includes it and result validation must
match the protected caller's expected operation, refusing otherwise identical
package results from another operation. The inactive service expects trusted
`INSPECTION_OPERATION_ID` configuration, and native fixtures supply fresh IDs.
This correlation field is not an approval token or proof of worker completion;
the durable root operation and authenticated completion integration remain open.

Installer repository provisioning now includes root-owned mode `0700`
`var/lib/homenode-update/inspection` in the static ownership journal, with public
permissions refused by record validation. It creates no package slot, launch
environment or activation. The installation/journal item budget is consistently
bounded to 64 entries to accommodate the growing fixed allowlist; unknown paths
and duplicate records remain refused. Package publication/resume, trusted launch
configuration, exclusive root operation and completion/result journaling are
still required to connect acquisition to the inactive inspection service.
