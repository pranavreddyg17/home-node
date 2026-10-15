# File download recovery

Authenticated `GET /api/v1/files/{id}/download` supports a single byte range:
`bytes=offset-`, `bytes=start-end`, or `bytes=-suffixLength`. Responses use 206,
the selected Content-Length and Content-Range, and an ETag derived from the
published SHA-256. An exact strong If-Range ETag resumes that representation;
a stale, weak or date validator falls back to the full 200 response. Unknown
range units and multipart ranges are ignored. Unsatisfiable or malformed single
byte ranges return 416. HEAD describes the full representation without a body.
These behaviors follow the relevant [HTTP range and If-Range rules](https://www.rfc-editor.org/rfc/rfc9110.html#section-14.2).

Files remain attachments with sandbox/no-sniff/no-store protections. An approved
Files session is required for metadata and content. Session and Files capability
are checked throughout a download and before releasing its final requested bytes.

The current protocol has one trusted whole-file hash. A partial download therefore
reads and hashes the complete guest object in one pass, retaining only its final
requested piece until the published digest matches. Corruption anywhere in that
object, a guest error or lost permission leaves an incomplete response. Clients
must treat premature EOF as failure and retain the published SHA-256 for complete
file verification. Memory stays bounded by guest chunks; a short range can still
require a full guest-side read. Range requests save client network traffic, not
guest disk work. Trusted per-chunk integrity metadata is future work.

Local HTTP tests cover an actual temporary guest byte store, exact range bytes,
HEAD, invalid ranges, validator fallback, missing authentication, capability loss
and corruption outside the selected range. Installed transfer/gateway services,
mobile download managers, large-file network interruption and physical storage
performance remain separate qualification gates.
