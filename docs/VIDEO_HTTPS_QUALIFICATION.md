# Native HTTPS video workflow qualification

Status: image admission, two-guest catalog/policy staging and initial browser
job orchestration implemented; combined build/workflow execution is unfinished. No complete native video
workflow has passed. Existing guest codec tests and controller component tests
do not qualify the combined path.

The disposable root wrapper accepts paired Files/video image manifests only
under explicit video, Files HTTPS and browser opt-ins with bounded 32 KiB Files
setup. Both manifests must validate before either identity is published. Source
and builder revisions must match, image hashes must differ, and profile-specific
overlay hashes remain independently recorded. Each image is subsequently copied
through a retained source descriptor, SHA-256 checked, protected and synced by
the supervisor fixture. These unsigned images and ephemeral catalog signatures
are development inputs, not publisher release artifacts.

The video-specific fixture policy admits two instances, two vCPUs and 2048 MiB
with two distinct reserved guest UID slots. Both images enter the fixture-signed
catalog and protected-storage inventory. Files-only cases retain their existing
single-instance policy. Cleanup must inspect every admitted allocation and
observe exit before discarding storage; a third unexpected allocation fails.

Remaining implementation/qualification: consume paired build artifacts in a
reproducible workflow; demonstrate enrollment, upload, approved conversion and
verified output through the new scenario; exercise cancellation/retry/reconnect;
require explicit job/cleanup markers and read-only journal assertions. Exercise
failure and foreign-guest/host access paths with both guests present. Physical
networking, signed installation and full release gates remain separate.

The initial browser scenario now requests jobs capability only for explicit video
mode. It generates two seconds of bounded test media as the unprivileged browser
account through a fixed, parent-admitted FFmpeg binary, uploads via Files, selects
an approved 720p preset in Jobs, waits for success, downloads and decodes the
output and checks its published checksum. The parent requires an additional
video completion marker. This is implemented syntax/compilation coverage, not
native execution evidence.

Review found an expected lifecycle blocker: `Manager.purge` rejects reserved
UID/volume/channel ownership evidence, while job cleanup still calls that legacy
operation. The scenario requires the cleanup-pending UI state to clear before
emitting its completion marker. It cannot pass solely because conversion and
download succeed. The fix must add inode-qualified, journaled reserved-video
retirement and safe UID release; removing the existing refusal is unacceptable.
The complete video workflow remains unqualified until this and the other gates
are implemented and demonstrated.

The two-guest fixture budget includes the supervisor's required 512 MiB emulator
reserve for each guest: two 512 MiB image profiles therefore require 2048 MiB,
not 1536 MiB. The admission formula is unchanged; the explicit fixture policy
now supplies the full required budget instead of bypassing capacity checks.

Reserved video retirement now has a read-only provenance snapshot. It joins the
stopped video record, immutable lease/pool/group identity, volume inode and size,
channel directory and bounded socket history in one transaction. It rejects
foreign workloads, active/desired-running state, stale request revisions,
maintenance, missing records, mismatched identities/inodes and more than 256
socket records. Errors return no partial receipt. Tests assert that reading a
snapshot creates no runtime operation and changes no instance state.

This snapshot grants no unlink, ownership-change, restart or UID-release
authority. The live stopped/process barrier, descriptor-qualified deletion,
interruption journal and final lease retirement are still required before the
legacy reserved-resource purge refusal can be replaced.

Retirement preparation now atomically records a bounded canonical intent and its
pending operation identity before any physical effect. Exact replay must match
the original request, ownership snapshot and a recognized pending stage; a
changed request/inode, orphan operation or missing/unknown stage refuses.
Preparation rechecks a caller-supplied live stopped guard and rereads provenance
before returning. Losing that guard retains only the pending journal and returns
no receipt. Tests inject a stage-write failure and require rollback of both the
intent and operation. Preparation preserves the runtime state and UID lease.

This journal is not yet routed from reserved purge and grants no filesystem
mutation authority. Descriptor-qualified socket/volume/directory retirement,
resumption after each physical interruption and final checked UID release remain
unfinished. The legacy refusal remains in place until those steps are proved.

