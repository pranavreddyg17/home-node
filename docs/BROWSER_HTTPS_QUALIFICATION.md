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

The first native run, 39550613386 at `936e96c`, failed before enrollment because
the cached Chrome executable did not meet the root-owned/non-writable program
admission check. The Node fixture binary passed; the Chrome check refused.
The first repair attempted to reinstall Chrome from the configured package
repository. Run 39551737347 at `f5d74b9` showed that apt could not download
that cached version and left its root-owned executable at mode 777.
The preparation step now locks down the cached program bundle in the disposable
runner, requires a canonical main executable and ordinary sandbox-helper file,
restores the helper's root-owned 4755 mode, and verifies unchanged executable
bytes across the permission changes. It records SHA-256, version and metadata.
The cached image browser is a qualification input, not a newly verified release
package or a claim about the newest maintained browser version.
The ownership check, certificate validation and sandbox requirement remain
unchanged. Neither this earlier failure nor the upload-ID regression is a
successful browser qualification.

Browser failure diagnostics emit only fixed workflow phases, known network error
codes and coarse API status categories. Raw Playwright call logs are excluded
because action failures can contain enrollment input values.

The browser-specific TLS assertion also requires Chrome to reject the same
server certificate under a wrong hostname before enrollment. Its test DNS
mapping reaches the same native gateway; a generic DNS or connection failure
cannot satisfy the check. The parent requires the explicit hostname-refusal
marker as well as the successful UI workflow. Real and effective browser-driver
UIDs must match and be nonzero. This extension is pending native execution.

Run 39554140444 at `5e5abe6` passed executable admission but refused a writable
ancestor of Chrome. The Node hierarchy passed; the browser bundle and
`/opt/google` had already been protected, leaving `/opt` on Chrome's remaining
ancestor chain. Preparation now protects that directory entry as well and runs
both complete program-admission checks before starting the HTTPS fixture.
Ancestor refusals include only the path and ownership/mode metadata. The guard
continues to reject writable program hierarchies.

Browser preparation and its read-only program hierarchy preflight now run
before guest image construction and VM execution. Admission failures therefore
stop before the expensive workload stages; the full native UI and workload
checks remain required afterward.
