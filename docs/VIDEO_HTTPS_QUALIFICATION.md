# Native HTTPS video workflow qualification

Status: image admission and two-guest catalog/policy staging implemented; public
job and browser orchestration remain unfinished. No complete native video
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

The video-specific fixture policy admits two instances, two vCPUs and 1536 MiB
with two distinct reserved guest UID slots. Both images enter the fixture-signed
catalog and protected-storage inventory. Files-only cases retain their existing
single-instance policy. Cleanup must inspect every admitted allocation and
observe exit before discarding storage; a third unexpected allocation fails.

Remaining implementation: consume paired build artifacts in a reproducible
workflow; enroll a browser device with jobs capability; upload a generated valid
video through Files; queue approved presets through the UI; observe durable job
completion; download and verify output; exercise cancellation/retry/reconnect;
require explicit job/cleanup markers and read-only journal assertions. Exercise
failure and foreign-guest/host access paths with both guests present. Physical
networking, signed installation and full release gates remain separate.
