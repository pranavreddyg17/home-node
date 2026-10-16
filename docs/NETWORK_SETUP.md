# Private HTTPS setup and diagnosis

This is the network stage for the dedicated Ubuntu Server host. The resumable
installer, automated certificate issuance/renewal and physical-device acceptance
are not complete. These checks must not be presented as full onboarding success.

## Owner steps

1. Enroll the host and intended phone/computer in the owner's Tailscale network
   through the local console. Do not store a tailnet-admin API token in HomeNode.
2. Choose a stable, non-identifying DNS name. Enable HTTPS certificates for the
   tailnet and obtain the certificate/key for that exact host name using the
   supported `tailscale cert` workflow. Certificate Transparency makes certificate
   names public; keep personal details out of the name.
3. Restrict tailnet policy to the intended owner devices and the chosen HomeNode
   TCP port. Check both an allowed device and a device that must be denied. A
   local interface check cannot establish whether a remote device is authorized.
4. Provision the TLS directory as root-owned and traversable only by the
   dedicated gateway group (`root:homenode-gateway`, mode `0750`). Install `server.crt` as
   root-owned `0644`, and `server.key` as `root:homenode-gateway` `0640`.
   The controller and transfer identities must not belong to the gateway group.
   Use single-link regular files without extended POSIX ACLs; avoid symlinked
   TLS files.
5. Use an unprivileged port such as 8787 and include it in the exact HTTPS origin.
   Run the read-only identity check before starting the gateway:

```sh
sudo homenode network-check \
  --bind 100.100.1.2 \
  --port 8787 \
  --origin https://home.example.ts.net:8787 \
  --tls-cert /etc/homenode/tls/server.crt \
  --tls-key /etc/homenode/tls/server.key
```

Replace the example address/name with this host's actual identity. The command
requires a running local Tailscale client/daemon (`/usr/bin/tailscale`), its
reported host DNS name and assigned IPv4 address to match the configuration,
that address on an active local interface, a matching
certificate/key, a trusted server-auth certificate chain, current validity,
matching DNS name and matching listener/origin ports. It never transmits the key
or emits its contents. The installed `homenode-gateway` runs the same checks before opening its
HTTPS listener and authenticates the private controller socket using kernel
peer credentials. The controller listens only on that Unix socket and cannot
read the gateway key. Legacy direct HTTPS `serve` also checks the identity.

The dedicated production gateway additionally requires the configured IPv4
address on an up, point-to-point kernel `tailscale0` interface. It binds the
listener to that device before opening TCP and verifies the kernel interface
index. Missing interfaces, unsupported socket options and mismatched readback
prevent startup; there is no wildcard fallback. Userspace Tailscale networking
and custom TUN names are unsupported. The read-only `network-check` command
does not open a socket and therefore does not prove this listener enforcement.
Neither command proves effective tailnet ACLs.

If the daemon recreates its interface, the existing listener remains tied to
the old kernel interface index. The gateway checks the interface, index and
configured address every second and closes its listener after an invalid
observation. Its service then restarts and repeats identity/certificate
admission. Installed automatic recovery is not yet qualified; use the local
console to inspect a service that cannot recover. Do not work around startup failure by granting the gateway network
administration capabilities or exposing a LAN listener. See the
[private ingress qualification gate](PRIVATE_INGRESS_QUALIFICATION.md) for the
remaining namespace, installed-service and physical-device evidence.
Failure does not fall back to HTTP.

`identityValid` proves only those checks. Output explicitly reports that tailnet
policy and phone reachability are not verified. From the intended phone, verify
that the exact HTTPS address opens with no certificate warning. From the denied
device, verify access is blocked. Perform the enrollment and sample-job download
checks separately; do not replace them with a successful local check.

## Certificate replacement

Certificates exported by `tailscale cert` require owner-managed renewal.
HomeNode does not yet automate issuance or renewal. Monitor expiry and renew
before it; production startup warns when fewer than 14 days remain.

Stage the renewed certificate/key in the protected directory, validate their
pair and permissions, and replace each final regular file by atomic rename.
The gateway revalidates protected files at most once a minute when a new TLS
handshake arrives. Incomplete, mismatched, untrusted or expired replacements
reject new handshakes; the old identity is not used as fallback after a failed
reload. Repair the pair to recover. Existing TLS connections are not terminated
by replacement. Expiry is checked on every new handshake, including between
reloads. TLS session tickets are disabled so resumption cannot bypass identity
revalidation. Private key files are never copied into controller state or logs.

A failed check gives a bounded diagnosis for address ownership, origin/port,
protected files, key pair or certificate trust. Actual systemd permissions,
private DNS, phone behavior, policy-denial behavior and renewal on the supported
host remain release gates.

References: [Tailscale HTTPS certificates](https://tailscale.com/docs/how-to/set-up-https-certificates),
[Tailscale CLI](https://tailscale.com/docs/reference/tailscale-cli),
[status data definition](https://github.com/tailscale/tailscale/blob/main/ipn/ipnstate/ipnstate.go).

The installed gateway admits at most 64 TCP connections and eight per source
address before TLS parsing. It separately admits at most 64 active HTTP requests
and eight per source address, including multiplexed HTTP/2 streams. Excess
requests receive a non-cacheable 503 response. Active stream slots are released
when the handler returns; idle address entries are removed. These limits protect
capacity but do not replace device authorization or workload-specific quotas.
Native multi-device HTTPS pressure testing remains a release gate.
