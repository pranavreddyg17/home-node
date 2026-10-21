# Browser to native Files qualification

Status: implemented fixture, native execution pending. The existing development
browser tests intercept workload responses and cannot qualify this path.

The manual guest-image workflow can select `profile=files`,
`files_https_bytes=32768` and `files_browser=true`. This is an explicit bounded
browser case, not a one-GiB browser or physical-phone demonstration. The workflow
requires the runner's protected Chrome executable and stages Node under the
root-owned fixture binary directory. Missing browser or sandbox support fails
qualification; there is no sandbox or TLS bypass.

The actual HTTPS protocol client completes its identity/recovery checks, then
uses fresh signed approval to invite a separate administrator/Files device.
Only the expiring one-use invitation is written to an inherited owned write
pipe. Standard descriptors, regular files, read pipes and invalid bounds are
refused. Session cookies, credential private keys and recovery codes are not
exported for browser enrollment. The invitation is not logged or placed in a
URL, argument, screenshot, trace or file.

The root disposable fixture creates a fresh browser account with no shared
application groups. Its private NSS database adds trust for the fixture peer
certificate; the host trust store and owner browser profiles are untouched.
The browser runs as this unprivileged account in the peer network namespace,
within a bounded transient service that cleans up its child processes. Chrome's
sandbox is explicitly requested. The client rejects sandbox-disabling and
certificate-bypass flags and does not ignore HTTPS errors. The source approach
follows [Playwright's sandbox option](https://playwright.dev/docs/api/class-browsertype#browser-type-launch-option-chromium-sandbox)
and [Chromium's NSS certificate guidance](https://chromium.googlesource.com/chromium/src/+/HEAD/docs/linux/cert_management.md).

Without API interception or a development server, the client requires trusted
TLS 1.3 and a secure context, enrolls a browser passkey, inspects current sessions,
approves Files start, uploads and verifies 32 KiB through the actual guest,
downloads and checks the bytes, moves the file through Trash/restore, and signs
a cooperative Files stop. It also checks a narrow viewport for horizontal
overflow. Passkey interaction uses a CDP virtual authenticator; that is not proof
of physical user verification or mobile platform support. The native supervisor
must still observe the retained instance stopped before fixture cleanup.

Successful evidence must retain source, compiler, frontend and browser versions,
image identity, workload markers and both positive/negative authority assertions.
Root Linux orchestration is never run on the owner Mac. Syntax checks, refusal
checks and Linux compilation alone do not qualify browser execution. Actual
Tailscale enrollment/ACLs, public certificate renewal, physical/mobile passkeys,
one-GiB browser transfers, all application workflows and release gates remain
unverified until their own demonstrations pass.

Review of the native capacity log exposed an integration defect before browser
execution: `state.Random()` uses `crypto/rand.Text()`, while the upload UI's
creation guard accepted only 32 lowercase hex characters. The guard now matches
the guest protocol's bounded resource alphabet instead of assuming UUID/hex
encoding. Browser transport regressions cover a native-format ID completing a
verified upload, plus inconsistent acknowledgement recovery with that format.
The native browser run remains necessary to establish the complete joined path.
