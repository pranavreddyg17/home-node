# Workload client peer authentication

The controller authenticates the supervisor listener as UID 0 and the transfer
listener as the observed `homenode-transfer` account UID. The transfer service
authenticates its supervisor listener as UID 0. Each new Unix connection checks
Linux kernel peer credentials before HTTP sends an operation body; existing
connections remain attached to the authenticated listener. Redirects remain
disabled. Missing, root, aliased-to-controller or out-of-range transfer account
identities refuse controller configuration. Relative or noncanonical socket
paths refuse dialing. Non-Linux peer observation cannot silently bypass checks.

These checks authenticate processes in addition to installed directory/socket
permissions. They do not replace protected paths, service isolation, server-side
controller authorization, request validation or catalog/guest policy admission.
They assume the supervisor creates its listener as root and transfer creates its
listener as its dedicated account; changing to socket activation requires an
explicit review of the kernel listener creator identity.

`TestWorkloadClientAuthenticatesListenerBeforeSendingOperation` exercises real
Unix connections: matching listener succeeds, foreign and invalid expected UIDs
receive no HTTP operation, zero transfer UID cannot enable guest calls, and an
unprivileged listener cannot impersonate root. The Linux test is included in the
gateway security workflow. Local race tests pass for runtimeclient and the two
affected commands; Linux test compilation passes. Native CI and real-guest
regression remain required before treating the change as qualified.

The separate backup maintenance client still uses its existing protected socket
transport; this change does not claim to qualify all maintenance boundaries or
the complete authenticated Files workflow.
