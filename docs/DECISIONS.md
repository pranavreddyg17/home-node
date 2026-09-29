# Implementation decisions

## Runtime transport (28 September 2026)

The initial three-workload profile will use **no virtual NIC**. A fixed virtio-serial channel carries the versioned guest adapter protocol over a supervisor-owned Unix socket. File bytes and AI requests pass through bounded application operations. Guest-originated payloads remain untrusted.

This removes the need for guest DNS or IP routes, including routes to the host, LAN, overlay, internet and other guests. It also makes networking fail closed before a VM starts: the reviewed XML contains no interface, hostdev, filesystem sharing, graphics, or QEMU argument extension. Future guest egress requires a separate reviewed profile. Host download/update services still require tightly controlled network access.

The XML schema and confinement controls must be checked against the supported Ubuntu libvirt release. Source: [libvirt domain XML](https://libvirt.org/formatdomain.html). No hardware isolation claim is made until guest-root and resource-exhaustion tests pass on physical Linux hardware.

## Identity

Use the maintained [go-webauthn](https://github.com/go-webauthn/webauthn) server and SimpleWebAuthn browser library. Require user verification and resident credentials. Pairing grants are bounded and single-use. Revoke a registered passkey together with its sessions and outstanding invitations. Synced passkeys represent an authorization identity, not proof of a particular physical device; the UI states that distinction.

Recovery consumes one offline code to authorize a replacement passkey ceremony. Only a successful ceremony advances the recovery epoch, revokes old devices and sessions, and rotates the remaining recovery codes. A failed ceremony does not delete the current identity.

## Persistent state

SQLite WAL with full synchronous commits, foreign keys, one connection/writer and explicit transactions. State directories and database files must be private. Unknown future schema versions fail rather than silently downgrading. Event retention is bounded; recovery material and bearer session secrets are never stored as plaintext.
