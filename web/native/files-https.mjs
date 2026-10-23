// Disposable Linux qualification client. No dev server or intercepted API.
import { chromium, expect } from '@playwright/test'
import { randomFillSync, createHash } from 'node:crypto'
import { realpath, stat, mkdtemp, open, rm } from 'node:fs/promises'
import { createReadStream } from 'node:fs'
import { dirname, join } from 'node:path'
import { spawn } from 'node:child_process'
import { tmpdir } from 'node:os'

const origin = 'https://home.example.ts.net:8787'
const browserExecutable = process.argv[2]
if (process.platform !== 'linux' || process.getuid() === 0 || process.geteuid() !== process.getuid() || process.env.HOMENODE_NATIVE_HTTPS_BROWSER !== '1') {
  throw new Error('Explicit disposable Linux browser qualification as a non-root account is required')
}
if (!browserExecutable?.startsWith('/') || await realpath(browserExecutable) !== browserExecutable) {
  throw new Error('A canonical browser executable is required')
}
const executable = await stat(browserExecutable)
if (!executable.isFile() || executable.uid !== 0 || (executable.mode & 0o022) !== 0) {
  throw new Error('Browser executable must be protected by root ownership')
}
for (let parent = dirname(browserExecutable); ; parent = dirname(parent)) {
  const directory = await stat(parent)
  if (!directory.isDirectory() || directory.uid !== 0 || (directory.mode & 0o022) !== 0) throw new Error('Browser executable has an unprotected ancestor')
  if (parent === '/') break
}
const payloadBytes = Number(process.env.HOMENODE_NATIVE_BROWSER_BYTES ?? '32768')
if (![32768, 1073741824].includes(payloadBytes)) throw new Error('Unsupported browser payload size')
const videoWorkflow = process.env.HOMENODE_NATIVE_BROWSER_VIDEO === '1'
if (videoWorkflow && payloadBytes !== 32768) throw new Error('Video workflow requires bounded Files setup')
// The parent admits this fixed root-protected program. No shell or user command
// is involved; generated test media and downloaded output stay in fixture paths.
async function fixtureFFmpeg(arguments_) {
  await new Promise((resolve, reject) => {
    const child = spawn('/usr/bin/ffmpeg', ['-nostdin', '-hide_banner', '-loglevel', 'error', ...arguments_], { env: { PATH: '/usr/bin:/bin', LANG: 'C' }, stdio: ['ignore', 'ignore', 'pipe'] })
    let bytes = 0, failed = false
    const timer = setTimeout(() => { failed = true; child.kill('SIGKILL') }, 30000)
    child.stderr.on('data', chunk => { bytes += chunk.length; if (bytes > 65536) { failed = true; child.kill('SIGKILL') } })
    child.once('error', () => { clearTimeout(timer); reject(new Error('Fixture media executable unavailable')) })
    child.once('close', code => { clearTimeout(timer); if (code !== 0 || failed) reject(new Error('Fixture media operation failed')); else resolve() })
  })
}
// Enrollment material arrives only on an inherited private pipe, never argv,
// URLs, logs, screenshots, traces or persistent browser storage exports.
let input = Buffer.alloc(0)
const inputDeadline = setTimeout(() => process.stdin.destroy(new Error('Invitation handoff timed out')), 30000)
try {
  for await (const chunk of process.stdin) {
    if (input.length + chunk.length > 4096) throw new Error('Invitation handoff is oversized')
    input = Buffer.concat([input, chunk])
  }
} finally { clearTimeout(inputDeadline) }
let invitation
try { invitation = JSON.parse(input.toString('utf8')) } catch { throw new Error('Invalid browser enrollment handoff') }
input.fill(0)
if (!invitation || typeof invitation !== 'object' || Array.isArray(invitation) || Object.keys(invitation).length !== 1 || typeof invitation.invitation !== 'string' || !/^[A-Za-z0-9_-]{20,256}$/.test(invitation.invitation)) {
  throw new Error('Invalid browser enrollment handoff')
}
let browser
let uploadDirectory
let phase = 'launch'
const responses = []
const deadline = setTimeout(() => {
  console.error('Native browser workflow exceeded its whole-run deadline')
  void browser?.close().finally(() => process.exit(1))
}, payloadBytes === 1073741824 ? 600000 : 300000)
try {
  browser = await chromium.launch({
    executablePath: browserExecutable, chromiumSandbox: true, headless: true, timeout: 30000,
    // CDP command-line inspection requires this explicit automation switch.
    args: ['--enable-automation', '--host-resolver-rules=MAP home.example.ts.net 100.100.1.2,MAP wrong.example.ts.net 100.100.1.2', '--no-proxy-server'],
  })
  phase = 'launch-policy'
  phase = 'launch-command-inspection'
  const session = await browser.newBrowserCDPSession()
  const { arguments: arguments_ } = await session.send('Browser.getBrowserCommandLine')
  if (arguments_.some(argument => /^(--no-sandbox|--disable-setuid-sandbox|--ignore-certificate-errors(?:-spki-list)?|--allow-insecure-localhost)(=|$)/.test(argument))) {
    phase = 'launch-forbidden-security-flag'
    throw new Error('Browser qualification cannot disable sandbox or certificate checks')
  }
  phase = 'browser-context'
  const context = await browser.newContext({ ignoreHTTPSErrors: false, acceptDownloads: true, viewport: { width: 1280, height: 900 } })
  const page = await context.newPage()
  page.setDefaultTimeout(15000)
  const errors = []
  page.on('pageerror', () => errors.push('pageerror'))
  page.on('response', response => {
    const url = new URL(response.url())
    if (url.origin !== origin || !url.pathname.startsWith('/api/v1/')) return
    const group = url.pathname.split('/')[3]
    const category = ['auth', 'devices', 'apps', 'operations', 'transfers', 'files', 'sessions', 'session', 'setup'].includes(group) ? group : 'other'
    responses.push(`${category}:${response.status()}`)
    if (responses.length > 8) responses.shift()
  })
  phase = 'virtual-authenticator'
  const authenticator = await context.newCDPSession(page)
  await authenticator.send('WebAuthn.enable')
  await authenticator.send('WebAuthn.addVirtualAuthenticator', { options: {
    protocol: 'ctap2', transport: 'internal', hasResidentKey: true,
    hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true,
  } })
  phase = 'trusted-navigation'
  const navigation = await page.goto(origin, { waitUntil: 'domcontentloaded', timeout: 30000 })
  const security = await navigation.securityDetails()
  if (navigation.status() !== 200 || security?.protocol !== 'TLS 1.3' || !await page.evaluate(() => window.isSecureContext)) {
    throw new Error('Native browser did not establish trusted TLS 1.3 and a secure context')
  }
  phase = 'certificate-hostname-refusal'
  const wrongHost = await context.newPage()
  let hostnameRefused = false
  try {
    await wrongHost.goto('https://wrong.example.ts.net:8787', { waitUntil: 'domcontentloaded', timeout: 15000 })
  } catch (error) {
    hostnameRefused = String(error?.message ?? '').includes('ERR_CERT_COMMON_NAME_INVALID')
  } finally { await wrongHost.close() }
  if (!hostnameRefused) throw new Error('Browser did not refuse the mismatched certificate hostname')
  console.log('PASS native browser refuses certificate hostname mismatch')
  phase = 'enrollment'
  await page.getByRole('button', { name: 'Pair a device', exact: true }).click()
  await page.getByLabel('Device name', { exact: true }).fill('Native browser device')
  await page.getByLabel('Enrollment code', { exact: true }).fill(invitation.invitation)
  invitation.invitation = ''
  await page.getByRole('button', { name: 'Create a passkey', exact: true }).click()
  await expect.poll(async () => await page.getByRole('heading', { name: 'Your home server', exact: true }).isVisible() || await page.getByRole('heading', { name: 'Save these codes', exact: true }).isVisible(), { timeout: 30000 }).toBe(true)
  if (await page.getByRole('heading', { name: 'Save these codes', exact: true }).isVisible()) {
    await page.getByRole('button', { name: 'I saved my recovery codes', exact: true }).click()
  }
  await expect(page.getByRole('heading', { name: 'Your home server', exact: true })).toBeVisible()
  phase = 'session-inventory'
  await page.locator('nav').getByRole('button', { name: 'Devices', exact: false }).click()
  await expect(page.getByRole('region', { name: 'Active sessions', exact: true })).toContainText('This browser session')
  await page.locator('nav').getByRole('button', { name: 'Apps', exact: false }).click()
  const app = page.getByRole('region', { name: 'Private Files', exact: true })
  phase = 'guest-start'
  await app.getByRole('button', { name: 'Start Files', exact: true }).click()
  await expect(app.getByText('running', { exact: true })).toBeVisible({ timeout: 120000 })
  await page.locator('nav').getByRole('button', { name: 'Files', exact: false }).click()
  phase = 'prepare-upload'
  const name = 'native-browser-upload.bin'
  uploadDirectory = await mkdtemp(join(tmpdir(), 'homenode-browser-upload-'))
  const uploadPath = join(uploadDirectory, name)
  const source = await open(uploadPath, 'wx', 0o600)
  const sourceHash = createHash('sha256'), pattern = Buffer.alloc(256 * 1024)
  try {
    for (let offset = 0; offset < payloadBytes; offset += pattern.length) {
      const chunk = pattern.subarray(0, Math.min(pattern.length, payloadBytes - offset))
      // Fresh bytes plus an explicit offset distinguish chunks so reordering
      // cannot hide behind an identical repeated-block test payload.
      randomFillSync(chunk)
      chunk.writeBigUInt64LE(BigInt(offset), 0)
      await source.writeFile(chunk)
      sourceHash.update(chunk)
    }
  } finally { await source.close() }
  const checksum = sourceHash.digest('hex')
  phase = 'upload'
  let creationRequests = 0
  page.on('request', request => {
    if (request.method() === 'POST' && request.url() === `${origin}/api/v1/transfers`) creationRequests++
  })
  const firstChunk = payloadBytes === 1073741824 ? page.waitForResponse(response => {
    const url = new URL(response.url())
    return response.request().method() === 'POST' && response.status() === 200 && url.origin === origin &&
      /^\/api\/v1\/transfers\/[A-Za-z0-9_-]{20,64}\/chunks$/.test(url.pathname)
  }, { timeout: 120000 }) : null
  await page.getByLabel('Upload a file', { exact: true }).setInputFiles(uploadPath)
  if (firstChunk) {
    const acknowledged = await (await firstChunk).json()
    if (!/^[A-Za-z0-9_-]{20,64}$/.test(acknowledged.id) || acknowledged.size !== payloadBytes || acknowledged.sha256 !== checksum || !Number.isSafeInteger(acknowledged.offset) || acknowledged.offset <= 0 || acknowledged.offset >= payloadBytes) {
      throw new Error('Initial large upload acknowledgement is inconsistent')
    }
    phase = 'pause-upload'
    await page.getByRole('button', { name: 'Pause upload', exact: true }).click()
    await expect(page.locator('.transfer-progress')).toContainText('Paused. Select the same file to resume.')
    phase = 'reload-and-resume-upload'
    await page.reload({ waitUntil: 'domcontentloaded' })
    await page.locator('nav').getByRole('button', { name: 'Files', exact: false }).click()
    const unfinished = page.getByRole('region', { name: 'Unfinished uploads', exact: true })
    await unfinished.getByRole('button', { name: `Resume ${name}`, exact: true }).click()
    const resumed = page.waitForResponse(response => response.request().method() === 'GET' && response.status() === 200 && response.url() === `${origin}/api/v1/transfers/${acknowledged.id}`, { timeout: 120000 })
    await page.getByLabel('Upload a file', { exact: true }).setInputFiles(uploadPath)
    const retained = await (await resumed).json()
    if (retained.id !== acknowledged.id || retained.size !== payloadBytes || retained.sha256 !== checksum || !Number.isSafeInteger(retained.offset) || retained.offset < acknowledged.offset || retained.offset >= payloadBytes || retained.state !== 'uploading') {
      throw new Error('Reload did not retain acknowledged upload identity and offset')
    }
  }
  await expect(page.locator('.transfer-progress')).toContainText('Upload verified', { timeout: payloadBytes === 1073741824 ? 360000 : 60000 })
  if (creationRequests !== 1) throw new Error('Browser upload created more than one transfer')
  if (firstChunk) console.log('PASS native browser one GiB pause, reload and resume retains transfer identity and acknowledged offset')
  let row = page.locator('.file-row').filter({ hasText: name })
  await expect(row).toHaveCount(1)
  await row.getByText('Integrity', { exact: true }).click()
  await expect(row.locator('.checksum')).toContainText(checksum)
  phase = 'rename-and-filename-safety'
  const downloadIdentity = await row.getByRole('link', { name: 'Download', exact: true }).getAttribute('href')
  const rename = async nextName => {
    const previous = row
    const prompt = page.waitForEvent('dialog').then(async dialog => {
      if (dialog.type() !== 'prompt' || dialog.message() !== 'New filename') throw new Error('Unexpected rename prompt')
      await dialog.accept(nextName)
    })
    await Promise.all([row.getByRole('button', { name: 'Rename', exact: true }).click(), prompt])
    await expect(previous).toHaveCount(0)
    row = page.locator('.file-row').filter({ hasText: nextName })
    await expect(row).toHaveCount(1)
    await expect(row.getByRole('link', { name: 'Download', exact: true })).toHaveAttribute('href', downloadIdentity)
  }
  await rename('<img data-native-executable src=x onerror=alert(1)>.bin')
  await expect(page.locator('[data-native-executable]')).toHaveCount(0)
  const renamedName = 'native-browser-renamed.bin'
  await rename(renamedName)
  console.log('PASS native browser filename text rendering and rename identity')
  phase = 'narrow-viewport'
  await page.setViewportSize({ width: 390, height: 844 })
  if (!await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)) {
    throw new Error('Native Files view overflows the narrow viewport')
  }
  phase = 'download'
  const downloaded = page.waitForEvent('download', { timeout: 30000 })
  await row.getByRole('link', { name: 'Download', exact: true }).click()
  const download = await downloaded
  if (await download.failure()) throw new Error('Native browser download failed')
  if (download.suggestedFilename() !== renamedName) throw new Error('Native download did not preserve the renamed filename')
  const path = await download.path()
  const downloadedInfo = await stat(path)
  if (!downloadedInfo.isFile() || downloadedInfo.size !== payloadBytes) throw new Error('Native browser download has an unexpected size')
  // Keep verifier memory bounded independently of the selected object size.
  // Check both metadata and the bytes actually read: neither a growing file nor
  // a truncated stream may satisfy the download integrity assertion.
  const receivedHash = createHash('sha256')
  let receivedBytes = 0
  for await (const chunk of createReadStream(path, { highWaterMark: 256 * 1024 })) {
    receivedBytes += chunk.length
    if (receivedBytes > payloadBytes) throw new Error('Native browser download exceeded its expected size')
    receivedHash.update(chunk)
  }
  if (receivedBytes !== payloadBytes || receivedHash.digest('hex') !== checksum) {
    throw new Error('Native browser download failed integrity verification')
  }
  await download.delete()
  phase = 'trash-restore'
  await row.getByRole('button', { name: 'Move to trash', exact: true }).click()
  await expect(row).toHaveCount(0)
  await page.getByRole('button', { name: 'Trash', exact: true }).click()
  await expect(row).toHaveCount(1)
  await row.getByRole('button', { name: 'Restore', exact: true }).click()
  await expect(row).toHaveCount(0)
  await page.getByRole('main').getByRole('button', { name: 'Files', exact: true }).click()
  await expect(row).toHaveCount(1)
  if (videoWorkflow) {
    phase = 'video-input'
    const videoName = 'native-browser-video.mp4', videoPath = join(uploadDirectory, videoName)
    await fixtureFFmpeg(['-f', 'lavfi', '-i', 'testsrc2=duration=2:size=160x90:rate=10', '-an', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-f', 'mp4', videoPath])
    const sourceInfo = await stat(videoPath)
    if (!sourceInfo.isFile() || sourceInfo.size < 32 || sourceInfo.size > 8 * 1024 * 1024) throw new Error('Generated video is outside fixture bounds')
    await page.getByLabel('Upload a file', { exact: true }).setInputFiles(videoPath)
    await expect(page.locator('.transfer-progress')).toContainText('Upload verified', { timeout: 60000 })
    // A previous completed upload can leave that status visible briefly; the
    // new file row must exist before navigation may unmount the upload form.
    await expect(page.locator('.file-row').filter({ hasText: videoName })).toHaveCount(1)
    await page.locator('nav').getByRole('button', { name: 'Jobs', exact: false }).click()
    const input = page.getByLabel('Input file', { exact: true })
    const option = input.locator('option').filter({ hasText: videoName })
    await expect(option).toHaveCount(1)
    const inputID = await option.getAttribute('value')
    if (!/^[A-Za-z0-9_-]{20,64}$/.test(inputID)) throw new Error('Uploaded video input identity is unavailable')
    await input.selectOption(inputID)
    await page.getByLabel('Conversion preset', { exact: true }).selectOption('mp4-720p')
    phase = 'video-job'
    await page.getByRole('button', { name: 'Queue conversion', exact: true }).click()
    const job = page.locator('.file-row').filter({ hasText: videoName })
    await expect(job).toHaveCount(1)
    await expect(job.getByText('succeeded', { exact: true })).toBeVisible({ timeout: 120000 })
    const resultLink = job.getByRole('link', { name: 'Download result', exact: true })
    const resultHref = await resultLink.getAttribute('href')
    if (!/^\/api\/v1\/files\/[A-Za-z0-9_-]{20,64}\/download$/.test(resultHref)) throw new Error('Video output identity is unavailable')
    phase = 'video-output'
    const pendingDownload = page.waitForEvent('download', { timeout: 30000 })
    await resultLink.click()
    const resultDownload = await pendingDownload
    if (await resultDownload.failure()) throw new Error('Video output download failed')
    const resultPath = await resultDownload.path(), resultInfo = await stat(resultPath)
    if (!resultInfo.isFile() || resultInfo.size < 32 || resultInfo.size > 8 * 1024 * 1024) throw new Error('Video output is outside fixture bounds')
    const hash = createHash('sha256'); let length = 0
    for await (const chunk of createReadStream(resultPath, { highWaterMark: 256 * 1024 })) {
      length += chunk.length
      if (length > 8 * 1024 * 1024) throw new Error('Video output exceeded fixture bounds')
      hash.update(chunk)
    }
    if (length !== resultInfo.size) throw new Error('Video output changed during verification')
    await fixtureFFmpeg(['-i', resultPath, '-f', 'null', '-'])
    const outputHash = hash.digest('hex')
    await page.locator('nav').getByRole('button', { name: 'Files', exact: false }).click()
    const output = page.locator(`.file-row:has(a[href="${resultHref}"])`)
    await expect(output).toHaveCount(1)
    await output.getByText('Integrity', { exact: true }).click()
    await expect(output.locator('.checksum')).toContainText(outputHash)
    await resultDownload.delete()
    console.log('PASS native browser video output decode and published checksum')
    phase = 'video-cleanup'
    await page.locator('nav').getByRole('button', { name: 'Jobs', exact: false }).click()
    await expect(job).toHaveCount(1)
    await expect(job.getByText('succeeded', { exact: true })).toBeVisible()
    await expect(job).not.toContainText('Temporary workload cleanup is pending', { timeout: 30000 })
    console.log('PASS native browser video conversion, decoded output integrity and acknowledged cleanup')
  }
  await page.locator('nav').getByRole('button', { name: 'Apps', exact: false }).click()
  phase = 'guest-stop'
  page.once('dialog', dialog => dialog.accept())
  await app.getByRole('button', { name: 'Stop app', exact: true }).click()
  await expect(app.getByText('stopped', { exact: true })).toBeVisible({ timeout: 120000 })
  expect(errors).toEqual([])
  console.log(`PASS native browser HTTPS Files: TLS 1.3, virtual passkey, session inventory, signed start/stop, ${payloadBytes}-byte upload/download, Trash/restore and narrow viewport; browser=${browser.version()}`)
  await context.close()
} catch (error) {
  // Playwright action errors can include input values. Report only fixed phase,
  // known network codes and coarse HTTP statuses, never its raw call log.
  const known = ['ERR_CERT_AUTHORITY_INVALID', 'ERR_CERT_COMMON_NAME_INVALID', 'ERR_SSL_PROTOCOL_ERROR', 'ERR_CONNECTION_REFUSED', 'ERR_NAME_NOT_RESOLVED', 'ERR_ABORTED']
  const code = known.find(value => String(error?.message ?? '').includes(value)) ?? 'assertion-or-runtime-failure'
  console.error(`Native browser workflow failed phase=${phase}; code=${code}; responses=${responses.join(',')}`)
  process.exitCode = 1
} finally {
  clearTimeout(deadline)
  if (uploadDirectory) { try { await rm(uploadDirectory, { recursive: true }) } catch { console.error('Native upload fixture cleanup failed'); process.exitCode = 1 } }
  try { await browser?.close() } catch { console.error('Native browser cleanup failed'); process.exitCode = 1 }
}
