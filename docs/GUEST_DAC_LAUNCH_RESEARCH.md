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

## Read-only provisioning proposal

On a disposable supported Linux host, root can inspect a candidate using `homenode guest-uid-plan --owner-id <32-hex-installer-owner> --first-uid <first> --last-uid <last> --controller-uid <controller> --transfer-uid <transfer> [--backup-uid <backup>]`. The command requires protected local-only NSS and combines account/delegation and running-task conflicts. Any conflict in the proposed range refuses the entire proposal. Explicit service UID arguments are proposal inputs, not proof of the installed identities. Output reports `policyPublished:false` and `servicesActivated:false` with mandatory pending gates. This command does not reserve UIDs, change allocation policy, or enable reserved-DAC guests.

For an existing account installation, use `homenode guest-uid-plan --journal-dir /var/lib/homenode-install --first-uid <first> --last-uid <last>`. This mode rejects explicit owner/service UID overrides and derives them from completed account journals, verifying live owner-marked account steps and backup identity when journaled. Output includes the reviewed service UIDs. It still grants no policy publication or activation authority.

`homenode guest-uid-prepare --journal-dir /var/lib/homenode-install --first-uid <first> --last-uid <last>` commits qualified canonical intent to the create-only private `guest-uid-intent.json`. Exact retries retain its inode and sync file/directory; changed or uncertain existing records are preserved and refused. Preparation requires journal-derived identities and retains every pending gate. An intent is not provisioned runtime policy and does not prevent future host ID allocations by itself.

## Automatic allocation eligibility

