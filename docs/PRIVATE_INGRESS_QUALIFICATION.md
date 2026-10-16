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

A one-second health watcher checks the current interface index, flags and
configured address. An invalid observation closes the listener instead of
retargeting it. The executable's existing shutdown path then exits and the
installed `Restart=on-failure` policy repeats all startup admission. Normal
listener close stops the watcher. Component race tests check that health
failure unblocks Accept and that both failure and normal close stop the
watcher; they do not prove installed systemd or daemon restart behavior.
Accept also discards a connection returned concurrently with a failed health
observation and reports an explicit ingress identity error. A gated real TCP
test holds an accepted connection until after the failed observation to check
this ordering, while ordinary close must remain a normal closed-listener error.

`cmd/homenode-gateway.TestListenerFailureClosesActiveTLSAfterGrace` exercises
the executable's shared serving/shutdown function with certificate-verified
TLS and an active request that waits for cancellation. External listener close
must retain a non-success result for service restart; expiration of the drain
deadline must force server close and cancel that request. The test uses a
20-millisecond grace period; production uses ten seconds. It does not exercise
systemd, real passkeys or the production proxy's workload stream lifecycle.

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
qualification below. Focused Ubuntu run 38940153931 at source be3ab03 passed
the initial namespace fixture and component suite; see
[retained evidence](evidence/private-ingress-be3ab03.json). macOS checks only
compile the Linux-specific test binary.

The focused gateway workflow additionally compiles the test binary and runs
`packaging/inspection/private_ingress_fixture.py` on disposable Ubuntu. Its
explicit root opt-in creates two fresh network namespaces, drops all
capabilities and switches the listener children to UID/GID 65534. Veth
interfaces represent permitted and hostile ingress. The device-bound listener
must accept the permitted path before and after refusing LAN and routed CGNAT
sources; an unbound control must accept all those paths. The namespaces have
no Tailscale firewall rules. Reverse-path filtering is disabled only inside
the fixture namespaces to ensure it cannot hide a missing socket restriction.
This uses the production socket-binding helper with a veth surrogate, not
production TUN/daemon admission, installed services, TLS or actual tailnet ACLs.
The retained initial run proves these socket-helper checks at that source.
The subsequent [source c77a593 run](evidence/private-ingress-c77a593.json)
passed interface recreation and manual listener reopening, as well as the
component suite. The [source 87ba848 run](evidence/private-ingress-87ba848.json)
subsequently passed the watched child's real interface-index observation and
Accept closure. The [source 1950588 run](evidence/private-ingress-1950588.json)
then passed the unfiltered component race suites including the concurrent-accept
guard and executable TLS shutdown test. Real-TUN admission and the later legacy
direct-listener change remain outside that retained source.

The fixture also deletes and recreates its permitted interface, verifies a
different kernel interface index, and repeats the route/control checks. The
old listener must refuse all paths after replacement; a freshly opened bound
listener must restore only permitted ingress. This exercises manual listener
reopening, not automatic gateway restart or real daemon TUN recreation.
An additional watched child accepts permitted traffic before replacement and
must observe the real index change, close its blocked Accept and exit cleanly
within five seconds. It uses the production watcher with a ten-millisecond
fixture interval and index observation on the veth surrogate; it does not
exercise production point-to-point/address admission or installed restart.

The final fixture phase creates a real kernel TUN named `tailscale0`, assigns
the configured address and starts `ListenTailnet` as the same capability-free
child. Removing that address must close its blocked Accept through the
production one-second interface watcher; a fresh invocation must refuse the
missing address with `ErrIdentity`. This phase requires `/dev/net/tun` in the
disposable runner. It does not transmit packets through the TUN, run a real
Tailscale daemon, load certificates or start installed services. The earlier
veth phase remains the ingress packet-path evidence. Native qualification for
this phase passed in [source 6c3166b's native run](evidence/private-ingress-6c3166b.json).
That evidence does not include the later source-systemd confinement phase.

An additional systemd phase copies the gateway source unit's hardening
properties, including its empty capability sets, address-family restriction,
private devices, filesystem/namespace protections and resource limits. It
starts the production-listener test child as UID/GID 65534 inside the fixture
network namespace, with the test binary mounted read-only into `/run` so
`PrivateTmp` remains enabled. TUN admission and address-loss shutdown must
still work. The fixture stops any attempted transient unit before namespace
cleanup. This uses a test executable and explicit namespace/binary bindings;
it excludes installed NSS identities, proxy group, protected certificate
loading, controller mediation and actual gateway service restart. Native
execution of this confinement phase passed in
[source d28bd1d's run](evidence/private-ingress-d28bd1d.json), retaining the
kernel/environment and source-service digest. This still does not qualify
the installed executable or coordinated service lifecycle. Source gateway unit changes
trigger the focused workflow so changes to those protections are retested.

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
