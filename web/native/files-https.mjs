// Disposable Linux qualification client. No dev server or intercepted API.
import { chromium, expect } from '@playwright/test'
import { randomBytes, createHash } from 'node:crypto'
import { readFile, realpath, stat } from 'node:fs/promises'
import { dirname } from 'node:path'

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
let phase = 'launch'
const responses = []
const deadline = setTimeout(() => {
  console.error('Native browser workflow exceeded its whole-run deadline')
  void browser?.close().finally(() => process.exit(1))
}, 300000)
try {
  browser = await chromium.launch({
    executablePath: browserExecutable, chromiumSandbox: true, headless: true, timeout: 30000,
    args: ['--host-resolver-rules=MAP home.example.ts.net 100.100.1.2,MAP wrong.example.ts.net 100.100.1.2', '--no-proxy-server'],
  })
  phase = 'launch-policy'
  const session = await browser.newBrowserCDPSession()
  const { arguments: arguments_ } = await session.send('Browser.getBrowserCommandLine')
  if (arguments_.some(argument => /^(--no-sandbox|--disable-setuid-sandbox|--ignore-certificate-errors(?:-spki-list)?|--allow-insecure-localhost)(=|$)/.test(argument))) {
    throw new Error('Browser qualification cannot disable sandbox or certificate checks')
  }
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
  const bytes = randomBytes(32768), name = 'native-browser-upload.bin'
  const checksum = createHash('sha256').update(bytes).digest('hex')
  phase = 'upload'
  await page.getByLabel('Upload a file', { exact: true }).setInputFiles({ name, mimeType: 'application/octet-stream', buffer: bytes })
  await expect(page.locator('.transfer-progress')).toContainText('Upload verified', { timeout: 60000 })
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
  if (!downloadedInfo.isFile() || downloadedInfo.size !== bytes.length) throw new Error('Native browser download has an unexpected size')
  const received = await readFile(path)
  if (received.length !== bytes.length || createHash('sha256').update(received).digest('hex') !== checksum) {
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
  await page.locator('nav').getByRole('button', { name: 'Apps', exact: false }).click()
  phase = 'guest-stop'
  page.once('dialog', dialog => dialog.accept())
  await app.getByRole('button', { name: 'Stop app', exact: true }).click()
  await expect(app.getByText('stopped', { exact: true })).toBeVisible({ timeout: 120000 })
  expect(errors).toEqual([])
  console.log(`PASS native browser HTTPS Files: TLS 1.3, virtual passkey, session inventory, signed start/stop, 32768-byte upload/download, Trash/restore and narrow viewport; browser=${browser.version()}`)
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
  try { await browser?.close() } catch { console.error('Native browser cleanup failed'); process.exitCode = 1 }
}
