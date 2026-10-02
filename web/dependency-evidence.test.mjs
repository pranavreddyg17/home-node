import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, rmSync, renameSync, symlinkSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { dependencyEvidence } from './dependency-evidence.mjs'

function fixture(t, locked = '1.0.0', installed = '1.0.0') {
  const root = mkdtempSync(path.join(tmpdir(), 'homenode-bundle-evidence-'))
  t.after(() => rmSync(root, { recursive: true }))
  const directory = path.join(root, 'node_modules/@fixture/package')
  mkdirSync(directory, { recursive: true })
  writeFileSync(path.join(directory, 'LICENSE'), 'fixture notice')
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
  const notices = readFileSync(path.join(root, 'build-evidence/frontend-notices.txt'), 'utf8')
  assert.ok(notices.includes('@fixture/package@1.0.0'))
  assert.ok(notices.includes('fixture notice'))
  assert.ok(notices.includes('Incomplete source collection'))
  assert.equal(record.completeness, 'incomplete')
  assert.equal(record.dependencies[0].name, '@fixture/package')
  assert.equal(record.dependencies[0].version, '1.0.0')
  assert.deepEqual(record.dependencies[0].licenseFiles, [{ path: 'LICENSE', sha256: createHash('sha256').update('fixture notice').digest('hex'), text: 'fixture notice' }])
  assert.equal(record.dependencies[0].manifestSHA256, createHash('sha256').update(readFileSync(path.join(root, 'node_modules/@fixture/package/package.json'))).digest('hex'))
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


test('symlink and oversized package manifests refuse before publication', t => {
  for (const mutation of ['link', 'oversize']) {
    const { root, plugin, bundle, evidence } = fixture(t)
    const manifest = path.join(root, 'node_modules/@fixture/package/package.json')
    if (mutation === 'link') {
      renameSync(manifest, manifest + '.target')
      symlinkSync(manifest + '.target', manifest)
    } else {
      writeFileSync(manifest, Buffer.alloc(1024 * 1024 + 1, 32))
    }
    assert.throws(() => plugin.writeBundle({}, bundle))
    assert.equal(existsSync(evidence), false)
  }
})


test('unsafe and excessive notice files refuse before publication', t => {
  for (const mutation of ['link', 'oversize', 'utf8', 'count']) {
    const { root, plugin, bundle, evidence } = fixture(t)
    const directory = path.join(root, 'node_modules/@fixture/package')
    const notice = path.join(directory, 'LICENSE')
    if (mutation === 'link') {
      renameSync(notice, path.join(directory, 'original'))
      symlinkSync(path.join(directory, 'original'), notice)
    } else if (mutation === 'oversize') {
      writeFileSync(notice, Buffer.alloc(1024 * 1024 + 1, 32))
    } else if (mutation === 'utf8') {
      writeFileSync(notice, Buffer.from([255]))
    } else {
      for (let i = 0; i < 17; i++) writeFileSync(path.join(directory, `NOTICE.${i}`), 'notice')
    }
    assert.throws(() => plugin.writeBundle({}, bundle))
    assert.equal(existsSync(evidence), false)
  }
})
