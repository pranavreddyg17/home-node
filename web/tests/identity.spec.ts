import { test, expect, type Page, type Route } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'

async function authenticator(page: Page) {
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('WebAuthn.enable')
  await cdp.send('WebAuthn.addVirtualAuthenticator', { options: {
    protocol: 'ctap2', transport: 'internal', hasResidentKey: true,
    hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true,
  } })
}

test('enroll, pair with limited access, revoke, sign in, and recover', async ({ page, browser }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const output = execFileSync('../bin/homenode', ['setup-code', '--state-dir', process.env.HOMENODE_TEST_STATE!], { encoding: 'utf8' })
  const code = output.trim().split('\n').at(-1)!
  await authenticator(page)
  await page.goto('/')
  await page.getByLabel('Device name', { exact: true }).fill('Owner laptop')
  await page.getByLabel('Enrollment code').fill(code)
  await page.getByRole('button', { name: 'Create a passkey', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Save these codes' })).toBeVisible()
  const recovery = await page.locator('.recovery-codes code').first().textContent()
  await expect(page.locator('.recovery-codes code')).toHaveCount(8)
  await page.getByRole('button', { name: 'I saved my recovery codes' }).click()
  await expect(page.getByRole('heading', { name: 'Your home server' })).toBeVisible()
  await expect(page.locator('.check-row')).toHaveCount(11)
  await expect(page.getByText('Kernel safe path resolution',{exact:true})).toBeVisible()
  await expect(page.getByText('Kernel mount identity',{exact:true})).toBeVisible()
  await expect(page.getByText('Pinned socket permissions',{exact:true})).toBeVisible()
  await expect(page.getByRole('heading', { name: 'External backup', exact: true })).toBeVisible()
  await expect(page.getByText('No acknowledged external backup is recorded.')).toBeVisible()
  await expect(page.getByText('Weekly backup reminder: create your first external backup.')).toBeVisible()
  // Transport/UI fixture: mocked backup operations do not prove disk backup.
  const repositoryId = 'a'.repeat(64)
  let approvalBody = '', approvalKey = '', backupRequests = 0
  let backupPaused = false, configurationFailed = false
  let backupUnavailable = false
  await page.route('**/api/v1/backups/configuration', route => configurationFailed ? route.fulfill({status:503,json:{error:{code:'UNAVAILABLE',message:'Fixture availability unavailable'}}}) : route.fulfill({ json: { enabled: true, repositoryId, availability: backupUnavailable ? 'unavailable' : backupPaused ? 'paused' : 'available' } }))
  await page.route('**/api/v1/backups/approval', async route => {
    approvalBody = route.request().postData()!; approvalKey = route.request().headers()['idempotency-key']
    expect(JSON.parse(approvalBody)).toEqual({ repositoryId })
    expect(approvalBody).not.toContain('fixture-password')
    await route.fulfill({ json: { options: { publicKey: { challenge: 'YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo', rpId: 'localhost', userVerification: 'required', timeout: 10000 } }, challengeToken: 'backup-fixture-challenge' } })
  })
  await page.route('**/api/v1/auth/approval/finish', async route => {
    expect(route.request().headers()['x-approval-challenge']).toBe('backup-fixture-challenge')
    expect(JSON.parse(route.request().postData()!).type).toBe('public-key')
    await route.fulfill({ json: { approvalToken: 'backup-fixture-grant' } })
  })
  await page.route('**/api/v1/backups', async route => {
    backupRequests++
    const headers = route.request().headers(), bytes = route.request().postDataBuffer()!
    expect(headers['content-type']).toBe('application/vnd.homenode.backup-credential')
    expect(headers['idempotency-key']).toBe(approvalKey)
    expect(headers['x-action-approval']).toBe('backup-fixture-grant')
    const length = bytes.readUInt32BE(0)
    expect(bytes.subarray(4, 4 + length).toString()).toBe(approvalBody)
    expect(bytes.subarray(4 + length).toString()).toBe('fixture-password-π')
    await route.fulfill({ status: 202, json: { jobId: 'backup-fixture-job', status: 'preparing' } })
  })
  await page.locator('nav').getByRole('button', { name: 'Settings', exact: false }).click()
  await expect(page.getByText('No acknowledged external backup is recorded.')).toBeVisible()
  await page.getByLabel('Backup reminder interval (days)').fill('14')
  await page.getByRole('button', { name: 'Save reminder interval' }).click()
  await expect(page.getByText('Backup reminder interval saved.')).toBeVisible()
  expect(await page.evaluate(async () => (await (await fetch('/api/v1/backups/outcomes')).json()).reminder.intervalDays)).toBe(14)
  await page.locator('nav').getByRole('button', { name: 'Overview', exact: false }).click()
  await page.locator('nav').getByRole('button', { name: 'Settings', exact: false }).click()
  await expect(page.getByLabel('Backup reminder interval (days)')).toHaveValue('14')
  await page.getByLabel('Repository password').fill('fixture-password-π')
  await page.getByRole('button', { name: 'Verify passkey and start backup' }).click()
  await expect(page.getByText('Backup job backup-fixture-job started.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Repository password')).toHaveValue('')
  expect(backupRequests).toBe(1)
  await page.evaluate(() => Object.defineProperty(navigator.credentials, 'get', { configurable: true, value: async () => { throw new DOMException('Fixture cancelled', 'NotAllowedError') } }))
  await page.getByLabel('Repository password').fill('cancelled-secret')
  await page.getByRole('button', { name: 'Verify passkey and start backup' }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByLabel('Repository password')).toHaveValue('')
  expect(backupRequests).toBe(1)
  await page.evaluate(() => Reflect.deleteProperty(navigator.credentials, 'get'))
  await page.getByLabel('Repository password').fill('refresh-cleared-secret')
  backupPaused = true
  await page.getByRole('button', {name:'Check backup availability'}).click()
  await expect(page.getByText('New backups are paused while work or maintenance is active.',{exact:false})).toBeVisible()
  await expect(page.getByLabel('Repository password')).toHaveCount(0)
  backupPaused = false
  await page.getByRole('button', {name:'Check backup availability'}).click()
  await expect(page.getByLabel('Repository password')).toHaveValue('')
  expect(backupRequests).toBe(1)
  await page.getByLabel('Repository password').fill('unavailable-cleared-secret')
  backupUnavailable = true
  await page.getByRole('button', {name:'Check backup availability'}).click()
  await expect(page.getByText('Backup availability could not be verified.',{exact:false})).toBeVisible()
  await expect(page.getByLabel('Repository password')).toHaveCount(0)
  backupUnavailable = false
  await page.getByRole('button', {name:'Check backup availability'}).click()
  await expect(page.getByLabel('Repository password')).toHaveValue('')
  await page.getByLabel('Repository password').fill('failed-check-cleared-secret')
  configurationFailed = true
  await page.getByRole('button', {name:'Check backup availability'}).click()
  await expect(page.getByRole('alert').filter({hasText:'Fixture availability unavailable'})).toBeVisible()
  await expect(page.getByLabel('Repository password')).toHaveCount(0)
  expect(backupRequests).toBe(1)
  configurationFailed = false
  await page.getByRole('button', {name:'Check backup availability'}).click()
  await expect(page.getByLabel('Repository password')).toHaveValue('')
  const snapshotIds = Array.from({ length: 25 }, (_, index) => index.toString(16).padStart(64, '0'))
  let snapshotRequests = 0
  await page.route('**/api/v1/backups/snapshots', async route => {
    const data = route.request().postDataBuffer()!
    const length = data.readUInt32BE(0)
    const selection = JSON.parse(data.subarray(4, 4 + length).toString())
    expect(data.subarray(4 + length).toString()).toBe('selector-fixture-password')
    expect(route.request().url()).not.toContain('password')
    expect(route.request().headers()['content-type']).toBe('application/vnd.homenode.backup-credential')
    snapshotRequests++
    expect(selection.cursor).toBe(snapshotRequests === 1 ? '' : snapshotIds[24])
    await route.fulfill({ json: { snapshots: snapshotRequests === 1 ? snapshotIds.map(id => ({ id, createdAt: '2026-01-01T00:00:00Z' })) : [], next: snapshotRequests === 1 ? snapshotIds[24] : '' } })
  })
  await page.getByLabel('Password for snapshot browsing').fill('selector-fixture-password')
  await page.getByRole('button', { name: 'Browse newest snapshots' }).click()
  await expect(page.getByLabel('Password for snapshot browsing')).toHaveValue('')
  await expect(page.getByRole('button', { name: 'Load older snapshots' })).toBeVisible()
  await expect(page.getByText(snapshotIds[0], { exact: true })).toBeVisible()
  let foreignPreview = false, previewRequests = 0
  await page.route('**/api/v1/backups/preview', async route => {
    const data = route.request().postDataBuffer()!
    const length = data.readUInt32BE(0)
    const selection = JSON.parse(data.subarray(4, 4 + length).toString())
    expect(selection.snapshotId).toBe(snapshotIds[0])
    expect(data.subarray(4 + length).toString()).toBe('preview-fixture-password')
    previewRequests++
    await route.fulfill({ json: { snapshotId: foreignPreview ? snapshotIds[1] : snapshotIds[0], createdAt: '2026-01-01T00:00:00Z', release: '0.1.0', catalogVersion: 3, validation: 'metadata-compatible', files: [{ workload: 'management', bytes: 1024 }] } })
  })
  await page.getByLabel('Password for snapshot browsing').fill('preview-fixture-password')
  await page.getByRole('button', { name: 'Inspect compatibility' }).first().click()
  await expect(page.getByRole('heading', { name: 'Compatible metadata' })).toBeVisible()
  await expect(page.getByText('Payload integrity and application recovery have not been tested by this preview. No files were restored.')).toBeVisible()
  await expect(page.getByLabel('Password for snapshot browsing')).toHaveValue('')
  foreignPreview = true
  await page.getByLabel('Password for snapshot browsing').fill('preview-fixture-password')
  await page.getByRole('button', { name: 'Inspect compatibility' }).first().click()
  await expect(page.getByRole('alert').filter({ hasText: 'The compatibility preview could not be verified.' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Compatible metadata' })).toHaveCount(0)
  await expect(page.getByLabel('Password for snapshot browsing')).toHaveValue('')
  expect(previewRequests).toBe(2)
  await page.unroute('**/api/v1/backups/preview')
  await page.getByLabel('Password for snapshot browsing').fill('selector-fixture-password')
  await page.getByRole('button', { name: 'Load older snapshots' }).click()
  await expect(page.getByText('No snapshots on this page.')).toBeVisible()
  await expect(page.getByLabel('Password for snapshot browsing')).toHaveValue('')
  expect(snapshotRequests).toBe(2)
  await page.unroute('**/api/v1/backups/snapshots')
  for (const refused of [false, true]) {
  const resumeJob = (refused ? 'B' : 'A').repeat(24)
  let resumeAccepted = false, resumeRequests = 0, resumeKey = ''
  await page.route('**/api/v1/backups/outcomes', route => route.fulfill({ json: {
    schema: 1, current: refused ? null : { status: 'published', publishedAt: 10 }, lastPublished: { status: 'published', publishedAt: 10 }, workerCompletion: refused && !resumeAccepted ? 'refused' : 'complete', resumeJobId: resumeAccepted ? '' : resumeJob, reminder: { state: 'unavailable', intervalDays: 0 },
  } }))
  await page.route(`**/api/v1/backups/${resumeJob}/resume/approval`, async route => {
    expect(route.request().postData()).toBe('{}')
    resumeKey = route.request().headers()['idempotency-key']
    await route.fulfill({ json: { options: { publicKey: { challenge: 'YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo', rpId: 'localhost', userVerification: 'required', timeout: 10000 } }, challengeToken: 'backup-fixture-challenge' } })
  })
  await page.route(`**/api/v1/backups/${resumeJob}/resume`, async route => {
    resumeRequests++
    expect(route.request().postData()).toBe('{}')
    expect(route.request().headers()['idempotency-key']).toBe(resumeKey)
    expect(route.request().headers()['x-action-approval']).toBe('backup-fixture-grant')
    expect(route.request().headers()['content-type']).toBe('application/json')
    resumeAccepted = true
    await route.fulfill({ status: 202, json: { jobId: resumeJob, status: 'restoring' } })
  })
  await page.getByRole('button', { name: 'Refresh backup status' }).click()
  if (refused) {
    await expect(page.getByText('The backup repository was refused before runtime acquisition.', { exact: false })).toBeVisible()
    await expect(page.getByText('The backup worker stopped before runtime acquisition.', { exact: false })).toBeVisible()
    await expect(page.getByText('The backup worker has completed and released its runtime barrier.', { exact: false })).toHaveCount(0)
  }
  await expect(page.getByText('Backup reminder timing is unavailable.', { exact: false })).toBeVisible()
  await expect(page.getByLabel('Backup reminder interval (days)')).toHaveValue('7')
  expect((await (await page.request.get('/api/v1/backups/outcomes')).json()).reminder.intervalDays).toBe(14)
  await page.evaluate(() => Object.defineProperty(navigator.credentials, 'get', { configurable: true, value: async () => { throw new DOMException('Resume cancelled', 'NotAllowedError') } }))
  await page.getByRole('button', { name: 'Verify passkey and resume workloads' }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Resume cancelled' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Verify passkey and resume workloads' })).toBeEnabled()
  expect(resumeRequests).toBe(0)
  await page.evaluate(() => Reflect.deleteProperty(navigator.credentials, 'get'))
  await page.getByRole('button', { name: 'Verify passkey and resume workloads' }).click()
  await expect(page.getByText('Workload restoration started.', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Verify passkey and resume workloads' })).toHaveCount(0)
  expect(resumeRequests).toBe(1)
  await page.unroute('**/api/v1/backups/outcomes')
  await page.unroute(`**/api/v1/backups/${resumeJob}/resume/approval`)
  await page.unroute(`**/api/v1/backups/${resumeJob}/resume`)
  }
  await page.unroute('**/api/v1/backups/configuration')
  await page.unroute('**/api/v1/backups/approval')
  await page.unroute('**/api/v1/auth/approval/finish')
  await page.unroute('**/api/v1/backups')
  await page.locator('nav').getByRole('button', { name: 'Overview', exact: false }).click()
  await page.locator('nav').getByRole('button', { name: 'Settings', exact: false }).click()


  await page.route('**/api/v1/backups/outcomes', route => route.fulfill({ json: {
    schema: 1, current: { status: 'unknown', publishedAt: 0 }, lastPublished: { status: 'published', publishedAt: 10 }, reminder: { state: 'overdue', intervalDays: 7, dueAt: 604810 },
  } }))
  await page.getByRole('button', { name: 'Refresh backup status' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'latest backup outcome is uncertain' })).toBeVisible()
  await expect(page.getByText('Backup reminder: the last acknowledged publication is at least 7 days old.')).toBeVisible()
  await expect(page.getByText('Last acknowledged publication:', { exact: false })).toBeVisible()
  await page.unroute('**/api/v1/backups/outcomes')
  await page.route('**/api/v1/backups/outcomes', route => route.fulfill({ json: {
    schema: 1, current: { status: 'published', publishedAt: 10 }, lastPublished: { status: 'published', publishedAt: 10 }, workerCompletion: 'uncertain',
  } }))
  await page.getByRole('button', { name: 'Refresh backup status' }).click()
  await expect(page.getByRole('status')).toContainText('Backup worker completion is uncertain')
  await expect(page.getByText('Last acknowledged publication:', { exact: false })).toBeVisible()
  await page.unroute('**/api/v1/backups/outcomes')
  await page.route('**/api/v1/backups/outcomes', route => route.fulfill({ status: 503, json: { error: 'unavailable' } }))
  await page.getByRole('button', { name: 'Refresh backup status' }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Backup status is unavailable' })).toBeVisible()
  await expect(page.getByText('Last acknowledged publication:', { exact: false })).toHaveCount(0)
  await page.unroute('**/api/v1/backups/outcomes')
  await page.locator('nav').getByRole('button', { name: 'Overview', exact: false }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.setViewportSize({ width: 1280, height: 900 })
  // UI transport fixture: a passkey-approved command is accepted, while its
  // response stalls. Observing the transition must not submit another command.
  let appPhase = 'absent', appRequests = 0, appApprovalKey = ''
  let heldAppAction: Route | undefined
  await page.route('**/api/v1/apps', route => route.fulfill({ json: { runtimeConfigured: true, apps: appPhase === 'absent' ? [] : [{ workload: 'files', instanceId: '6'.repeat(32), state: appPhase, updatedAt: 1 }] } }))
  await page.route('**/api/v1/apps/files/actions/approval', route => {
    expect(route.request().postDataJSON()).toEqual({ action: 'start' })
    appApprovalKey = route.request().headers()['idempotency-key']
    return route.fulfill({ json: { options: { publicKey: { challenge: 'YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo', rpId: 'localhost', userVerification: 'required', timeout: 10000 } }, challengeToken: 'app-fixture-challenge' } })
  })
  await page.route('**/api/v1/auth/approval/finish', route => {
    expect(route.request().headers()['x-approval-challenge']).toBe('app-fixture-challenge')
    expect(route.request().postDataJSON().type).toBe('public-key')
    return route.fulfill({ json: { approvalToken: 'app-fixture-grant' } })
  })
  await page.route('**/api/v1/apps/files/actions', route => {
    appRequests++; appPhase = 'starting'; heldAppAction = route
    expect(route.request().postDataJSON()).toEqual({ action: 'start' })
    expect(route.request().headers()['idempotency-key']).toBe(appApprovalKey)
    expect(route.request().headers()['x-action-approval']).toBe('app-fixture-grant')
  })
  await page.locator('nav').getByRole('button', { name: 'Apps', exact: false }).click()
  const filesApp = page.getByRole('region', { name: 'Private Files', exact: true })
  await expect(filesApp.getByRole('button', { name: 'Start Files', exact: true })).toBeEnabled()
  // Exercise native signal cancellation quickly while verifying the production
  // HTTP bound is 30 seconds; passkey interaction keeps its own timeout.
  await page.evaluate(() => {
    const original = AbortSignal.timeout.bind(AbortSignal)
    const requested: number[] = []
    Object.assign(window, { approvalTimeouts: requested, restoreApprovalTimeout: () => { AbortSignal.timeout = original } })
    AbortSignal.timeout = milliseconds => { requested.push(milliseconds); return original(milliseconds === 30000 ? 1000 : milliseconds) }
  })
  await filesApp.getByRole('button', { name: 'Start Files', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('The request timed out. Check its current status before retrying.')
  await expect(filesApp.getByText('starting', { exact: true })).toBeVisible()
  appPhase = 'running'
  await expect(filesApp.getByText('running', { exact: true })).toBeVisible()
  expect(appRequests).toBe(1)
  expect((await page.evaluate(() => (window as unknown as { approvalTimeouts: number[] }).approvalTimeouts)).filter(value => value === 30000).length).toBeGreaterThanOrEqual(3)
  await page.evaluate(() => (window as unknown as { restoreApprovalTimeout: () => void }).restoreApprovalTimeout())
  await heldAppAction?.abort().catch(() => {})
  await page.unroute('**/api/v1/apps')
  await page.unroute('**/api/v1/apps/files/actions/approval')
  await page.unroute('**/api/v1/apps/files/actions')
  await page.unroute('**/api/v1/auth/approval/finish')
  await page.locator('nav').getByRole('button', { name: 'Files', exact: false }).click()
  await expect(page.getByRole('heading', { name: 'Your files' })).toBeVisible()
  await page.route('**/api/v1/files', route => route.fulfill({ json: [{ id: '0'.repeat(32), name: 'expired-retention.txt', size: 1, sha256: '1'.repeat(64), createdAt: 1, trashUntil: 0 }] }))
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(page.getByText('expired-retention.txt', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Trash', exact: true }).click()
  await expect(page.getByText('expired-retention.txt', { exact: true })).toBeVisible()
  await expect(page.getByText('Retention expired; removal awaits Private Files.', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Restore', exact: true })).toBeDisabled()
  await expect(page.getByRole('link', { name: 'Download', exact: true })).toHaveCount(0)
  await page.getByRole('main').getByRole('button', { name: 'Files', exact: true }).click()
  await page.unroute('**/api/v1/files')
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  const storagePanel = page.getByRole('region', { name: 'Files storage', exact: true })
  await expect(storagePanel.getByRole('meter', { name: 'Files quota usage' })).toHaveAttribute('max', String(12 * 2 ** 30))
  let storageAvailable = true
  await page.route('**/api/v1/files/storage', route => storageAvailable ? route.fulfill({ json: {
    quotaBytes: 12 * 2 ** 30, storedBytes: 2 ** 30, trashBytes: 128 * 2 ** 20,
    uploadReservedBytes: 256 * 2 ** 20, jobReservedBytes: 512 * 2 ** 20, cleanupReservedBytes: 256 * 2 ** 20,
    chargedBytes: 2 * 2 ** 30, availableBytes: 10 * 2 ** 30,
  } }) : route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Fixture storage accounting unavailable' } } }))
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(storagePanel).toContainText('2.00 GiB of 12.00 GiB charged')
  await expect(storagePanel).toContainText('128.0 MiB in trash')
  await expect(storagePanel).toContainText('Pending cleanup: 256.0 MiB')
  storageAvailable = false
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(storagePanel).toContainText('Storage usage is unavailable.')
  await expect(page.getByRole('heading', { name: 'Your files' })).toBeVisible()
  await page.unroute('**/api/v1/files/storage')
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(storagePanel.getByRole('meter', { name: 'Files quota usage' })).toBeVisible()
  // Recover a server-recorded unfinished upload after reopening the page.
  const recoveredUpload = { id: 'b'.repeat(32), name: 'unfinished.txt', size: 12, offset: 4, sha256: '1'.repeat(64), state: 'uploading', expiresAt: Math.floor(Date.now() / 1000) + 3600 }
  const recoveredKey = `homenode-transfer:${recoveredUpload.sha256}:${recoveredUpload.size}`
  const newerUploadId = 'c'.repeat(32)
  let discardRequests = 0
  await page.evaluate(({ key, id }) => localStorage.setItem(key, id), { key: recoveredKey, id: newerUploadId })
  await page.route('**/api/v1/transfers', route => route.request().method() === 'GET' ? route.fulfill({ json: [recoveredUpload] }) : route.continue())
  await page.route(`**/api/v1/transfers/${recoveredUpload.id}/cancel`, route => {
    discardRequests++; recoveredUpload.state = 'cancelling'
    return route.fulfill({ status: 202, json: recoveredUpload })
  })
  await page.reload()
  await page.locator('nav').getByRole('button', { name: 'Files', exact: false }).click()
  const unfinishedPanel = page.getByRole('region', { name: 'Unfinished uploads', exact: true })
  await expect(unfinishedPanel).toContainText('4 bytes of 12 bytes acknowledged')
  await unfinishedPanel.getByRole('button', { name: 'Discard unfinished.txt', exact: true }).click()
  await expect(unfinishedPanel).toContainText('Cleanup pending')
  await expect(unfinishedPanel.getByRole('button', { name: 'Discard unfinished.txt', exact: true })).toBeDisabled()
  expect(discardRequests).toBe(1)
  expect(await page.evaluate(key => localStorage.getItem(key), recoveredKey)).toBe(newerUploadId)
  await page.evaluate(key => localStorage.removeItem(key), recoveredKey)
  await page.unroute('**/api/v1/transfers')
  await page.unroute(`**/api/v1/transfers/${recoveredUpload.id}/cancel`)
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(unfinishedPanel).toContainText('No unfinished uploads were reported')
  // The creation request committed, but its reply was lost before a transfer
  // bookmark could be saved. Reopening the page must reuse the original key.
  const creationId = 'd'.repeat(32)
  const emptyHash = createHash('sha256').update(Buffer.alloc(0)).digest('hex')
  const creationMarker = `homenode-transfer:${emptyHash}:0`
  let creationKey = '', creationRequests = 0
  let heldCreation: Route | undefined, heldFinalization: Route | undefined
  let finalizationRequests = 0
  await page.route('**/api/v1/transfers', async route => {
    if (route.request().method() === 'GET') return route.continue()
    creationRequests++
    const key = route.request().headers()['idempotency-key']
    const body = route.request().postDataJSON()
    expect(body).toEqual({ name: 'empty-retry.txt', size: 0, sha256: emptyHash })
    if (creationRequests === 1) { creationKey = key; expect(key).toHaveLength(36); heldCreation = route; return }
    expect(key).toBe(creationKey)
    return route.fulfill({ status: 201, json: { id: creationId, name: body.name, size: 0, sha256: emptyHash, offset: 0, state: 'uploading', expiresAt: Math.floor(Date.now() / 1000) + 3600 } })
  })
  await page.route(`**/api/v1/transfers/${creationId}/finalize`, route => {
    if (++finalizationRequests === 1) { heldFinalization = route; return }
    return route.fulfill({ json: { id: creationId } })
  })
  await page.route(`**/api/v1/transfers/${creationId}`, route => route.fulfill({ json: { id: creationId, name: 'empty-retry.txt', size: 0, sha256: emptyHash, offset: 0, state: 'ready', expiresAt: Math.floor(Date.now() / 1000) + 3600 } }))
  await page.getByLabel('Upload a file').setInputFiles({ name: 'empty-retry.txt', mimeType: 'text/plain', buffer: Buffer.alloc(0) })
  await expect.poll(() => Boolean(heldCreation)).toBe(true)
  const creationAborted = page.waitForEvent('requestfailed', { timeout: 3000, predicate: request => request === heldCreation!.request() })
  await page.getByRole('button', { name: 'Pause upload', exact: true }).click()
  await creationAborted
  await expect(page.locator('.transfer-progress')).toContainText('Paused.')
  await heldCreation?.abort().catch(() => {})
  expect(await page.evaluate(key => localStorage.getItem(key), creationMarker)).toBeNull()
  expect(JSON.parse((await page.evaluate(key => localStorage.getItem(key), `${creationMarker}:creation`))!).key).toBe(creationKey)
  await page.reload()
  await page.locator('nav').getByRole('button', { name: 'Files', exact: false }).click()
  await page.getByLabel('Upload a file').setInputFiles({ name: 'renamed-empty.txt', mimeType: 'text/plain', buffer: Buffer.alloc(0) })
  await expect.poll(() => Boolean(heldFinalization)).toBe(true)
  await expect(page.locator('.transfer-progress')).toContainText('Verifying complete file')
  await expect(page.getByRole('button', { name: 'Pause upload', exact: true })).toHaveCount(0)
  const finalizationAborted = page.waitForEvent('requestfailed', { timeout: 3000, predicate: request => request === heldFinalization!.request() })
  await page.locator('nav').getByRole('button', { name: 'Overview', exact: false }).click()
  await finalizationAborted
  await heldFinalization?.abort().catch(() => {})
  expect(await page.evaluate(key => localStorage.getItem(key), creationMarker)).toBe(creationId)
  await page.locator('nav').getByRole('button', { name: 'Files', exact: false }).click()
  await page.getByLabel('Upload a file').setInputFiles({ name: 'renamed-empty.txt', mimeType: 'text/plain', buffer: Buffer.alloc(0) })
  await expect(page.locator('.transfer-progress')).toContainText('Upload verified')
  expect(creationRequests).toBe(2)
  expect(finalizationRequests).toBe(2)
  expect(await page.evaluate(key => localStorage.getItem(key), creationMarker)).toBeNull()
  expect(await page.evaluate(key => localStorage.getItem(key), `${creationMarker}:creation`)).toBeNull()
  await page.unroute('**/api/v1/transfers')
  await page.unroute(`**/api/v1/transfers/${creationId}/finalize`)
  await page.unroute(`**/api/v1/transfers/${creationId}`)
  // Browser transport fixture; native guest and authenticated API have separate evidence.
  const uploadContent = Buffer.from('Resume the original transfer after a temporary outage.')
  const uploadHash = createHash('sha256').update(uploadContent).digest('hex')
  const uploadKey = `homenode-transfer:${uploadHash}:${uploadContent.length}`
  const uploadId = 'a'.repeat(32)
  const pending = { id: uploadId, name: 'resume.txt', size: uploadContent.length, sha256: uploadHash, offset: 7, state: 'uploading', expiresAt: Math.floor(Date.now() / 1000) + 3600 }
  let transferAvailable = false, duplicateTransfers = 0, uploadedOffset = -1, chunkRequests = 0
  let transferMismatch = true
  let transferMissing = false
  let fileListUnavailable = false
  let holdStatus = false
  let heldStatus: Route | undefined
  let heldChunk: Route | undefined
  await page.evaluate(({ key, id }) => localStorage.setItem(key, id), { key: uploadKey, id: uploadId })
  await page.route(`**/api/v1/transfers/${uploadId}`, route => {
    if (holdStatus) { heldStatus = route; return }
    return transferMissing ? route.fulfill({ status: 404, json: { error: { code: 'NOT_FOUND' } } }) : transferAvailable ? route.fulfill({ json: { ...pending, sha256: transferMismatch ? '0'.repeat(64) : uploadHash } }) : route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Fixture transfer service unavailable' } } })
  })
  await page.route('**/api/v1/transfers', async route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: pending.state === 'ready' ? [] : [pending] })
    duplicateTransfers++
    await route.fulfill({ status: 500, json: { error: { message: 'Duplicate transfer fixture' } } })
  })
  await page.route(`**/api/v1/transfers/${uploadId}/chunks`, async route => {
    const body = JSON.parse(route.request().postData()!)
    uploadedOffset = body.offset
    chunkRequests++
    expect(Buffer.from(body.data, 'base64')).toEqual(uploadContent.subarray(7))
    expect(body.sha256).toBe(createHash('sha256').update(uploadContent.subarray(7)).digest('hex'))
    // The guest accepted the bytes but the acknowledgement is lost.
    pending.offset = uploadContent.length
    heldChunk = route
  })
  await page.route('**/api/v1/files', route => fileListUnavailable ? route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Fixture file list temporarily unavailable' } } }) : route.continue())
  await page.route(`**/api/v1/transfers/${uploadId}/finalize`, route => {
    pending.state = 'ready'; fileListUnavailable = true
    return route.fulfill({ json: { id: uploadId } })
  })
  const uploadFile = { name: 'resume.txt', mimeType: 'text/plain', buffer: uploadContent }
  await page.getByLabel('Upload a file').setInputFiles(uploadFile)
  await expect(page.getByRole('alert')).toContainText('Fixture transfer service unavailable')
  expect(await page.evaluate(key => localStorage.getItem(key), uploadKey)).toBe(uploadId)
  expect(duplicateTransfers).toBe(0)
  holdStatus = true
  await page.getByLabel('Upload a file').setInputFiles(uploadFile)
  await expect.poll(() => Boolean(heldStatus)).toBe(true)
  const statusAborted = page.waitForEvent('requestfailed', { timeout: 3000, predicate: request => request === heldStatus!.request() })
  await page.getByRole('button', { name: 'Pause upload', exact: true }).click()
  await statusAborted
  await expect(page.locator('.transfer-progress')).toContainText('Paused.')
  expect(await page.evaluate(key => localStorage.getItem(key), uploadKey)).toBe(uploadId)
  expect(chunkRequests).toBe(0)
  await heldStatus?.abort().catch(() => {})
  holdStatus = false
  transferAvailable = true
  await page.getByLabel('Upload a file').setInputFiles(uploadFile)
  await expect(page.getByRole('alert')).toContainText('The saved upload does not match this file.')
  expect(chunkRequests).toBe(0)
  expect(duplicateTransfers).toBe(0)
  expect(await page.evaluate(key => localStorage.getItem(key), uploadKey)).toBe(uploadId)
  transferMismatch = false
  await page.evaluate(key => localStorage.removeItem(key), uploadKey)
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await unfinishedPanel.getByRole('button', { name: 'Resume resume.txt', exact: true }).click()
  await expect(unfinishedPanel).toContainText('Selected upload: resume.txt')
  await page.getByLabel('Upload a file').setInputFiles({ ...uploadFile, buffer: Buffer.from('wrong file') })
  await expect(page.getByRole('alert')).toContainText('This file does not match the selected upload.')
  expect(chunkRequests).toBe(0)
  expect(duplicateTransfers).toBe(0)
  expect(await page.evaluate(key => localStorage.getItem(key), uploadKey)).toBeNull()
  transferMissing = true
  await page.getByLabel('Upload a file').setInputFiles(uploadFile)
  await expect(page.getByRole('alert')).toContainText('The selected upload is no longer available.')
  expect(chunkRequests).toBe(0)
  expect(duplicateTransfers).toBe(0)
  transferMissing = false
  // Pause while local chunk bytes are still being read, before any POST.
  await page.evaluate(() => {
    const original = Blob.prototype.arrayBuffer
    let reads = 0
    Blob.prototype.arrayBuffer = function () {
      const result = original.call(this)
      if (++reads !== 2) return result
      Blob.prototype.arrayBuffer = original
      return new Promise<ArrayBuffer>(resolve => {
        Object.assign(window, { releaseUploadRead: () => { void result.then(resolve) } })
      })
    }
  })
  await page.getByLabel('Upload a file').setInputFiles(uploadFile)
  await expect.poll(() => page.evaluate(() => 'releaseUploadRead' in window)).toBe(true)
  await page.getByRole('button', { name: 'Pause upload', exact: true }).click()
  await page.evaluate(() => (window as unknown as { releaseUploadRead: () => void }).releaseUploadRead())
  await expect(page.locator('.transfer-progress')).toContainText('Paused. Select the same file to resume.')
  expect(chunkRequests).toBe(0)
  await page.getByLabel('Upload a file').setInputFiles(uploadFile)
  await expect.poll(() => uploadedOffset).toBe(7)
  await page.getByRole('button', { name: 'Pause upload', exact: true }).click()
  await expect(page.locator('.transfer-progress')).toContainText('Paused. Select the same file to resume.')
  expect(await page.evaluate(key => localStorage.getItem(key), uploadKey)).toBe(uploadId)
  await heldChunk?.abort().catch(() => {})
  await page.getByLabel('Upload a file').setInputFiles(uploadFile)
  await expect(page.locator('.transfer-progress')).toContainText('Upload verified')
  await expect(page.getByRole('alert')).toContainText('File list is unavailable.')
  await expect(page.locator('.transfer-progress')).not.toContainText('Upload stopped.')
  await expect(unfinishedPanel.getByRole('button', { name: 'Clear resume selection', exact: true })).toHaveCount(0)
  expect(uploadedOffset).toBe(7)
  expect(chunkRequests).toBe(1)
  expect(duplicateTransfers).toBe(0)
  expect(await page.evaluate(key => localStorage.getItem(key), uploadKey)).toBeNull()
  fileListUnavailable = false
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await page.unroute('**/api/v1/files')
  await page.unroute(`**/api/v1/transfers/${uploadId}`)
  await page.unroute('**/api/v1/transfers')
  await page.unroute(`**/api/v1/transfers/${uploadId}/chunks`)
  await page.unroute(`**/api/v1/transfers/${uploadId}/finalize`)
  await page.locator('nav').getByRole('button', { name: 'Jobs', exact: false }).click()
  await expect(page.getByRole('heading', { name: 'Convert a video' })).toBeVisible()
  const jobInput = { id: 'e'.repeat(32), name: 'fixture-video.mp4', size: 1, sha256: '2'.repeat(64), createdAt: 1, trashUntil: null }
  const queuedJob = { id: '7'.repeat(32), attemptId: 'f'.repeat(32), inputId: jobInput.id, preset: 'mp4-720p', state: 'queued', cleanupPending: false, outputId: null, errorCode: null, createdAt: 1, updatedAt: 1 }
  let jobRequests = 0, jobKey = ''
  await page.route('**/api/v1/files', route => route.fulfill({ json: [jobInput] }))
  await page.route('**/api/v1/jobs', route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: jobRequests ? [queuedJob] : [] })
    jobRequests++
    expect(route.request().postDataJSON()).toEqual({ inputId: jobInput.id, preset: 'mp4-720p', retryGroup: '' })
    const key = route.request().headers()['idempotency-key']
    if (jobRequests === 1) { jobKey = key; return route.abort('connectionreset') }
    expect(key).toBe(jobKey)
    return route.fulfill({ status: 202, json: queuedJob })
  })
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await page.getByRole('combobox', { name: 'Input file', exact: true }).selectOption(jobInput.id)
  await page.getByRole('button', { name: 'Queue conversion', exact: true }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  expect(await page.evaluate(key => Object.values(localStorage).includes(key), jobKey)).toBe(true)
  await page.reload()
  await page.locator('nav').getByRole('button', { name: 'Jobs', exact: false }).click()
  await page.getByRole('combobox', { name: 'Input file', exact: true }).selectOption(jobInput.id)
  await page.getByRole('button', { name: 'Queue conversion', exact: true }).click()
  await expect.poll(() => jobRequests).toBe(2)
  await expect(page.locator('.attempt-label')).toHaveCount(1)
  await expect.poll(() => page.evaluate(key => Object.values(localStorage).includes(key), jobKey)).toBe(false)
  await page.unroute('**/api/v1/files')
  await page.unroute('**/api/v1/jobs')
  await page.locator('nav').getByRole('button', { name: 'AI', exact: false }).click()
  await page.getByRole('button', { name: 'New conversation' }).click()
  await expect(page.getByLabel('Message', { exact: true })).toBeVisible()
  const conversationId = await page.locator('.conversation-picker select').inputValue()
  const retryPrompt = 'Private prompt retained only by the server and current form'
  const completedGeneration = { id: '9'.repeat(32), conversationId, prompt: retryPrompt, output: 'Recovered answer', state: 'succeeded', createdAt: 1 }
  let generationRequests = 0, generationKey = ''
  await page.route('**/api/v1/ai/generations', route => {
    generationRequests++
    expect(route.request().postDataJSON()).toEqual({ conversationId, prompt: retryPrompt })
    const key = route.request().headers()['idempotency-key']
    if (generationRequests === 1) { generationKey = key; return route.abort('connectionreset') }
    if (generationRequests === 2) expect(key).toBe(generationKey)
    else expect(key).not.toBe(generationKey)
    return route.fulfill({ status: 202, json: generationRequests === 2 ? completedGeneration : { ...completedGeneration, id: '8'.repeat(32), output: 'Fresh answer' } })
  })
  await page.route(`**/api/v1/ai/conversations/${conversationId}/history`, route => route.fulfill({ json: generationRequests ? [completedGeneration] : [] }))
  await page.getByLabel('Message', { exact: true }).fill(retryPrompt)
  await page.getByRole('button', { name: 'Send message', exact: true }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  expect(await page.evaluate(key => Object.values(localStorage).includes(key), generationKey)).toBe(true)
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain(retryPrompt)
  await page.reload()
  await page.locator('nav').getByRole('button', { name: 'AI', exact: false }).click()
  await page.locator('.conversation-picker select').selectOption(conversationId)
  await expect(page.getByText('Recovered answer', { exact: true })).toBeVisible()
  await page.getByLabel('Message', { exact: true }).fill(retryPrompt)
  await page.getByRole('button', { name: 'Send message', exact: true }).click()
  await expect.poll(() => generationRequests).toBe(2)
  await expect(page.locator('.chat-message.user-message')).toHaveCount(1)
  await expect(page.getByRole('button', { name: 'Cancel generation', exact: true })).toHaveCount(0)
  await expect.poll(() => page.evaluate(key => Object.values(localStorage).includes(key), generationKey)).toBe(false)
  await page.getByLabel('Message', { exact: true }).fill(retryPrompt)
  await page.getByRole('button', { name: 'Send message', exact: true }).click()
  await expect(page.getByText('Fresh answer', { exact: true })).toBeVisible()
  await expect(page.locator('.chat-message.user-message')).toHaveCount(2)
  expect(generationRequests).toBe(3)
  await page.unroute('**/api/v1/ai/generations')
  await page.unroute(`**/api/v1/ai/conversations/${conversationId}/history`)
  // UI reload fixture only; backend pending-intent reporting has Go coverage.
  await page.route('**/api/v1/ai/conversations', async route => {
    if (route.request().method() !== 'GET') return route.continue()
    return route.fulfill({ json: [{ id: conversationId, title: 'Pending cleanup', createdAt: 1, deletionPending: true }] })
  })
  await page.reload()
  await page.locator('nav').getByRole('button', { name: 'AI', exact: false }).click()
  await page.locator('.conversation-picker select').selectOption(conversationId)
  await expect(page.getByRole('status')).toContainText('Deletion is pending')
  await expect(page.getByLabel('Message', { exact: true })).not.toBeVisible()
  await expect(page.getByRole('button', { name: 'Delete conversation', exact: true })).toBeDisabled()
  await page.unroute('**/api/v1/ai/conversations')
  await page.reload()
  await page.locator('nav').getByRole('button', { name: 'AI', exact: false }).click()
  await page.locator('.conversation-picker select').selectOption(conversationId)
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: 'Delete conversation', exact: true }).click()
  await expect(page.getByLabel('Message', { exact: true })).not.toBeVisible()

  await page.getByRole('button', { name: 'Devices', exact: false }).click()
  await page.getByLabel('Device name', { exact: true }).fill('Paired phone')
  await page.getByLabel('JOBS', { exact: true }).uncheck()
  await page.getByLabel('AI', { exact: true }).uncheck()
  await page.getByRole('button', { name: 'Create invitation' }).click()
  const invitation = await page.locator('.invitation code').textContent()
  const phoneContext = await browser.newContext()
  const phone = await phoneContext.newPage()
  await authenticator(phone)
  await phone.goto('http://localhost:18787/')
  await phone.getByRole('button', { name: 'Pair a device' }).click()
  await phone.getByLabel('Enrollment code').fill(invitation!)
  await phone.getByRole('button', { name: 'Create a passkey', exact: true }).click()
  await expect(phone.getByRole('heading', { name: 'Your home server' })).toBeVisible()
  await expect(phone.getByRole('heading', { name: 'External backup', exact: true })).toHaveCount(0)
  const denial = await phone.evaluate(async () => (await fetch('/api/v1/devices')).status)
  expect(denial).toBe(403)
  await phone.locator('nav').getByRole('button', { name: 'Settings', exact: false }).click()
  await expect(phone.getByRole('heading', { name: 'External backup', exact: true })).toHaveCount(0)
  expect(await phone.evaluate(async () => (await fetch('/api/v1/backups/outcomes')).status)).toBe(403)
  await page.reload()
  await page.getByRole('button', { name: 'Devices', exact: false }).click()
  page.once('dialog', dialog => dialog.accept())
  await page.locator('.device-row').filter({ hasText: 'Paired phone' }).getByRole('button', { name: 'Revoke' }).click()
  await expect(page.locator('.device-row').filter({ hasText: 'Paired phone' })).toContainText('Revoked')
  expect(await phone.evaluate(async () => (await fetch('/api/v1/session')).status)).toBe(401)
  await page.getByRole('button', { name: 'Settings', exact: false }).click()
  await page.getByRole('button', { name: 'Preview report' }).click()
  await expect(page.locator('.diagnostics')).toContainText('[redacted]')
  await page.getByRole('button', { name: 'Sign out', exact: true }).click()
  await page.getByRole('button', { name: 'Sign in with a passkey' }).click()
  await expect(page.getByRole('heading', { name: 'Your home server' })).toBeVisible()
  const recoveryContext = await browser.newContext()
  const recovered = await recoveryContext.newPage()
  await authenticator(recovered)
  await recovered.goto('http://localhost:18787/')
  await recovered.getByRole('button', { name: 'Use a recovery code' }).click()
  await recovered.getByLabel('Recovery code', { exact: true }).fill(recovery!)
  await recovered.getByRole('button', { name: 'Create a passkey', exact: true }).click()
  await expect(recovered.getByRole('heading', { name: 'Save these codes' })).toBeVisible()
  expect(await page.evaluate(async () => (await fetch('/api/v1/session')).status)).toBe(401)
  await recovered.getByRole('button', { name: 'I saved my recovery codes' }).click()
  await expect(recovered.getByRole('heading', { name: 'Your home server' })).toBeVisible()
  expect(errors).toEqual([])
  await phoneContext.close(); await recoveryContext.close()
})
