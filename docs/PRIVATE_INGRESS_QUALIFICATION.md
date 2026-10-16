# Private ingress qualification

Status: open release gate. No retained result currently proves the installed
gateway cannot be reached through a non-overlay interface. This gate applies
to the supported dedicated Linux host, independently of application sessions.

`networkcheck.inspectTailnet` checks a running daemon, this node's address and
DNS identity. Certificate admission establishes TLS identity. Neither check
establishes the ingress interface of a subsequent connection or the effective
host firewall. A CGNAT source address is not an authenticated overlay identity.
The real-TLS fixtures substitute their client address and cannot close this gate.

Tailscale documents that its default `on` mode activates rules, including a
drop for CGNAT sources arriving outside `tailscale0`. `nodivert` creates rules
without activating them; `off` does not create them. The installed rule names
alone therefore do not prove enforcement. See [upstream netfilter modes](https://tailscale.com/docs/reference/netfilter-modes).

## Enforcement work

The baseline must enforce overlay-only ingress to the configured HTTPS port
without granting the gateway firewall administration or daemon administration.
Qualify either a kernel-enforced interface restriction on its listener/service
or a separately privileged, narrowly scoped host firewall lifecycle. A startup
declaration that the administrator selected `on` is insufficient evidence.
Do not parse unrestricted daemon debug output into diagnostics or grant the
gateway access to daemon state files to infer firewall configuration.

Admission must reject unavailable enforcement instead of falling back to a
wildcard listener. Define behavior for daemon restart, firewall replacement,
interface disappearance/recreation and host reboot before enabling the
supported deployment claim. Runtime checks must not leave an unbounded period
of exposed service after an enforcement failure. Keep local console recovery.

## Required disposable Linux fixture

Use separate network namespaces and veth paths for the permitted overlay-like
interface and hostile LAN ingress. This fixture must never mutate the owner's
workstation network. Install the actual gateway service and its selected
enforcement mechanism; a handler with a fabricated RemoteAddr is insufficient.
Use an isolated fixture CA and verify TLS hostnames normally.

| Case | Required observation |
| --- | --- |
| Permitted ingress, correct TLS identity | HTTPS health succeeds; unauthenticated private operations still fail |
| LAN source outside CGNAT, route to gateway bind address | No completed TLS connection to the listener |
| LAN ingress with a routable spoofed CGNAT source | No completed TLS connection; positive control proves the test path works when enforcement is deliberately removed |
| IPv6 and alternate local addresses | No additional listener or alternate-interface bypass |
| Tailscale rules absent, inactive or replaced | Independent ingress enforcement still blocks LAN traffic, or service admission refuses to expose HTTPS |
| Overlay interface disappears and is recreated | No LAN exposure; recovery follows the documented identity and interface checks |
| Gateway restart and host boot ordering | Same positive and negative outcomes before and after restart |
| Enforcement unavailable on supported kernel/systemd | Startup fails closed with an actionable local diagnostic |

Retain exact source, OS/kernel/systemd/Tailscale versions, service configuration,
rule/interface observations, command results and log digests. Tests must include
both the negative connection attempt and a working positive path; timeout alone
can also mean a broken fixture. Namespace tests qualify interface enforcement,
not real Tailscale cryptography or effective tailnet policy.

The separate physical-device gate still needs allowed and denied tailnet
devices, real certificates/passkeys, ACL change/revocation, direct and relayed
connections, and daemon restart. Application authentication remains required
even when network admission succeeds.
