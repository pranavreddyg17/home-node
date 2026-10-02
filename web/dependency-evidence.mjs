import { createHash } from 'node:crypto'
import { readFileSync, mkdirSync, writeFileSync, openSync, closeSync, fstatSync, readSync, constants, readdirSync } from 'node:fs'
import path from 'node:path'

function readManifest(filename) {
  const fd = openSync(filename, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK)
  try {
    const before = fstatSync(fd)
    if (!before.isFile() || before.size < 1 || before.size > 1024 * 1024) {
      throw new Error('Invalid dependency manifest size/type')
    }
    const buffer = Buffer.alloc(1024 * 1024 + 1)
    let count = 0
    while (count < buffer.length) {
      const read = readSync(fd, buffer, count, buffer.length - count, null)
      if (!read) break
      count += read
    }
    const after = fstatSync(fd)
    if (count !== before.size || count > 1024 * 1024 || after.size !== before.size || after.mtimeMs !== before.mtimeMs) {
      throw new Error('Changed or oversized dependency manifest')
    }
    return buffer.subarray(0, count)
  } finally {
    closeSync(fd)
  }
}

// Build-graph evidence is incomplete until dependency/license review qualifies it.
export function dependencyEvidence() {
  let root
  let packages
  return {
    name: 'homenode-dependency-evidence',
    apply: 'build',
    enforce: 'post',
    configResolved(config) {
      root = config.root
      packages = JSON.parse(readFileSync(path.join(root, 'package-lock.json'), 'utf8')).packages
    },
    writeBundle(_options, bundle) {
      const dependencies = new Map()
      const assets = []
      for (const [filename, output] of Object.entries(bundle)) {
        const bytes = output.type === 'chunk' ? output.code : output.source
        assets.push({ path: filename, sha256: createHash('sha256').update(bytes).digest('hex') })
        if (output.type !== 'chunk') continue
        for (const id of Object.keys(output.modules)) {
          const marker = id.lastIndexOf('/node_modules/')
          if (marker < 0) continue
          const tail = id.slice(marker + '/node_modules/'.length)
          const parts = tail.split('/')
          const name = parts[0].startsWith('@') ? parts.slice(0, 2).join('/') : parts[0]
          const directory = id.slice(0, marker + '/node_modules/'.length) + name
          const key = path.relative(root, directory).split(path.sep).join('/')
          const locked = packages[key]
          const manifest = readManifest(path.join(directory, 'package.json'))
          const installed = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(manifest))
          if (!locked || !locked.version || installed.name !== name || installed.version !== locked.version) {
            throw new Error(`Bundled dependency differs from lockfile: ${name}`)
          }
          const licenseFiles = readdirSync(directory).filter(name => /^(LICENSE|COPYING|NOTICE)(\..*)?$/i.test(name)).sort().map(name => {
            const data = readManifest(path.join(directory, name))
            return { path: name, sha256: createHash('sha256').update(data).digest('hex'),
              text: new TextDecoder('utf-8', { fatal: true }).decode(data) }
          })
          dependencies.set(key, { path: key, name, version: locked.version,
            integrity: locked.integrity ?? null, resolved: locked.resolved ?? null,
            manifestSHA256: createHash('sha256').update(manifest).digest('hex'),
            license: installed.license ?? null, licenseFiles })
        }
      }
      const evidence = { schema: 1, completeness: 'incomplete',
        assets: assets.sort((a, b) => a.path.localeCompare(b.path)),
        dependencies: [...dependencies.values()].sort((a, b) => a.path.localeCompare(b.path)) }
      if (!evidence.dependencies.length) throw new Error('Empty bundled dependency inventory')
      const encoded = JSON.stringify(evidence) + '\n'
      if (Buffer.byteLength(encoded) > 8 * 1024 * 1024) throw new Error('Oversized dependency evidence')
      mkdirSync(path.join(root, 'build-evidence'), { recursive: true })
      writeFileSync(path.join(root, 'build-evidence/dependencies.json'), encoded)
    },
  }
}
