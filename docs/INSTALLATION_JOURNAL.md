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
