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
