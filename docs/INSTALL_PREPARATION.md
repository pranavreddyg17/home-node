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

`sudo homenode guest-identity-prepare` records a dedicated-host identity
configuration proposal after verifying the owned primary and maintenance
accounts. It accepts `--journal-dir` for an existing private journal. The exact
original and desired bytes are retained in `guest-identity-nss-intent.json`;
JSON exposes their hashes and reports `configurationApplied: false`. Preparation
preserves host configuration and refuses remote or ambiguous identity sources.
It does not establish UID allocation exclusivity or enable reserved storage.
`sudo homenode guest-identity-apply` applies that saved proposal to an owned,
empty installation with an existing durable activation block. It accepts the
same journal option; application requires that journal to be the fixed
`/var/lib/homenode-install` directory used by the installed unit conditions.
It validates loaded unit ownership before stopping services,
and retains the activation block while rechecking service dormancy, guest
emptiness and account ownership through publication. It holds the shared
account-writer lock and preserves the original configuration under
`/etc/.homenode-nsswitch.stage`; retries use the recorded file identities.
Success reports `configurationApplied: true`. This command does not reserve
UID ranges, publish runtime policy or release the activation block. Native API
application and packaged CLI retry have been verified in disposable Linux CI;
supported physical-host acceptance remains pending.

`guest-allocation-prepare` records an explicit `login.defs` proposal using the
owned accounts and protected host configuration. All boundaries are required:

```sh
sudo homenode guest-allocation-prepare \
  --first-uid 2000000000 --last-uid 2000000001 \
  --uid-min 1000 --uid-max 60000 \
  --sys-uid-min 100 --sys-uid-max 999 \
  --sub-uid-min 100000 --sub-uid-max 600100000
```

These values are an example explicit policy, not detected host defaults. Choose
ranges for the dedicated host before preparation. The command preserves host
bytes, reports hashes with `configurationApplied: false`, and accepts
`--journal-dir` for an existing private journal.

`sudo homenode guest-allocation-apply` applies the saved allocator proposal.
It accepts only the journal option, requires the fixed installation journal,
strict local NSS, an owned empty installation and its retained activation
block. It preserves the original inode under `/etc/.homenode-login-defs.stage`
and reports `configurationApplied: true` only after qualified publication.
The activation block remains in place. Native application qualification, future
drift enforcement and exclusive reservation remain pending.

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

After preparing storage intent, the image phase is available through:

```sh
sudo homenode guest-storage-migrate-images \
  --publisher-key "$VERIFIED_PUBLISHER_KEY_HEX" \
  --minimum-catalog-version "$VERIFIED_MINIMUM_CATALOG_VERSION"
```

Supply the independently verified publisher Ed25519 public key as 64 hexadecimal
characters and a positive minimum catalog version. The installed key alone is
not a trust source. This command requires an existing durable recovery activation
block, quiesced owned services and vacant control, supervisor and volume
destinations. It retains account allocation exclusion, signed catalog and image
provenance while migrating the three immutable images and publishing the image
parent's ownership in the installation journal. Exact interrupted retries use
the retained migration records; conflicting records are preserved and refused.
The command neither creates nor releases the activation block. Successful JSON
reports `imageOwnershipMigrated` and `parentJournalPublished`; `policyPublished`,
`servicesActivated` and `activationQualified` remain false. Native qualification
of this assembled command remains pending; this phase does not migrate a
populated runtime or publish the guest runtime policy.

These commands do not constitute a completed storage migration. The
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

### Empty volume-parent migration phase

After the signed image ownership and parent-journal phase, an existing blocked
installation can run:

```sh
sudo homenode guest-storage-migrate-empty-volumes \
  --publisher-key <independently-verified-ed25519-public-key-hex> \
  --minimum-catalog-version <independent-positive-floor>
```

The command requires the retained activation block, loaded unit conditions,
dormant services, an empty guest cgroup and vacant runtime destinations. It
selects the old volume group from the authenticated account allocation, binds
an empty volume parent to immutable inode/journal provenance, and publishes its
reserved guest group under retained runtime/account exclusion. Interrupted
ownership and journal steps retry only the recorded inode and journal states.
It refuses populated or foreign directories without deleting their contents.

Success JSON reports `emptyParentOwnershipMigrated` and
`parentJournalPublished`. `populatedVolumesMigrated`, `policyPublished`,
`servicesActivated` and `activationQualified` remain false. This phase does not
prepare runtime channel parents or release activation. Full installed-command
native qualification remains pending; temporary-root publisher tests alone do
not prove that journey.

### Configuration migration phase

The channel parent can first be provisioned under a blocked installation using
`homenode guest-storage-provision-channels` with the same independently verified
`--publisher-key` and positive `--minimum-catalog-version` arguments. The command
creates an absent `/run/homenode` under a retained protected `/run` scope and
requires migrated images and
volume ownership, vacant runtime destinations and dormant services/guests. It
records and publishes only the new empty channel parent, or retries its recorded
inode. Foreign and uncertain partial directories are preserved and refused.
Success reports `channelParentPublished`; service activation remains false.
A prior-boot receipt can be archived only while both channel paths are absent.
Automatic boot-time service composition remains unfinished. Native installed-command qualification remains pending.

Once images, volumes and `/run/homenode/guests` have independently qualified
ownership, an existing blocked installation can publish reserved configuration:

```sh
sudo homenode guest-storage-migrate-configuration \
  --publisher-key <independently-verified-ed25519-public-key-hex> \
  --minimum-catalog-version <independent-positive-floor>
```

The command retains dormant-runtime and account allocation exclusion. It checks
the signed catalog and actual image bytes, captures the journaled policy and
environment, records their transition and stages both replacements. Publication
preflights both recorded file pairs and can resume an interrupted exchange or
journal update. Existing stage receipts must match the recorded inodes; foreign
files and unrecorded partial stages are refused and preserved.

Success reports `policyPublished`, `environmentPublished` and
`configurationJournalPublished`. `servicesActivated` and `activationQualified`
remain false. A missing channel parent is refused; this command does not
provision it or release activation. Native qualification of the complete installed
command remains pending.
