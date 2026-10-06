# Reserved guest DAC launch: unresolved production boundary

The manager now persists guest UID/GID assignments, but Linux preparation deliberately refuses reserved-DAC activation until ownership and device access are coordinated. XML schema validation and a setpriv process credential test do not establish a working KVM launch.

## Upstream evidence

In libvirt v10.0.0, the DAC child-label path supplies the selected UID/GID to the command object. The generic child launcher calls virGetGroupList before forking. That helper skips supplementary-account lookup when a UID has no system database entry, then includes the selected primary GID. These paths support keeping reserved UIDs unallocated in NSS; creating passwd accounts would introduce group memberships that need separate qualification. This is source evidence, not evidence of the installed distro build or every launch helper path.

Sources: [DAC driver](https://raw.githubusercontent.com/libvirt/libvirt/v10.0.0/src/security/security_dac.c), [command launcher](https://raw.githubusercontent.com/libvirt/libvirt/v10.0.0/src/util/vircommand.c), [identity utilities](https://raw.githubusercontent.com/libvirt/libvirt/v10.0.0/src/util/virutil.c).

## Required launch experiment

Use only a disposable supported Linux/KVM host and an independently qualified reserved UID. Keep the existing enforcing AppArmor profile, bounded cgroup and zero-capability/NoNewPrivs verification. Supply a static numeric DAC label with relabel disabled, descriptor-qualified guest-owned0600 data volume, and a guest-owned0710 channel directory whose group is the transfer service group.

Observe the installed libvirt/QEMU versions, effective device ownership/ACLs, image parent traversal permissions and all process task credentials. Prove successful cold launch, adapter authentication, stopped restart with the same UID/GID, and audit refusal after identity drift. Show that a second guest cannot read or write the first guest's data/channel, and cannot acquire extra groups or capabilities. Stop and preserve uncertain state after any failed verification.

## Device policy decision remains gated

Current startup gives image/volume parents traversal through the libvirt-qemu group. An arbitrary primary group therefore cannot be assumed to reach those files. Likewise, /dev/kvm access cannot be inferred from XML or an unprivileged sleep process. Evaluate exact UID-specific device access against a single qualified primary group, including reboot/udev recreation and revocation. Do not add broad supplementary groups or relax the credential verifier merely to make a fixture start.

Before enabling production reserved-DAC preparation, implement the chosen device/image policy, immutable ownership intent, stopped-runtime barrier, volume/channel retries, maintenance admission and cleanup. The native experiment must exercise the actual shipped service protections and installed libvirt build. Exclusive UID pool provisioning and prevention of future account/subordinate allocation changes remain separate mandatory work. The complete product specification remains authoritative.
