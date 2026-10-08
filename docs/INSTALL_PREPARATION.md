# Preparing a supported host

`homenode install-prepare` connects journaled account creation, configuration
publication and verified local image placement. Run it from the installed
package as root on an eligible Ubuntu Server 24.04 x86-64 host. Services remain
inactive; this command does not complete private networking or enrollment.
Maintained bootable guest releases and a complete installation wizard are still
pending. Development fixture images are not usable guest releases.

Obtain the signed catalog and all three `<sha256>.raw` artifacts from a trusted
release. Establish the publisher public key and minimum catalog version through
an independent trusted release channel; copying a key out of the catalog itself
does not establish publisher identity. The command requires those values rather
than inventing or accepting a local signing identity.

Example with owner-specific placeholders:

```sh
sudo homenode install-prepare \
  --publisher-key '<independently verified 64-character public key>' \
  --catalog-floor 4 \
  --catalog /path/to/release/catalog.json \
  --images /path/to/release \
  --bind 100.100.1.2 \
  --origin https://home.example.ts.net:8787 \
  --memory-mib 4096 --cpus 2 --instances 3
```

For update acquisition, append the independently reviewed update trust inputs:

```sh
  --update-root /path/to/release/root.json \
  --update-root-sha256 '<independently verified root SHA256>' \
  --update-metadata-url https://updates.example/metadata/ \
  --update-targets-url https://updates.example/targets/ \
  --update-sequence-floor 5 \
  --update-provenance /path/to/release/provenance-policy.json \
  --update-provenance-sha256 '<independently verified policy SHA256>'
```

These are continuation arguments for the preceding command. Replace the example
repository with the reviewed release repository; metadata and target scopes must
use HTTPS on the same host. Each file/pin pair is mandatory when supplied.
Provenance policy requires repository configuration and pinned TUF bootstrap.
See [the protected provenance policy contract](PROVENANCE_POLICY.md).
Acquisition refuses absent provenance policy even if older preparation completed
without it. Preparation alone does not activate updates or authorize installation.

Use the server's own Tailscale IPv4 address and canonical HTTPS origin. The
runtime budget reserves separate host capacity and must fit every release
profile, including concurrent Files/video execution and storage headroom. The
server measures capacity and verifies service identity; command input cannot
supply account IDs or fake host capacity. Default policy generation is 1,
HTTPS port is 8787, and the free disk reserve is 4 GiB. `--port`, `--generation`
and `--disk-reserve` can explicitly change these settings.

Before opening the installation journal, the command verifies the signed
catalog, local source file names/types/sizes, settings and host prerequisites.
Streaming image hashes are checked during publication. An artifact altered
since initial inspection fails publication. Errors can leave journaled partial
progress; rerun the same command with the same release/settings to reconcile it.
SIGINT/SIGTERM cancel work at the next checked boundary. There is a 30-minute
execution deadline. Ownership conflicts preserve existing paths and require
inspection rather than forced overwrite or automatic account deletion.

The default private journal `/var/lib/homenode-install` is created securely;
`--journal-dir` requires an existing private directory when changing that path.
Keep the journal with the host: it is the durable ownership record, not a cache.
Configuration and images are not automatically erased when a later phase fails.

Success prints JSON with `prepared: true`, `servicesActivated: false`, a preview
and remaining gates. Complete protected private HTTPS, restrictive tailnet
allowed/denied device checks, actual host enforcement and VM qualification,
service validation/activation, passkey enrollment and the phone sample job
before claiming a working deployment. Those activation/onboarding phases are
not yet orchestrated by this command. See [the journal implementation](INSTALLATION_JOURNAL.md)
and [network setup](NETWORK_SETUP.md).

Replay accounts for verified images already published by this installation;
matching foreign artifacts do not count. Owned partial staging is cleaned and
capacity is observed again. JSON separates the original disk requirement,
verified image bytes and free space still needed, while preserving measured
capacity. Preparation uses that catalog-derived storage admission instead of
doctor's provisional free-space floor; remaining host prerequisite checks are
mandatory. Other host processes can still consume space between observations.

After provisioning protected HTTPS files, run:

```sh
sudo homenode install-check
```

The command takes the installation lock and reads committed settings, trust
floor, accounts and image state from the existing journal. It rehashes all three
images and requires completed publication; a lost image acknowledgement must be
reconciled with `install-prepare` first. It rechecks actual account ownership
markers/membership and the running local Tailscale/HTTPS identity. The certificate
must be root-owned mode 0644, and the private key root:homenode mode 0640; the
shared runtime group cannot read the key. It does not start services or advance
journals. A custom journal path can be selected with `--journal-dir`.

Successful JSON separates artifact, live-account and HTTPS identity verification
and reports certificate expiry. It still lists enforcement/VM-overhead,
allowed/denied tailnet policy, activation and phone-enrollment/sample-job gates.
A local Tailscale certificate is not evidence of peer access policy. Results
are current observations rather than saved authorization to skip later checks.

## Reserved guest storage proposal

After account provisioning, inspect a proposed guest UID range on the dedicated
Linux host. Choose the entire range deliberately; conflicts refuse the proposal
rather than shrinking it. For example, a two-guest development range is:

```sh
sudo homenode guest-storage-plan --first-uid 2000000000 --last-uid 2000000001
sudo homenode guest-storage-prepare --first-uid 2000000000 --last-uid 2000000001
sudo homenode guest-storage-check
```

Each command accepts `--journal-dir` for an existing private installer journal.
`check` derives the range from saved intent and takes no range overrides. The
commands require root on Linux and completed, live-verified owned account
journals. They independently observe the standard root-owned mode 0660 KVM
device, require its group to differ from management and maintenance groups, and
propose image/volume parents mode 0710, immutable images mode 0440 and private
volumes mode 0600. These modes appear in the proposal; the commands do not apply
them to installed storage.

`prepare` writes create-only `guest-storage-intent.json` in the private journal.
An exact retry preserves the existing inode. Conflicting, partial or ambiguous
records are preserved and refused; do not delete an intent to bypass refusal.
`check` retains the original descriptor while repeating live identity, range and
device observations, then verifies its bytes and pathname identity again.
Successful JSON reports `intentCommitted` for preparation or `intentValid` for
checking, while `policyPublished`, `servicesActivated` and
`activationQualified` remain false.

These commands are preparation tools, not a completed storage migration. The
installer still needs durable allocation exclusion, a retained runtime activation
barrier, coordinated storage ownership and runtime policy publication, and native
application launch/restart/isolation qualification. A dormant-service snapshot or
an empty guest cgroup observation cannot authorize ownership changes. The check
is also unsuitable for auditing a running pool: its guest processes intentionally
occupy leased UIDs. See [guest launch research](GUEST_DAC_LAUNCH_RESEARCH.md) and
the [product acceptance matrix](PRODUCT_ACCEPTANCE_MATRIX.md) for the remaining
qualification scope.

## Recovery service quiescence

`sudo homenode recovery-quiesce` operates on an existing recovery installation
with its durable activation marker already present. It accepts `--journal-dir`,
requires owned installed configuration and vacant restore destinations, verifies
the manager's loaded unit identity and activation conditions, then synchronously
stops the fixed application services/socket. It checks dormancy and guest
subtree emptiness afterward. A remaining libvirt guest causes refusal rather
than an arbitrary PID kill.

This command leaves the activation marker in place on success or failure. Its
JSON reports current dormancy/emptiness observations and explicitly reports no
publication authorization, migration or activation release. Restored-file
publication and the retained migration transaction remain unfinished; native
qualification of this command is pending.
