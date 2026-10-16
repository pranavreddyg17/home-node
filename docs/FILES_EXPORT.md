# Export saved Files data

In Files, choose **Export saved files** to download
`homenode-saved-files.zip`. A paired device needs the Files capability. Private
Files must be running when there are saved objects to read.

The archive includes the saved-file list observed when export starts. Trash and
unfinished uploads are excluded. Restore anything wanted from Trash first and
finish any desired uploads before exporting. Avoid changing or trashing files
during the transfer; an unavailable object aborts the archive. New files added
after the manifest snapshot are not included.

Each data entry has a unique numbered filename under `files/`. Reserved path
characters, control/format characters and trailing dots/spaces are replaced or
removed. `manifest.json` preserves original names and IDs and records the size,
SHA-256 and archive path for each file. Duplicate original names remain separate
entries. Verify the manifest and extracted files before relying on the export.

Export uses bounded guest chunks and checks each published whole-file checksum.
Authorization is rechecked between reads and before writes/finalization. Data
entries are stored without recompression to avoid spending CPU on already
compressed media. The server does not build a second archive on its own disk.
It shares the normal download concurrency budget. A corrupt/unavailable read,
revoked session, write failure or cancelled connection aborts HTTP without a
completed ZIP central directory. Retry the export; the archive itself is not
resumable.

The current limit is 10,000 saved entries and 12 GiB of file contents, plus ZIP
and manifest overhead. Exceeding either bound returns `EXPORT_LIMIT` before
archive output rather than exporting a truncated list. Individual downloads
remain available. Ensure the receiving device has enough space.

This is a file export, not a consistent application backup. It contains no AI
history, job records, identity credentials, app images or backup repository.
Tests cover real guest-store bytes through the controller, empty objects,
duplicate names, Trash exclusion, manifest hashes, corruption, revocation,
capacity release and explicit oversize refusal. The browser test downloads an
empty manifest through the real controller. Installed guest/transfer/gateway
services and large physical-device exports remain unqualified.
