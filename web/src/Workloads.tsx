import { useEffect, useRef, useState } from 'react'
import { sha256 } from '@noble/hashes/sha2.js'
import { bytesToHex } from '@noble/hashes/utils.js'
import { api, message, type Session } from './api'

type App = { workload: string; instanceId: string; state: string; updatedAt: number }
type Operation = { id: string; state: string; result: { code?: string } }
type FileInfo = { id: string; name: string; size: number; sha256: string; createdAt: number; trashUntil: number | null }
type Transfer = { id: string; name: string; size: number; sha256: string; offset: number; state: string; expiresAt: number }
const bytes = (n: number) => n >= 2 ** 30 ? `${(n / 2 ** 30).toFixed(2)} GiB` : n >= 2 ** 20 ? `${(n / 2 ** 20).toFixed(1)} MiB` : n >= 1024 ? `${(n / 1024).toFixed(1)} KiB` : `${n} bytes`
async function action<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api/v1${path}`, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() }, body: JSON.stringify(body) })
  const data = await response.json()
  if (!response.ok) throw new Error(data.error?.message ?? 'The action failed.')
  return data as T
}

export function Apps({ session, verify }: { session: Session; verify: () => Promise<void> }) {
  const [apps, setApps] = useState<App[]>([])
  const [configured, setConfigured] = useState(false)
  const [operation, setOperation] = useState<Operation | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const refresh = async () => { const data = await api<{ apps: App[]; runtimeConfigured: boolean }>('/apps'); setApps(data.apps); setConfigured(data.runtimeConfigured) }
  useEffect(() => { void refresh().catch(e => setError(message(e))) }, [])
  useEffect(() => {
    if (!operation || !['pending', 'executing'].includes(operation.state)) return
    let active = true
    const timer = setInterval(() => { void api<Operation>(`/operations/${operation.id}`).then(op => { if (active) { setOperation(op); void refresh().catch(e => setError(message(e))) } }).catch(e => { if (active) setError(message(e)) }) }, 1000)
    return () => { active = false; clearInterval(timer) }
  }, [operation?.id, operation?.state])
  const run = async (name: string, command: string) => { setBusy(true); setError(''); try { setOperation(await action<Operation>(`/apps/${name}/actions`, { action: command })); await refresh() } catch (e) { setError(message(e)) } finally { setBusy(false) } }
  return <div className="stack">{error && <p className="form-error" role="alert">{error}</p>}{!configured && <p className="inline-notice">Workload services are not configured on this host. A supported Linux host and verified guest images are required.</p>}{operation && <p role="status">App operation: {operation.state}{operation.result.code ? ` · ${operation.result.code}` : ''}</p>}{session.device.capabilities.includes('admin') && <button className="refresh-button" onClick={() => void verify().catch(e => setError(message(e)))}>Verify passkey for app changes</button>}
    {(['files', 'ai'] as const).map(name => { const app = apps.find(a => a.workload === name); const phase = app?.state ?? 'absent'; return <section className="panel" key={name}><div className="section-heading"><h2>{name === 'files' ? 'Private Files' : 'Private AI'}</h2><span className="state-pill">{phase}</span></div><p>{name === 'files' ? 'Your files live on a dedicated guest data disk. No public sharing or host folders are exposed.' : 'CPU inference runs in its own guest, with a verified model profile and no cloud fallback or administrative tools.'}</p><p>Network access: none. Installation uses a signed catalog and a verified immutable system image.</p><div className="button-row"><button disabled={!configured || busy || ['starting', 'stopping', 'running'].includes(phase) || !session.device.capabilities.includes('admin')} onClick={() => void run(name, 'start')}>Start {name === 'files' ? 'Files' : 'AI'}</button><button disabled={!configured || busy || phase !== 'running' || !session.device.capabilities.includes('admin')} onClick={() => { if (window.confirm('Stop this app? Active requests may be interrupted. Its data will be retained.')) void run(name, 'stop') }}>Stop app</button></div></section> })}
  </div>
}

export function Files() {
  const [files, setFiles] = useState<FileInfo[]>([])
  const [error, setError] = useState('')
  const [phase, setPhase] = useState('')
  const [progress, setProgress] = useState(0)
  const [busy, setBusy] = useState(false)
  const [trash, setTrash] = useState(false)
  const pause = useRef(false)
  const refresh = () => api<FileInfo[]>('/files').then(setFiles)
  useEffect(() => { void refresh().catch(e => setError(message(e))); return () => { pause.current = true } }, [])
  const upload = async (file: File) => {
    if (file.size > 2 ** 30) { setError('This build accepts files up to 1 GiB.'); return }
    setBusy(true); setError(''); pause.current = false; setProgress(0); setPhase('Calculating file checksum')
    try {
      const digest = sha256.create()
      for (let offset = 0; offset < file.size; offset += 2 ** 20) {
        if (pause.current) { setPhase('Paused. Select the same file to resume.'); return }
        digest.update(new Uint8Array(await file.slice(offset, offset + 2 ** 20).arrayBuffer()))
        setProgress(Math.min(file.size, offset + 2 ** 20) / file.size)
      }
      const hash = bytesToHex(digest.digest())
      const storageKey = `homenode-transfer:${hash}:${file.size}`
      let transfer: Transfer | null = null
      const saved = localStorage.getItem(storageKey)
      if (saved) {
        try { transfer = await api<Transfer>(`/transfers/${saved}`) } catch { localStorage.removeItem(storageKey) }
        if (transfer && (transfer.expiresAt * 1000 <= Date.now() || !['uploading', 'verifying', 'ready'].includes(transfer.state))) transfer = null
      }
      if (!transfer) { transfer = await api<Transfer>('/transfers', { name: file.name, size: file.size, sha256: hash }); localStorage.setItem(storageKey, transfer.id) }
      setPhase('Uploading'); setProgress(file.size ? transfer.offset / file.size : 1)
      while (transfer.offset < file.size) {
        if (pause.current) { setPhase('Paused. Select the same file to resume.'); return }
        const chunk: Uint8Array = new Uint8Array(await file.slice(transfer.offset, transfer.offset + 256 * 1024).arrayBuffer())
        let binary = ''
        for (let i = 0; i < chunk.length; i += 8192) binary += String.fromCharCode(...chunk.subarray(i, i + 8192))
        transfer = await api<Transfer>(`/transfers/${transfer.id}/chunks`, { offset: transfer.offset, data: btoa(binary), sha256: bytesToHex(sha256(chunk)) })
        setProgress(transfer.offset / file.size)
      }
      setPhase('Verifying complete file')
      await api(`/transfers/${transfer.id}/finalize`, {})
      localStorage.removeItem(storageKey); setPhase('Upload verified'); setProgress(1); await refresh()
    } catch (e) { setError(message(e)); setPhase('Upload stopped. Select the same file to retry or resume.') } finally { setBusy(false) }
  }
  const change = async (file: FileInfo, action: string, name = '') => { setError(''); try { await api(`/files/${file.id}/actions`, { action, name }); await refresh() } catch (e) { setError(message(e)) } }
  const visible = files.filter(f => Boolean(f.trashUntil) === trash)
  return <div className="stack">{error && <p className="form-error" role="alert">{error}</p>}<section className="panel"><div className="section-heading"><h2>Your files</h2><button onClick={() => void refresh().catch(e => setError(message(e)))}>Refresh</button></div><p>Uploads are checked before they become downloadable. If a connection breaks, select the same file again to resume.</p><label className="upload-picker">Upload a file<input type="file" disabled={busy} onChange={e => { const file = e.target.files?.[0]; if (file) void upload(file); e.target.value = '' }} /></label>{phase && <div className="transfer-progress" role="status"><span>{phase}</span><progress max={1} value={progress} aria-label="Transfer progress" />{busy && <button onClick={() => { pause.current = true }}>Pause upload</button>}</div>}</section>
    <section className="panel"><div className="button-row"><button aria-pressed={!trash} onClick={() => setTrash(false)}>Files</button><button aria-pressed={trash} onClick={() => setTrash(true)}>Trash</button></div>{!visible.length && <p>{trash ? 'Trash is empty.' : 'No files yet. Start Private Files in Apps, then upload your first file.'}</p>}{visible.map(file => <article className="file-row" key={file.id}><div><strong>{file.name}</strong><p>{bytes(file.size)} · {new Date(file.createdAt * 1000).toLocaleDateString()}</p><details><summary>Integrity</summary><code className="checksum">SHA-256: {file.sha256}</code></details></div><div className="button-row">{trash ? <button onClick={() => void change(file, 'restore')}>Restore</button> : <><a className="download-link" href={`/api/v1/files/${file.id}/download`} download>Download</a><button onClick={() => { const name = window.prompt('New filename', file.name); if (name) void change(file, 'rename', name) }}>Rename</button><button onClick={() => void change(file, 'trash')}>Move to trash</button></>}</div></article>)}</section>
  </div>
}