Pending checkpoint advancement now authenticates the original canonical intent,
current stopped provenance, pending operation hash and exact prior stage in one
transaction. Only the next forward edge is allowed; an exact retry can observe
its already-committed target. Neither advancing nor reading repairs missing
records. The live guard is rechecked after the update, followed by fresh journal
validation; a lost guard returns no receipt while preserving the resumable phase.
Tests cover forward/replay, skipped/backward transitions, changed ownership,
missing operation, corrupted intent/stage, update failure and interruption after
commit. Checkpoints cannot publish completion or release the reserved UID.
Physical mutation and final retirement are still not wired to the purge path.

An inode-qualified Linux socket-retirement scope is now implemented. It pins the
root-owned channel parent, recorded guest-owned directory and exact socket with
no-follow descriptors, verifies ownership/modes/mount identity and rejects extra
entries, hardlinks, replacements and quarantine collisions. It checkpoints
before a no-replace quarantine rename, syncs, unlinks only the recorded name,
syncs again and checks retained-inode link count and absence before advancing.
Interrupted checkpoint/rename/unlink cases retain the UID and journal; a socket
already absent is reconciled only under its pending removal checkpoint.

The scope requires `fs.protected_hardlinks=1` throughout its checks, in addition
to a live stopped/UID-process exclusion supplied by the caller. The
[kernel hardlink policy documentation](https://docs.kernel.org/admin-guide/sysctl/fs.html#protected-hardlinks)
and [kernel link admission implementation](https://github.com/torvalds/linux/blob/master/fs/namei.c)
describe the non-owner restrictions for special-file hardlinks. Host root and
configuration remain trusted; no claim is made against malicious root changes.

A separate disposable Ubuntu component workflow exercises real socket inodes,
interruption replay, descriptor closure, foreign entries/replacements/symlinks,
hardlinks and collision preservation. Its explicitly opted-in negative case
temporarily disables the hardlink control, requires refusal without effects and
restores/verifies the original value. This never runs on the owner Mac. The
component injects its stopped guard and does not qualify actual KVM/process
exclusion, volume/directory retirement, UID release or the full video workflow.
The scope is not yet wired into reserved purge. Native component execution passed at `19b735a`; live KVM/process exclusion remains unqualified.

The [disposable Linux component run](https://github.com/pranavreddyg17/home-node/actions/runs/40074693919)
at `19b735a` passed all ten filesystem cases, including interruption at the
checkpoint, rename and unlink boundaries, descriptor closure, foreign-resource
refusal and disabled-hardlink-policy refusal/restoration. [Retained evidence](evidence/video-socket-retirement-19b735a.json)
records exact source, kernel, compiler, log hash and cases. Its injected stopped
guard and retained UID lease make this socket component evidence only.

A separate volume-retirement scope now retains the recorded empty channel and
pins the raw volume with an `O_PATH` descriptor. It authenticates the immutable
intent, inode, size, owner, mode, single link and mount identity before a durable
`volume-removing` checkpoint, no-replace quarantine rename and unlink. Each
boundary rechecks the empty channel, stopped guard and provenance. Interrupted
removal can resume only from its recorded checkpoint; successful replay retains
the channel directory, ownership metadata and UID lease. No guest data is read.

The Linux component fixture uses an opaque synthetic raw file and an injected
stopped guard. It exercises replay, descriptor closure, interruption boundaries,
replacement/collision/symlink/hardlink refusal, changed mode/size, recreated
socket and substituted parent. Local Linux cross-compilation and the full supervisor race suite passed.
[Disposable Linux execution](https://github.com/pranavreddyg17/home-node/actions/runs/40077382129)
at `4747795` passed all twelve volume cases and ten socket regression cases;
[retained evidence](evidence/video-volume-retirement-4747795.json) records source,
environment, cases and log hash. This is not a formatted
guest disk, live process-exclusion, directory-retirement or UID-release proof,
and the reserved purge path continues to refuse destructive cleanup.

The directory-retirement scope now pins the channel parent and recorded empty
directory, checks exact ownership/mode/inode/mount identity and requires both
volume names to remain absent. A durable `directory-removing` checkpoint precedes
a no-replace quarantine rename and `rmdir`, each followed by parent sync and
retained-descriptor checks. Replay accepts absence only under the recorded
removal checkpoint. It retains all provenance and the UID lease. Native tests
cover descriptor closure, each interruption boundary and refusal of replacement,
collision, symlink, changed mode, foreign entry, recreated volume, substituted
parent and unrecorded absence. Disposable Linux execution is pending. This does
not establish live process exclusion or safe UID reuse; reserved purge remains
disabled.
