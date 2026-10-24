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