The combined observer now reads protected login.defs and requires explicit unambiguous UID_MIN/MAX, SYS_UID_MIN/MAX and SUB_UID_MIN/MAX ranges disjoint from the candidate. Missing/duplicate/malformed ranges refuse. Shadow documents these ordinary/system/subordinate automatic allocation settings ([Ubuntu24.04 login.defs](https://manpages.ubuntu.com/manpages/noble/man5/login.defs.5.html), [useradd](https://manpages.ubuntu.com/manpages/noble/man8/useradd.8.html)). This checks defaults; it does not prohibit explicit UID arguments or command-line configuration overrides.

The observer also excludes systemd v255's documented default nspawn allocation range524288..1879048191. NSS absence alone is insufficient to reserve a numeric range there: systemd documents checking its base UID for conflicts ([versioned upstream range policy](https://raw.githubusercontent.com/systemd/systemd/v255/docs/UIDS-GIDS.md)). Actual distro build boundaries, other allocators, future policy drift and explicit privileged administration still require qualification/enforcement. No exclusive allocation guarantee is claimed. Existing generic manager/test UID pools remain unchanged; this stricter check applies to host first-provisioning observation.

`homenode guest-uid-check --journal-dir /var/lib/homenode-install` reads canonical saved intent, requalifies installed owner/service identities and current host first-provisioning eligibility, and rereads intent after lookups. It refuses altered intent or new conflicts without repair. A successful check is a snapshot and reports activationQualified:false; it is unsuitable as a running-pool audit because guest processes occupy their leased UIDs.

## KVM preflight API qualification

Host preflight now requires a no-symlink character-device open plus KVM_GET_API_VERSION returning12, with descriptor close success. The [kernel API documentation](https://docs.kernel.org/virt/kvm/api.html) identifies12 as the stable API, and the [Linux6.8 UAPI header](https://raw.githubusercontent.com/torvalds/linux/v6.8/include/uapi/linux/kvm.h) defines the query. It creates no VM/vCPU. This qualifies the checker process only; a single-primary-kvm-group guest policy still needs immutable image traversal, private volume/channel ownership, installed libvirt credentials and native launch proof. No such access policy is enabled by this preflight change.

The KVM probe now first pins O_PATH metadata and requires the standard misc device identity10:232 before driver I/O ([Linux6.8 misc allocation header](https://raw.githubusercontent.com/torvalds/linux/v6.8/include/linux/miscdevice.h)). It reopens the pinned descriptor through a retained procfs fd directory and checks the reopened inode/device identity before the API query. It never reopens the original mutable name. This avoids invoking another device driver merely to discover that a path is not KVM. Native root device/API access passed CI37585441086; reserved guest VM launch remains unverified.

## Single primary KVM group experiment

A disposable-root native experiment now selects a locally/task-unoccupied high UID and uses the existing non-root KVM device group as its sole primary group. It requires root-owned0660 standard device metadata and no extended ACL, launches a fixed Python API query through capability-cleared/NoNewPrivs setpriv, holds the child for every-task credential verification, and confirms unchanged device policy. The query pins device metadata before I/O. It is selected only on CI hardware that exposes KVM, otherwise native evidence remains unavailable. Passing would establish this experiment's device access and exact credentials only: it does not prove installed libvirt/QEMU groups, image traversal, volume ownership, udev/reboot behavior, or actual guest launch. Production remains gated.


## Native libvirt launch experiment

CI37586384346 completed successfully at1c2b18ac2183187d348dd160ab92adc25e4f5747. Its hardware branch actually executed both the root KVM API probe and sole-primary-group allow/deny experiment. This establishes reduced-credential API access and wrong-group EACCES on that runner; it does not establish a VM launch.

TestNativeReservedDACLibvirtLaunch now exercises production domain XML, Start, Verify, Stop and channel directory retry twice against an empty disposable libvirt service. It uses unused numeric identities, synthetic raw disks, a bounded owned systemd slice, and checks Unix listener peer UID before any payload. It retains fixture files and the unit after uncertain launch or stop outcomes. Diagnostics are restricted to the synthetic domain's credentials, AppArmor label, cgroup, socket metadata and bounded log. The initial UID argument type error was corrected and Linux compilation passed; native execution is pending.

The fixture boots firmware only. Its public temporary parent and proposed root:KVM-group immutable disk do not qualify installed production directory permissions. It does not qualify application adapters, shipped supervisor service protections, exclusive UID policy, reboot/udev persistence, or cross-VM isolation. Production reserved-DAC activation remains refused until those coordinated gates pass.


## First actual reserved-DAC launch evidence

CI37589363542 failed at3a7ea68 after QEMU successfully launched on Ubuntu libvirt10.0.0-2ubuntu8.19/QEMU8.2.2. Diagnostics showed UID2000000000 across all four UID fields, primary/supplementary GID993 only, zero inheritable/permitted/effective/ambient capabilities, NoNewPrivs1, and an enforcing libvirt AppArmor label. These are process observations from a firmware-only synthetic guest, not full guest acceptance.

The production verifier refused because the process was in a threaded emulator cgroup with no memory.max file. Its libvirt parent reported805306368 bytes, while the scope reportedmax and the owned slice1073741824. A production fix must prove the effective enclosing per-domain bound through protected cgroup traversal and process membership, including negative tests; accepting the aggregate slice alone is insufficient.

The adapter socket was root:root with mode0775; QEMU's command line showed the listener passed as an fd. Current channel admission correctly refuses a root-owned socket for the reserved guest UID. This contradicts the assumption that QEMU creates its listener after dropping privileges. SO_PEERCRED was not reached. Resolve listener provenance and peer authentication with upstream research and a coordinated design before enabling production; do not simply trust root peers or chown an arbitrary socket. Restart and guest-adapter payload acceptance remain unverified.


The [kernel cgroup-v2 documentation](https://docs.kernel.org/admin-guide/cgroup-v2.html) defines a threaded subtree's nearest non-threaded ancestor as its resource domain. This supports examining the enclosing libvirt resource domain, not accepting an arbitrary missing leaf limit. Added a strict scope-path parser binding the observed machine-qemu systemd scope to the exact requested HomeNode ID, rejecting another VM/slice, traversal, malformed ordinal, controls and excessive depth. Targeted tests passed on macOS. Descriptor-pinned cgroup/type/limit observation and process-membership rechecks are still required before wiring this parser into production Verify; it currently changes no activation behavior.
