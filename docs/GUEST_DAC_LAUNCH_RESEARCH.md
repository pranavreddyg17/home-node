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


## Listener provenance research and next transport experiment

Libvirt10.0.0 [host chardev preparation](https://raw.githubusercontent.com/libvirt/libvirt/v10.0.0/src/qemu/qemu_process.c) creates a listening Unix backend and passes its descriptor to QEMU. The [socket helper](https://raw.githubusercontent.com/libvirt/libvirt/v10.0.0/src/qemu/qemu_command.c) binds/listens in the daemon and adjusts pathname permissions. [Linux Unix socket documentation](https://man7.org/linux/man-pages/man7/unix.7.html) defines SO_PEERCRED using credentials at connect/listen/socketpair time. Inference: the inherited listener is expected to report its privileged creator rather than QEMU's later DAC identity. Changing pathname ownership would not repair that authentication assumption.

Added a native pre-verification connection observing peer UID and immediately closing without any adapter frame. It grants no production admission and does not assert that a root peer is trustworthy. Actual observation awaits the next publication.

The preferred next experiment is reversing this channel: a transfer-owned per-domain listener exists before launch, with descriptor-pinned guest-only path access; QEMU uses the Unix client backend after dropping to its unique UID. Verify actual libvirt command construction, connecting UID, enforcing AppArmor, restart/reconnection and denied sibling/root clients before selecting it. Production needs a bounded authenticated accepted-connection registry, supervisor preparation/start/stop ordering, durable lease generation binding, teardown/revocation, and existing request serialization/cancellation preserved. Do not merely change XML while transfer still dials, or add a root-UID fallback. This remains a design to qualify, not an implemented transport.


CI37590289728 failed atc08e20d, but its explicit marker proves the draft memory observer passed real-QEMU positive/excessive-limit/other-ID/cancellation checks before unchanged production Verify refused missing leaf memory.max. Ordinary Linux race tests also passed. The production memory check now calls the retained-procfs membership/resource-domain observer instead of demanding a leaf file; exact domain scope and nearest threaded resource-domain bounds remain required. The process-pinning follow-up and inherited peer observation still require native qualification in the next run. Channel ownership/peer admission remains strict, so this does not enable reserved-DAC production preparation or satisfy full launch acceptance.


## Native listener observation corrects provenance inference

CI37590886954 failed at9ce7a0b after the process-pinned memory observer succeeded. The actual inherited listener peer UID was2000000000, matching QEMU, while pathname metadata remained root:root0775. This contradicts the prior prediction of a root peer: daemon bind/listen source alone was insufficient to establish final connected credentials. The underlying reason, potentially subsequent QEMU listen behavior, is not yet verified. Do not treat the earlier inference as a production fact.

Production memory verification progressed; the remaining policy refusal is channel pathname ownership admission. Guest-UID peer authentication remains appropriate on this observed stack. Next qualify a narrowly admitted, descriptor-pinned libvirt-created root-owned socket only after exact process/domain verification, retaining strict UID peer checks and alias/replacement refusals. Root ownership must not become general root-peer trust. The guest-connect experiment remains an alternative to qualify rather than a selected replacement architecture. Full adapter and restart acceptance remain unverified.


CI37591471926 completed failure at788375e. The actual hardware branch ran GuestConnect successfully in4.294s, establishing two firmware-only cold launches with assigned peer PID/UID/GID and DAC/AppArmor/process-memory observations in the synthetic fixture. The following inherited-listener test again observed guest UID2000000000 and failed unchanged socket ownership admission. The root negative/adoption follow-up was not in that run.

The pending production reserved-DAC Verify path now qualifies a single-link root:root0775 socket only after exact QEMU process/isolation checks and a no-payload connection proving the expected PID/UID/GID. It re-admits pinned socket/parent metadata after that observation, then descriptor-chowns guest:transfer and chmods0660 with final inode/ownership checks. Generic admission remains guest-owned only and transfer request authentication still requires the guest UID. Native adoption/restart and root-peer refusal must pass before accepting this change; production pool/preparation policy remains disabled.
