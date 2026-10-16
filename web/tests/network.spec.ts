import { test, expect, type Route } from '@playwright/test'

test('a stalled setup read times out and the owner can reconnect', async ({ page }) => {
  const held: Route[] = []
  let stalled = true
  await page.route('**/api/v1/setup', route => {
    if (stalled) {
      held.push(route)
      return
    }
    return route.fulfill({ json: { claimed: true, development: true } })
  })
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('The server did not respond.', { timeout: 20000 })
  await expect(page.getByRole('alert')).toContainText('private network connection')
  stalled = false
  for (const route of held) {
    // The browser has already aborted these requests after the deadline.
    await route.abort().catch(() => {})
  }
  await page.getByRole('button', { name: 'Retry connection', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Welcome home', exact: true })).toBeVisible()
  await expect(page.getByRole('alert')).toHaveCount(0)
})

test('slow AI status reads do not overlap and polling recovers after refusal', async ({ page }) => {
  const generation = { id: 'fixture-generation', conversationId: 'fixture-chat', prompt: 'Fixture question', output: '', state: 'running', createdAt: 1 }
  let active = 0, maximum = 0, completed = 0
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/setup') return route.fulfill({ json: { claimed: true, development: true } })
    if (path === '/api/v1/session') return route.fulfill({ json: { device: { id: 'fixture-owner', name: 'Fixture owner', capabilities: ['admin', 'ai'], createdAt: 1, revokedAt: null }, verifiedAt: 1, expiresAt: 9999999999 } })
    if (path === '/api/v1/host/report') return route.fulfill({ json: { generatedAt: new Date().toISOString(), host: { os: 'linux', architecture: 'amd64' }, checks: [], prerequisitesMet: false, executionEnabled: false } })
    if (path === '/api/v1/apps') return route.fulfill({ json: { apps: [], runtimeConfigured: false } })
    if (path === '/api/v1/ai/conversations') return route.fulfill({ json: [{ id: 'fixture-chat', title: 'Fixture chat', createdAt: 1, deletionPending: false, deletionRequiresAction: false }] })
    if (path.endsWith('/history')) return route.fulfill({ json: [generation] })
    if (path === '/api/v1/ai/generations/fixture-generation') {
      active++; maximum = Math.max(maximum, active)
      await new Promise(resolve => setTimeout(resolve, 2800))
      completed++; active--
      if (completed === 1) return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Fixture status temporarily unavailable' } } })
      return route.fulfill({ json: { ...generation, output: 'Status recovered' } })
    }
    return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Unused fixture endpoint' } } })
  })
  await page.goto('/')
  await page.locator('nav').getByRole('button', { name: 'AI', exact: false }).click()
  await page.locator('.conversation-picker select').selectOption('fixture-chat')
  await expect(page.getByText('Status recovered', { exact: true })).toBeVisible({ timeout: 12000 })
  await expect(page.getByRole('alert')).toHaveCount(0)
  expect(completed).toBeGreaterThanOrEqual(2)
  expect(maximum).toBe(1)
  let cancelled = false
  page.on('requestfailed', request => { if (new URL(request.url()).pathname === '/api/v1/ai/generations/fixture-generation') cancelled = true })
  await expect.poll(() => active).toBe(1)
  await page.locator('nav').getByRole('button', { name: 'Overview', exact: false }).click()
  await expect.poll(() => cancelled, { timeout: 3000 }).toBe(true)
})

