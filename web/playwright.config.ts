import { defineConfig } from '@playwright/test'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const directory = process.env.HOMENODE_TEST_STATE ?? join(mkdtempSync(join(tmpdir(), 'homenode-e2e-')), 'state')
process.env.HOMENODE_TEST_STATE = directory
export default defineConfig({
  testDir: './tests', workers: 1, timeout: 60000, retries: 0,
  use: {
    baseURL: 'http://localhost:18787',
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE } : {},
  },
  webServer: {
    command: `../bin/homenode serve --dev --port 18787 --state-dir '${directory}' --web-dir dist`,
    url: 'http://localhost:18787/api/v1/healthz', reuseExistingServer: false,
  },
})
