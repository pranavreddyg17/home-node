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