test('AI history failures can be retried and switching conversations aborts obsolete reads', async ({ page }) => {
  let unavailable = true, hold = false, held: Route | undefined, cancelled = false
  let generationCancelled = false, cancellationRequests = 0
  const staging = { id: 'staged-fixture', conversationId: 'first', prompt: '', output: '', state: 'staging', createdAt: 1 }
  page.on('requestfailed', request => { if (new URL(request.url()).pathname === '/api/v1/ai/conversations/first/history') cancelled = true })
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/setup') return route.fulfill({ json: { claimed: true, development: true } })
    if (path === '/api/v1/session') return route.fulfill({ json: { device: { id: 'fixture-owner', name: 'Fixture owner', capabilities: ['admin', 'ai'], createdAt: 1, revokedAt: null }, verifiedAt: 1, expiresAt: 9999999999 } })
    if (path === '/api/v1/host/report') return route.fulfill({ json: { generatedAt: new Date().toISOString(), host: { os: 'linux', architecture: 'amd64' }, checks: [], prerequisitesMet: false, executionEnabled: false } })
    if (path === '/api/v1/apps') return route.fulfill({ json: { apps: [], runtimeConfigured: false } })
    if (path === '/api/v1/ai/conversations') return route.fulfill({ json: ['first', 'second'].map(id => ({ id, title: id, createdAt: 1, deletionPending: false, deletionRequiresAction: false })) })
    if (path === '/api/v1/ai/conversations/first/history') {
      if (hold) { held = route; return }
      if (unavailable) return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Fixture history unavailable' } } })
      return route.fulfill({ json: [{ ...staging, state: generationCancelled ? 'cancelled' : 'staging' }] })
    }
    if (path === '/api/v1/ai/conversations/second/history') return route.fulfill({ json: [] })
    if (path === '/api/v1/ai/generations/staged-fixture') return route.fulfill({ json: { ...staging, state: generationCancelled ? 'cancelled' : 'staging' } })
    if (path === '/api/v1/ai/generations/staged-fixture/cancel') { cancellationRequests++; generationCancelled = true; return route.fulfill({ status: 202, json: { cancellationRequested: true } }) }
    return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Unused fixture endpoint' } } })
  })
  await page.goto('/')
  await page.locator('nav').getByRole('button', { name: 'AI', exact: false }).click()
  const picker = page.locator('.conversation-picker select')
  await picker.selectOption('first')
  await expect(page.getByRole('alert')).toContainText('Conversation history is unavailable.')
  await expect(page.getByText('Ask your first question.', { exact: true })).toHaveCount(0)
  await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Fixture question')
  await expect(page.getByRole('button', { name: 'Send message', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Delete conversation', exact: true })).toBeDisabled()
  hold = true
  await page.getByRole('button', { name: 'Retry history', exact: true }).click()
  await expect.poll(() => Boolean(held)).toBe(true)
  await picker.selectOption('second')
  await expect.poll(() => cancelled, { timeout: 3000 }).toBe(true)
  await expect(page.getByText('Ask your first question.', { exact: true })).toBeVisible()
  await held?.abort().catch(() => {})
  hold = false; unavailable = false
  await picker.selectOption('first')
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(page.getByText('Message submission was interrupted. Cancel this request before sending another message.', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Send message', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Cancel generation', exact: true }).click()
  await expect(page.getByText('cancelled', { exact: true })).toBeVisible()
  expect(cancellationRequests).toBe(1)
  await expect(page.getByRole('button', { name: 'Send message', exact: true })).toBeEnabled()
})

test('AI list refresh failures stay separate from creation and leaving aborts the read', async ({ page }) => {
  let unavailable = true, hold = false, created = false, mutations = 0, held: Route | undefined, cancelled = false
  const conversation = { id: 'created-fixture', title: 'Created fixture', createdAt: 1, deletionPending: false, deletionRequiresAction: false }
  const pendingMarker = 'homenode-conversation-create:fixture-owner'
  page.on('requestfailed', request => { if (new URL(request.url()).pathname === '/api/v1/ai/conversations') cancelled = true })
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/setup') return route.fulfill({ json: { claimed: true, development: true } })
    if (path === '/api/v1/session') return route.fulfill({ json: { device: { id: 'fixture-owner', name: 'Fixture owner', capabilities: ['admin', 'ai'], createdAt: 1, revokedAt: null }, verifiedAt: 1, expiresAt: 9999999999 } })
    if (path === '/api/v1/host/report') return route.fulfill({ json: { generatedAt: new Date().toISOString(), host: { os: 'linux', architecture: 'amd64' }, checks: [], prerequisitesMet: false, executionEnabled: false } })
    if (path === '/api/v1/apps') return route.fulfill({ json: { apps: [], runtimeConfigured: false } })
    if (path.endsWith('/history')) return route.fulfill({ json: [] })
    if (path === '/api/v1/ai/conversations') {
      if (route.request().method() === 'POST') { mutations++; created = true; unavailable = true; return route.fulfill({ status: 201, json: conversation }) }
      if (hold) { held = route; return }
      if (unavailable) return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Fixture list unavailable' } } })
      return route.fulfill({ json: created ? [conversation] : [] })
    }
    return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Unused fixture endpoint' } } })
  })
  await page.goto('/')
  await page.evaluate(marker => localStorage.setItem(marker, JSON.stringify({ key: crypto.randomUUID(), title: 'Unacknowledged fixture' })), pendingMarker)
  await page.locator('nav').getByRole('button', { name: 'AI', exact: false }).click()
  await expect(page.getByRole('alert')).toContainText('Conversation list is unavailable.')
  await expect(page.getByRole('button', { name: 'Retry conversation creation', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Forget pending request', exact: true })).toBeDisabled()
  unavailable = false
  await page.getByRole('button', { name: 'Refresh conversations', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  page.once('dialog', dialog => dialog.dismiss())
  await page.getByRole('button', { name: 'Forget pending request', exact: true }).click()
  expect(await page.evaluate(marker => localStorage.getItem(marker), pendingMarker)).not.toBeNull()
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: 'Forget pending request', exact: true }).click()
  expect(await page.evaluate(marker => localStorage.getItem(marker), pendingMarker)).toBeNull()
  expect(mutations).toBe(0)
  await page.getByRole('button', { name: 'New conversation', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(1)
  await expect(page.getByRole('alert')).toContainText('Conversation list is unavailable.')
  await expect(page.locator('.conversation-picker select')).toHaveValue(conversation.id)
  expect(mutations).toBe(1)
  unavailable = false
  await page.getByRole('button', { name: 'Refresh conversations', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  hold = true
  await page.getByRole('button', { name: 'Refresh conversations', exact: true }).click()
  await expect.poll(() => Boolean(held)).toBe(true)
  await page.locator('nav').getByRole('button', { name: 'Overview', exact: false }).click()
  await expect.poll(() => cancelled, { timeout: 3000 }).toBe(true)
  await held?.abort().catch(() => {})
  hold = false
  await page.locator('nav').getByRole('button', { name: 'AI', exact: false }).click()
  await expect(page.locator('.conversation-picker select')).toBeEnabled()
  await expect(page.getByRole('alert')).toHaveCount(0)
})

test('pending video cancellation retries the same attempt without starting another job', async ({ page }) => {
  let cancellations = 0, newJobs = 0
  const job = { id: 'fixture-group', attemptId: 'fixture-attempt', inputId: 'fixture-input', preset: 'mp4-720p', state: 'cancelling', createdAt: 1, outputId: null, errorCode: null, cleanupPending: false }
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/setup') return route.fulfill({ json: { claimed: true, development: true } })
    if (path === '/api/v1/session') return route.fulfill({ json: { device: { id: 'fixture-owner', name: 'Fixture owner', capabilities: ['jobs'], createdAt: 1, revokedAt: null }, verifiedAt: 1, expiresAt: 9999999999 } })
    if (path === '/api/v1/host/report') return route.fulfill({ json: { generatedAt: new Date().toISOString(), host: { os: 'linux', architecture: 'amd64' }, checks: [], prerequisitesMet: false, executionEnabled: false } })
    if (path === '/api/v1/apps') return route.fulfill({ json: { apps: [], runtimeConfigured: false } })
    if (path === '/api/v1/files') return route.fulfill({ json: [] })
    if (path === '/api/v1/jobs/fixture-group/cancel') {
      cancellations++; expect(route.request().postDataJSON()).toEqual({ attemptId: job.attemptId })
      job.state = 'cancelled'
      return route.fulfill({ status: 202, json: { cancellationRequested: true } })
    }
    if (path === '/api/v1/jobs') {
      if (route.request().method() === 'POST') newJobs++
      return route.fulfill({ json: [job] })
    }
    return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Unused fixture endpoint' } } })
  })
  await page.goto('/')
  await page.locator('nav').getByRole('button', { name: 'Jobs', exact: false }).click()
  await expect(page.getByText('Cancellation is pending. Retrying keeps this attempt.', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Retry cancellation', exact: true }).click()
  await expect(page.getByText('cancelled', { exact: true })).toBeVisible()
  await expect(page.locator('.attempt-label')).toHaveCount(1)
  expect(cancellations).toBe(1); expect(newJobs).toBe(0)
})

test('Jobs refreshes stay serialized when one response fails before the other completes', async ({ page }) => {
  let activeJobs = 0, activeFiles = 0, maxJobs = 0, maxFiles = 0, completedJobs = 0
  let fileAvailable = true
  const inputFile = { id: 'a'.repeat(32), name: 'available.mp4', size: 1, sha256: 'b'.repeat(64), createdAt: 1, trashUntil: null }
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/setup') return route.fulfill({ json: { claimed: true, development: true } })
    if (path === '/api/v1/session') return route.fulfill({ json: { device: { id: 'fixture-owner', name: 'Fixture owner', capabilities: ['admin', 'jobs', 'files'], createdAt: 1, revokedAt: null }, verifiedAt: 1, expiresAt: 9999999999 } })
    if (path === '/api/v1/host/report') return route.fulfill({ json: { generatedAt: new Date().toISOString(), host: { os: 'linux', architecture: 'amd64' }, checks: [], prerequisitesMet: false, executionEnabled: false } })
    if (path === '/api/v1/apps') return route.fulfill({ json: { apps: [], runtimeConfigured: false } })
    if (path === '/api/v1/jobs') {
      activeJobs++; maxJobs = Math.max(maxJobs, activeJobs)
      await new Promise(resolve => setTimeout(resolve, 2800))
      activeJobs--; completedJobs++
      if (completedJobs === 1) return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Fixture jobs temporarily unavailable' } } })
      return route.fulfill({ json: [] })
    }
    if (path === '/api/v1/files') {
      activeFiles++; maxFiles = Math.max(maxFiles, activeFiles)
      await new Promise(resolve => setTimeout(resolve, 3500))
      activeFiles--
      return route.fulfill({ json: fileAvailable ? [inputFile] : [{ ...inputFile, trashUntil: 0 }] })
    }
    return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Unused fixture endpoint' } } })
  })
  await page.goto('/')
  await page.locator('nav').getByRole('button', { name: 'Jobs', exact: false }).click()
  await expect(page.getByText('Reading job status…', { exact: true })).toBeVisible()
  await expect(page.getByText('No jobs yet. Upload a video in Files to get started.', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect.poll(() => completedJobs, { timeout: 5000 }).toBe(1)
  // The failed jobs request must not permit another files read while its
  // original companion response is still pending.
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Job status is unavailable.')
  await expect(page.getByText('Last observed', { exact: false })).toBeVisible({ timeout: 12000 })
  await expect(page.getByRole('alert')).toHaveCount(0)
  expect(completedJobs).toBeGreaterThanOrEqual(2)
  await page.getByRole('combobox', { name: 'Input file', exact: true }).selectOption(inputFile.id)
  await expect(page.getByRole('button', { name: 'Queue conversion', exact: true })).toBeEnabled()
  fileAvailable = false
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(page.getByRole('combobox', { name: 'Input file', exact: true })).toHaveValue('', { timeout: 6000 })
  await expect(page.getByRole('button', { name: 'Queue conversion', exact: true })).toBeDisabled()
  expect(maxJobs).toBe(1)
  expect(maxFiles).toBe(1)
  await page.unrouteAll({ behavior: 'wait' })
})

test('Apps observes an unacknowledged transition and disables actions while status is unavailable', async ({ page }) => {
  let running = false, unavailable = false, attention = false, mutations = 0
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (route.request().method() !== 'GET') mutations++
    if (path === '/api/v1/setup') return route.fulfill({ json: { claimed: true, development: true } })
    if (path === '/api/v1/session') return route.fulfill({ json: { device: { id: 'fixture-owner', name: 'Fixture owner', capabilities: ['admin'], createdAt: 1, revokedAt: null }, verifiedAt: 1, expiresAt: 9999999999 } })
    if (path === '/api/v1/host/report') return route.fulfill({ json: { generatedAt: new Date().toISOString(), host: { os: 'linux', architecture: 'amd64' }, checks: [], prerequisitesMet: false, executionEnabled: false } })
    if (path === '/api/v1/apps') return unavailable ? route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Fixture app status unavailable' } } }) : route.fulfill({ json: { runtimeConfigured: true, apps: [{ workload: 'files', instanceId: 'a'.repeat(32), state: attention ? 'failed' : running ? 'running' : 'starting', requiresAttention: attention, updatedAt: 1 }] } })
    return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Unused fixture endpoint' } } })
  })
  await page.goto('/')
  await page.locator('nav').getByRole('button', { name: 'Apps', exact: false }).click()
  const filesApp = page.getByRole('region', { name: 'Private Files', exact: true })
  await expect(filesApp.getByText('starting', { exact: true })).toBeVisible()
  running = true
  await expect(filesApp.getByText('running', { exact: true })).toBeVisible()
  unavailable = true
  await page.getByRole('button', { name: 'Refresh status', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('App status is unavailable.')
  await expect(filesApp.getByRole('button', { name: 'Stop app', exact: true })).toBeDisabled()
  unavailable = false
  await page.getByRole('button', { name: 'Refresh status', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(filesApp.getByRole('button', { name: 'Stop app', exact: true })).toBeEnabled()
  attention = true
  await page.reload()
  await page.locator('nav').getByRole('button', { name: 'Apps', exact: false }).click()
  await expect(filesApp.getByRole('status')).toContainText('This app needs recovery.')
  await expect(filesApp.getByRole('button', { name: 'Start Files', exact: true })).toBeDisabled()
  expect(mutations).toBe(0)
})

for (const screen of ['Jobs', 'Files', 'Apps'] as const) {
  test(`leaving ${screen} aborts pending reads and reopening starts a fresh refresh`, async ({ page }) => {
    let hold = false
    const held: Route[] = []
    const paths = screen === 'Jobs' ? ['/api/v1/jobs', '/api/v1/files'] : screen === 'Files' ? ['/api/v1/files', '/api/v1/files/storage', '/api/v1/transfers'] : ['/api/v1/apps']
    const requests: Record<string, number> = Object.fromEntries(paths.map(path => [path, 0]))
    await page.route('**/api/v1/**', route => {
      const path = new URL(route.request().url()).pathname
      if (path === '/api/v1/setup') return route.fulfill({ json: { claimed: true, development: true } })
      if (path === '/api/v1/session') return route.fulfill({ json: { device: { id: 'fixture-owner', name: 'Fixture owner', capabilities: ['admin', 'jobs', 'files'], createdAt: 1, revokedAt: null }, verifiedAt: 1, expiresAt: 9999999999 } })
      if (path === '/api/v1/host/report') return route.fulfill({ json: { generatedAt: new Date().toISOString(), host: { os: 'linux', architecture: 'amd64' }, checks: [], prerequisitesMet: false, executionEnabled: false } })
      if (paths.includes(path)) {
        requests[path]++
        if (hold) { held.push(route); return }
        if (path === '/api/v1/apps') return route.fulfill({ json: { apps: [], runtimeConfigured: false } })
        if (path === '/api/v1/files/storage') return route.fulfill({ json: { quotaBytes: 12 * 2 ** 30, storedBytes: 0, trashBytes: 0, uploadReservedBytes: 0, jobReservedBytes: 0, cleanupReservedBytes: 0, chargedBytes: 0, availableBytes: 12 * 2 ** 30 } })
        return route.fulfill({ json: [] })
      }
      if (path === '/api/v1/apps') return route.fulfill({ json: { apps: [], runtimeConfigured: false } })
      return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Unused fixture endpoint' } } })
    })
    await page.goto('/')
    await expect(page.getByText('CHECKS PASSED', { exact: true })).toBeVisible()
    for (const path of paths) requests[path] = 0
    hold = true
    await page.locator('nav').getByRole('button', { name: screen, exact: false }).click()
    await expect.poll(() => held.length).toBe(paths.length)
    const aborted = held.map(route => page.waitForEvent('requestfailed', { timeout: 3000, predicate: request => request === route.request() }))
    await page.locator('nav').getByRole('button', { name: 'Overview', exact: false }).click()
    for (const request of await Promise.all(aborted)) expect(request.failure()).not.toBeNull()
    for (const route of held) await route.abort().catch(() => {})
    hold = false
    await page.locator('nav').getByRole('button', { name: screen, exact: false }).click()
    await expect(page.getByText(screen === 'Jobs' ? 'Last observed' : screen === 'Files' ? 'File list last observed' : 'App status last observed', { exact: false })).toBeVisible()
    await expect(page.getByRole('alert')).toHaveCount(0)
    expect(requests).toEqual(Object.fromEntries(paths.map(path => [path, 2])))
  })
}
