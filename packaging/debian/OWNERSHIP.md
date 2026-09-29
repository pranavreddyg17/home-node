# Development package ownership

The package owns `/usr/bin/homenode`, `/usr/lib/homenode`,
`/usr/share/homenode` (web assets and inactive service templates), and
`/usr/share/doc/homenode`. Binaries and web assets
are root-owned and writable only by root. The guest executable is a build input
for immutable guest images; it must not be launched on the host.

This package does not automatically create users, start services, edit network
policy, enroll Tailscale, provision TLS, create disks, or install publisher keys.
It has no installation/removal hooks. Installation and ordinary removal leave
owner content and configuration alone. There are no default credentials.

Runtime state under `/var/lib/homenode`, administrator-approved configuration
under `/etc/homenode`, and external backups are deliberately outside this
package's payload. Provisioning these directories and service identities,
verified immutable images, signed catalog, resource slice, and private HTTPS
remains required before execution. Do not put private keys in a package.

`SHA256SUMS` records payload integrity; it is not publisher authentication.
Development packages are unsigned and are not a supported production release.
The release pipeline must add reviewed provenance and signing, verified update
metadata, licenses, host setup/rollback, service management, and release evidence.
