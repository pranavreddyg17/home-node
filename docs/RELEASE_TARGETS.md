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
