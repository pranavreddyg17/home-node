import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { dependencyEvidence } from './dependency-evidence.mjs'

function fixture(t, locked = '1.0.0', installed = '1.0.0') {
  const root = mkdtempSync(path.join(tmpdir(), 'homenode-bundle-evidence-'))
  t.after(() => rmSync(root, { recursive: true }))
  const directory = path.join(root, 'node_modules/@fixture/package')
  mkdirSync(directory, { recursive: true })
  writeFileSync(path.join(directory, 'package.json'), JSON.stringify({ name: '@fixture/package', version: installed, license: 'MIT' }))
  writeFileSync(path.join(root, 'package-lock.json'), JSON.stringify({ packages: { 'node_modules/@fixture/package': { version: locked, integrity: 'fixture-integrity' } } }))
  const plugin = dependencyEvidence()
  plugin.configResolved({ root })
  const bundle = { 'app.js': { type: 'chunk', code: 'console.log(7)', modules: { [path.join(directory, 'index.js')]: {} } } }
  return { root, plugin, bundle, evidence: path.join(root, 'build-evidence/dependencies.json') }
}

test('emitted package identity and asset hash are retained without host paths', t => {
  const { root, plugin, bundle, evidence } = fixture(t)
  plugin.writeBundle({}, bundle)
  const text = readFileSync(evidence, 'utf8')
  assert.ok(!text.includes(root))
  const record = JSON.parse(text)
  assert.equal(record.completeness, 'incomplete')
  assert.equal(record.dependencies[0].name, '@fixture/package')
  assert.equal(record.dependencies[0].version, '1.0.0')
  assert.equal(record.assets[0].sha256, createHash('sha256').update('console.log(7)').digest('hex'))
})

test('changed installed dependency refuses before evidence publication', t => {
  const { plugin, bundle, evidence } = fixture(t, '1.0.0', '2.0.0')
  assert.throws(() => plugin.writeBundle({}, bundle), /differs from lockfile/)
  assert.equal(existsSync(evidence), false)
})

test('unlocked bundled dependency refuses', t => {
  const { root, bundle, evidence } = fixture(t)
  writeFileSync(path.join(root, 'package-lock.json'), JSON.stringify({ packages: {} }))
  const plugin = dependencyEvidence()
  plugin.configResolved({ root })
  assert.throws(() => plugin.writeBundle({}, bundle), /differs from lockfile/)
  assert.equal(existsSync(evidence), false)
})

test('empty emitted dependency graph refuses', t => {
  const { plugin, evidence } = fixture(t)
  assert.throws(() => plugin.writeBundle({}, { 'app.js': { type: 'chunk', code: '', modules: {} } }), /Empty bundled dependency/)
  assert.equal(existsSync(evidence), false)
})
