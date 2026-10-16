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

## Implemented listener enforcement; qualification pending

The production gateway now uses `networkcheck.ListenTailnet`: it requires an
up, non-loopback, point-to-point `tailscale0` with the configured address,
sets `SO_BINDTODEVICE` before TCP bind/listen, and reads back
`SO_BINDTOIFINDEX` to verify the same interface index. It opens IPv4 only.
Any configuration, interface, socket-option or readback failure prevents the
listener from opening. It does not add capabilities to the service. Custom
TUN names and userspace networking are unsupported by this baseline.

The selected mechanism filters packets at the socket independently of the
Tailscale firewall chains. Linux documents that device-bound sockets process
only packets received on that interface; the supported Linux 6.8 implementation
permits the initial binding of an unbound socket without `CAP_NET_RAW`.
See [socket semantics](https://www.man7.org/linux/man-pages/man7/socket.7.html)
and [kernel implementation](https://github.com/torvalds/linux/blob/v6.8/net/core/sock.c).
This does not defend against a host administrator changing interfaces or
against a compromised gateway process opening additional sockets.

Linux component tests exercise a real loopback-bound listener, readback,
successful accept, nonexistent interfaces and mismatched interface indices.
They do not replace the namespace, interface lifecycle or installed-service
qualification below. No native Linux execution result is retained yet for
this change; macOS checks only compile the Linux test binary.

The baseline must enforce overlay-only ingress to the configured HTTPS port
without granting the gateway firewall administration or daemon administration.
Qualify the kernel-enforced listener restriction and its service lifecycle. A startup
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
