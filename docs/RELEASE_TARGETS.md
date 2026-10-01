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
