# Service templates

These development templates are packaged under `/usr/share/homenode/systemd`,
not activated automatically. Setup must create distinct system users `homenode`
and `homenode-transfer`, private primary groups, and the shared
`homenode-runtime` socket group. Neither unprivileged identity belongs to
`libvirt`, `kvm`, or the other's private group. Transfer alone can access guest
channel files; controller alone can access management state and TLS keys.

A root-owned `/etc/homenode/services.env` supplies `TAILNET_IP`, `HTTPS_PORT`
(use an unprivileged port such as 8787), `HTTPS_ORIGIN`, `POLICY_GENERATION`,
`CONTROLLER_UID`, `RUNTIME_GID`, and `TRANSFER_GID`. No secrets belong in this
file. Values must match the actual local account IDs and approved root-owned
runtime policy. Missing values must be caught by setup; programs reject invalid
values. TLS key directory is root-owned, readable only by the controller group.
No default address, private key, publisher trust root, or owner is included.

Supervisor needs root to provision raw volumes/channel ownership and communicate
with libvirt. It has no management/TLS access inside its mount namespace.
Controller gets only private control state; transfer gets its socket directory.
Controller/transfer drop all capabilities. Supervisor confinement still requires
Supervisor retains only ownership, protected filesystem access and process
inspection capabilities; it cannot administer networking or mount filesystems.
independent physical-host verification; these unit restrictions do not prove the
hypervisor boundary. Controller host diagnostics will report KVM inaccessible
under its private device namespace; the root supervisor remains authoritative
for workload eligibility.

Before promotion into `/etc/systemd/system`, the resumable installer must validate
host prerequisites, identities, filesystem ownership, tailnet/TLS policy, signed
catalog/images, and measured resource limits. Adjust the workload slice to match
approved policy while preserving host reserve. Never place the supervisor in the
workload slice: it must remain responsive to stop an overloaded guest.

Linux CI verifies unit syntax. Real systemd startup/restart, shutdown supervision,
libvirt/AppArmor behavior, socket permissions and resource enforcement remain
physical-host release gates. Backup needs an additional clean-stop maintenance
protocol; ordinary runtime stop currently destroys the guest and cannot be used
as a claim of consistent application backup.
