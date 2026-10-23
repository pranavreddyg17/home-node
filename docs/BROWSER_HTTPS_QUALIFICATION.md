# Browser to native Files qualification

Status: bounded native Files browser workflow passed; full release acceptance remains open. The existing development
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

The UI scenario now includes rename, literal rendering of an executable-looking
filename, preservation of the download object identity across renames, and the
renamed suggested download filename. A marker-bearing HTML-like name must not
create a DOM element; file bytes must still match the original upload. This
extension is pending native execution and is required alongside the TLS refusal
and successful workflow markers before the parent can report a browser pass.

Run 39555866689 at `91acf78` reached the browser but failed within launch-policy
inspection/context/authenticator setup, before navigation or enrollment. The
failure phase is now split into fixed nonsecret subphases to identify the next
failed operation. Run 39556180426 at `92478b4` instead failed before browser
launch: the protocol client retained its session and original transfer identity
across all four service restarts, then Files start failed after three guest
health response timeouts. Neither result qualifies the browser workflow.

Failed joined workflows now collect the allocated fixture domain's state and
reason, process credentials, channel metadata, bounded QEMU log and cgroup
resource counters before stopping the domain. They do not add a guest protocol
connection, change health deadlines, retry the failed workflow, or weaken
sandbox or certificate checks. Counters describe the disposable VM scope and
HomeNode slice; they do not contain uploaded data or enrollment material.

Run 40028388180 at `cd229bf` passed the direct reserved-manager lifecycle and
HTTPS identity/restart workflow, then failed at browser command inspection
before navigation. The bundled Playwright default switches omit
`--enable-automation`, while Chromium's
[CDP contract](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Browser.pdl)
requires it for `Browser.getBrowserCommandLine`. The fixture now supplies that
switch explicitly, retaining command-line security inspection and every
sandbox/TLS refusal. This addresses a confirmed missing prerequisite; native
execution must still establish whether the workflow completes.

Run [40029663221](https://github.com/pranavreddyg17/home-node/actions/runs/40029663221)
at `3d246c0` completed successfully. The joined test took 104.78 seconds and
retained all required browser markers: trusted TLS 1.3, wrong-hostname refusal,
virtual passkey enrollment, session inventory, signed Files start/stop, verified
32 KiB upload/download, literal filename rendering, rename identity, Trash/restore
and narrow viewport. The same run passed actual service identity/confinement,
protocol identity/recovery, all-four-service restart and partial-body revocation
checks. [Retained evidence](evidence/browser-https-files-3d246c0.json) records
source, image, browser, log identity and exact markers. This is the bounded
native browser scenario, not physical-device, 1 GiB browser, signed installed
release or all-workload acceptance.

An explicit large-browser mode is implemented but not yet qualified. Dispatch
with `profile=files`, `files_https_bytes=32768`, `files_browser=true` and
`files_browser_bytes=1073741824`. The protocol setup remains bounded so its
separate capacity-pressure case does not consume the guest space required by
the browser transfer. Selecting the large protocol profile with browser mode
continues to fail explicitly; neither case is silently reduced.

The unprivileged driver writes its generated input into a new private temporary
directory in 256 KiB pieces, hashes incrementally, and supplies a file path to
the actual browser upload control. Download verification also streams in bounded
pieces and checks both metadata length and actual bytes against the expected
SHA-256. Only generated input is removed during cleanup. The parent requires a
marker containing the selected byte count. The large fixture has a ten-minute
browser deadline and corresponding outer process budgets; production deadlines,
health checks, sandbox, TLS and authority checks are unchanged. A completed
transfer alone will not prove interruption/resume or physical-network loss.

The large scenario additionally pauses through the ordinary UI after its first
successful chunk response, reloads the page, selects the unfinished upload and
reselects the original file. It observes the real resumed GET response and
requires unchanged transfer identity/size/hash, a safe retained offset at least
as large as the acknowledged prefix and below the full size, and exactly one
browser transfer-creation request. No API interception or state seeding is used.
The parent requires a separate resume marker for large mode. This coverage is
implemented but still awaits native execution; a pause/reload does not prove
physical network-loss behavior.

Large source generation now refreshes the bounded buffer with random bytes for
each piece and embeds its byte offset in the first eight bytes. Distinct chunk
content makes reordered or misaddressed writes visible to the whole-file hash.
Earlier large runs at `1f36089`/`721f97c` use a repeated random block; any passing
result at those sources must retain that test-data limitation rather than claim
arbitrary chunk-order fault detection.
