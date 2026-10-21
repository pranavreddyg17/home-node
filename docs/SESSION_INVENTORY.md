# Active session inventory

Administrators can inspect active application sessions in Devices, alongside
registered devices. `GET /api/v1/sessions` returns device ID, creation time,
last passkey verification, last request activity, absolute expiry and a boolean
identifying the calling session. Times are Unix seconds; the browser displays
local dates. Activity includes background polling and is not proof of physical
presence. Passkeys may sync between physical devices.

The controller requires administrator capability and the identity service
revalidates the live actor within the inventory transaction. Only current-epoch
sessions with an unrevoked device, unexpired absolute lifetime and activity
within 30 minutes are included. Refresh sessions reads a new snapshot.
Unauthenticated callers receive 401; limited devices receive 403. An unavailable
inventory is reported as an error, never an empty success.

No token, token hash, credential ID or reusable session handle is returned.
The response shares the controller's authenticated no-store policy. Device
revocation continues to require fresh signed approval and terminates all of
that device's sessions; this view adds no unsigned mutation. The read is bounded
by the existing enrollment limits of 64 active devices and 20 sessions per
device, and refuses an inconsistent excess inventory.

Controller regression coverage includes live/current session identification,
idle/expired/revoked/old-epoch exclusions, credential redaction, limited and
unauthenticated refusals, and stale administrator withdrawal. The browser
identity workflow checks the view and its refresh after paired-device
revocation. Native HTTPS protocol qualification passed for administrator access and
limited-device refusal at `f483863` in [run 39546374712](https://github.com/pranavreddyg17/home-node/actions/runs/39546374712).
That run uses Go 1.26.9; the patched toolchain requires separate qualification.
Physical mobile passkeys and installed product qualification remain separate.

The implemented inventory contract is recorded in
[OpenAPI 3.1](../contracts/sessions.openapi.json). Other session and device
operations remain outside this contract slice.
