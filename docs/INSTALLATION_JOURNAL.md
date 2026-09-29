# Owned configuration installation

`internal/install` implements the filesystem transaction phase of the planned
installer. It is an internal component, not a complete installation command.
The production orchestrator still needs supported-host eligibility, verified
release/catalog/image admission, service account creation, private-network/TLS
setup, resource configuration and service activation. Input plans must come
from that trusted orchestrator; downloaded plan JSON is not an authority to
write configuration.

## Ownership and transaction behavior

The engine opens a protected host root and an existing private, owner-controlled
journal directory by descriptor. The production caller must be root and use
`/` as the host root. A cross-process exclusive lock prevents overlapping
install/rollback. A test caller can use a temporary host root without modifying
the real operating system.

A plan has at most 32 items, each confined to a fixed set of HomeNode directories
and configuration/unit/catalog paths. Individual file content is bounded to
128 KiB; executable bits and group/other write access are rejected. Files and
protected directories belong to the installer identity; the controller data
directory may have its separate unprivileged owner. TLS private keys, arbitrary
OS files, user content, guest images and general network configuration are not
accepted file destinations by this engine.

Before committing initial intent, an occupied file is refused even when its
content matches. Existing directories are checked and recorded as borrowed;
rollback cannot remove them. Planned parent directories must precede children.
The journal contains paths, ownership, modes and content digests, with no file
content or private keys. Atomic replacement plus file/directory sync commits
journal transitions.

For a new file, a unique journal-owned staging file receives the complete
bounded content, ownership and mode, then is synced. Atomic hard-link creation
publishes the final name without overwriting a competing destination. The parent
directory is synced before the journal records completion. Interrupted staging
is discarded and rebuilt; a fully published pending file must match its planned
content/ownership/mode before resume accepts it. Directory creation similarly
resumes from expected final or private initial attributes. An incompatible plan
cannot take over an existing journal.

Rollback reverses the recorded operations, deleting only files whose content,
ownership and mode still match, and only empty directories that the journal
created. Changed files, symlinks, unsafe ancestors and occupied data directories
stop rollback with a conflict; the engine never recursively deletes owner data
or resets permissions to force deletion. A filesystem operation completed just
before process exit is reconciled on the next run. Services must be stopped by
the orchestrator before rollback; this engine does not control systemd or imply
application-consistent data shutdown.

## Evidence and remaining integration

Tests exercise install/replay, process recreation, plan mismatch, simultaneous
installer denial, borrowed directory retention, foreign and changed file
protection, occupied owner-data preservation, cancellation, symlink/escape
rejection and corrupt/private-journal enforcement. Child processes exit at
incomplete staging, publication and removal boundaries; fresh engine instances
resume or roll back the actual files/journal. These are process-exit tests,
not physical power-loss qualification.

Linux CI additionally runs the installer fixtures as root to check distinct
controller-directory ownership. The fixtures operate only inside temporary
roots and activate no services. Host integration, ownership bootstrap of the
journal itself, account-operation journaling, image placement, updates,
full uninstall/reinstall, systemd behavior and supported-host power-loss tests
remain unfinished.

## Configuration generation

`ConfigurationPlan` builds the concrete Files/AI/Jobs host configuration for
this engine. It takes typed network settings, independent publisher trust and
catalog floor, existing account/group IDs, supervisor policy and measured host
capacity. It verifies the signed catalog and requires all three workloads.
Controller/transfer identities and all private/runtime/QEMU groups must be
separate; policy identities must match the supplied accounts.

The generator checks a 2 GiB host memory reserve, CPU reserve, each image's
resource grant, simultaneous Files/video capacity and disk space for immutable
images, persistent data disks, temporary video disks and the policy's free-space
reserve. Files data must accommodate the application's quota plus 4 GiB of
headroom; video data must accommodate bounded input/output plus 2 GiB headroom.
These are initial profile floors, not measured guest-overhead certification or
physical allocation. Image verification/placement and measured VM overhead
remain activation gates.

Service units are embedded from the reviewed repository sources in the
installer binary. The generated slice's exact memory bytes and CPU quota match
the supervisor budget; there are no arbitrary downloaded unit overrides.
Network values must use a canonical DNS HTTPS origin with its exact
unprivileged port, preventing environment syntax from entering services.env.
Plans contain policy, publisher key, catalog, services environment, units and
owned directory identities. They exclude TLS key contents. JSON previews omit
all file contents and explicitly list the checks still needed before activation.
A root-only fixture applies and rolls back this generated configuration through
the real engine in a temporary host tree; it neither creates accounts nor starts
services.

## Service identity admission

Read-only `sudo homenode accounts-check` inspects actual local identities before
configuration generation/activation. It requires Linux root to read protected
account databases; output contains only numeric IDs and admission status.
It does not create identities or start services.

The initial Ubuntu setup requires dedicated `homenode` and `homenode-transfer`
users and their private groups, plus `homenode-runtime`. Service UIDs and these
three GIDs must be in the system-account range 1–999. `libvirt-qemu` retains its
OS-assigned GID. Each user must have a unique UID, its own primary group,
`/nonexistent` as home, a nologin shell and a locked shadow password. The passwd
entry must use shadow credentials. Supplementary membership is restricted to
the shared runtime group; private groups have no foreign members/primary users,
and the runtime group contains exactly the two service identities. No aliases
may reuse these group IDs. Configuration planning enforces the same numeric
range/separation and policy identity bindings.

Protected bounded passwd/group/shadow snapshots validate these rules. The
supported NSS sources for passwd/group/shadow are local `files`, optionally
followed by `systemd`; remote/compatibility sources are rejected for this initial
profile. Fixed `getent` and `id` commands confirm current name resolution and
memberships with bounded output, fixed environment and deadlines. Password/hash
content is never returned in errors, previews or journals. This check does not
certify arbitrary custom PAM/SSH configuration or firmware/host confinement.

Negative tests cover UID/GID aliases, unlocked/empty passwords, non-shadow
credentials, interactive shells, real homes, private/runtime group sharing,
privileged supplementary membership and remote NSS sources. An opt-in Linux CI
fixture, after package installation, creates the two real system users/groups,
runs the installed CLI, rejects an added privileged membership and verifies the
repair. It deletes only its created fixture identities and starts no HomeNode
services. Ordinary tests and laptop development never run that mutation.

Production account creation and its ownership journal are still unfinished.
The eventual installer must refuse foreign occupied names, record its own
creation before issuing fixed account tools, resume interrupted creation, and
preserve pre-existing identities on rollback. A successful read-only check
cannot substitute for that ownership lifecycle or fresh-host onboarding.

`PlanAccountCreation` now defines the creation intent for the future account
journal. It refuses all occupied service names (including apparently valid
ones), stale memberships that a newly created user could inherit, QEMU group
aliases and exhausted system-ID space. It reserves distinct unused IDs in
100–999, also excluding dangling primary group IDs. Commands use fixed tool
paths and argument vectors, locked-password useradd defaults, nologin shells,
nonexistent homes and an installation marker. The planner changes nothing;
its preview explicitly requires live vacancy confirmation, committed ownership
intent, bounded execution/result verification and retained rollback ownership.
It is not yet a production account-creation command.

Native fixture cleanup checks definite absence before deleting a group because
`userdel` can remove an empty private group when `USERGROUPS_ENAB` is enabled.
Timeout/resolution failures are not treated as absence. See the
[userdel manual](https://www.man7.org/linux/man-pages/man8/userdel.8%40%40shadow-utils.html).
