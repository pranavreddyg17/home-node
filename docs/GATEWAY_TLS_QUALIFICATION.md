# Gateway TLS qualification boundaries

The gateway must establish both a valid public TLS connection and an
authenticated private Unix connection to the controller. The controller still
enforces its own sessions, capabilities and origin checks after transport
mediation.

The first-party browser guard accepts absent fetch metadata for native clients,
or a single `same-origin`/`none` value. It refuses `same-site` because a sibling
site can still be a different origin, and rejects unknown or repeated fetch
metadata and repeated Origin headers. POST requests still require the exact
configured Origin. These headers are request-context checks, not authentication;
native requests still need the normal session and capability checks.

| Check | Fixture | Scope and limits |
| --- | --- | --- |
| Distinct kernel identities and serving-peer pinning | `gatewaytransport.TestRootDistinctControllerGatewayPeers` | Disposable Linux child processes for controller UID 991, gateway UID 992 and foreign UID 994. Earlier retained evidence uses simulated TLS state; the new retained run also covers real fixture TLS. |
| Actual certificate-verified TLS 1.3 through distinct peers | The same root fixture's `gateway-tls` child | New real loopback handshake, trusted fixture certificate, production proxy and private controller transport. TLS 1.2 is refused. Passed in focused run 38846425818 at source e1810b8. |
| Invalid certificate and HTTP origin | The same child's negative cases | Untrusted roots, wrong certificate server name and foreign HTTP Host must fail. Passed in the same focused native run. |
| Production controller session and capability checks | `control.TestProductionControllerAuthorizationOverRealGatewayTLS` | New unprivileged same-UID Linux fixture checks health, unauthenticated Files refusal, seeded Files-only access, device-administration refusal and revocation on a reused TLS connection, with HSTS and no-store responses. Passed in the same focused native run. |

Both real-TLS fixtures preserve the connection state supplied by Go's TLS
server. They substitute a synthetic tailnet client address for the loopback
TCP address. Neither validates overlay routing, Tailscale enrollment or ACLs.
Their certificates are test fixtures rather than production issuance or
protected certificate-file loading. A seeded session does not qualify real
WebAuthn enrollment. Same-UID controller authorization and distinct-UID
transport mediation are separate tests, not combined installed-process proof.

Installed gateway/controller executables, certificate rotation, systemd
confinement, real private-network devices, passkeys, revocation during live
streams and physical-host operation still need end-to-end evidence. Passing
component tests does not satisfy those release gates.

The focused `Gateway security boundary` workflow runs these transport,
certificate and controller checks on disposable Ubuntu, including explicitly
opted-in distinct-UID children. It has independent concurrency and does not
cancel earlier runs. It does not install services or replace the full product
checks. The revocation fixture directly updates its seeded device record; it
does not exercise owner approval, active download termination or overlay ACL
propagation.

The workflow also includes a bounded 15-second request-object fuzz run with two
workers and triggers on parser changes. That check exercises ordinary JSON
request parsing; it is not sustained protocol fuzzing, an independent security
review or evidence for the supervisor/guest protocol gates.

[Retained gateway TLS evidence](evidence/gateway-tls-e1810b8.json) records the
source revision, successful workflow/job, log digest, assertions and limits.
This qualifies those component paths at that source only.
