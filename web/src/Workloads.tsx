import { approvedAction } from './approvals'
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
  const run = async (name: string, command: string) => { setBusy(true); setError(''); try { setOperation(await approvedAction<Operation>(`/apps/${name}/actions`, { action: command }, { 'Idempotency-Key': crypto.randomUUID() })); await refresh() } catch (e) { setError(message(e)) } finally { setBusy(false) } }
  return <div className="stack">{error && <p className="form-error" role="alert">{error}</p>}{!configured && <p className="inline-notice">Workload services are not configured on this host. A supported Linux host and verified guest images are required.</p>}{operation && <p role="status">App operation: {operation.state}{operation.result.code ? ` · ${operation.result.code}` : ''}</p>}{session.device.capabilities.includes('admin') && <button className="refresh-button" onClick={() => void verify().catch(e => setError(message(e)))}>Verify passkey for app changes</button>}
    {(['files', 'ai'] as const).map(name => { const app = apps.find(a => a.workload === name); const phase = app?.state ?? 'absent'; return <section className="panel" key={name}><div className="section-heading"><h2>{name === 'files' ? 'Private Files' : 'Private AI'}</h2><span className="state-pill">{phase}</span></div><p>{name === 'files' ? 'Your files live on a dedicated guest data disk. No public sharing or host folders are exposed.' : 'CPU inference runs in its own guest, with a verified model profile and no cloud fallback or administrative tools.'}</p><p>Network access: none. Installation uses a signed catalog and a verified immutable system image.</p><div className="button-row"><button disabled={!configured || busy || ['starting', 'stopping', 'running'].includes(phase) || !session.device.capabilities.includes('admin')} onClick={() => void run(name, 'start')}>Start {name === 'files' ? 'Files' : 'AI'}</button><button disabled={!configured || busy || !['running', 'starting'].includes(phase) || !session.device.capabilities.includes('admin')} onClick={() => { if (window.confirm('Stop this app? Active requests may be interrupted. Its data will be retained.')) void run(name, 'stop') }}>Stop app</button></div></section> })}
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
  const [pendingUpload, setPendingUpload] = useState<{ id: string; storageKey: string } | null>(null)
  const refresh = () => api<FileInfo[]>('/files').then(setFiles)
  useEffect(() => { void refresh().catch(e => setError(message(e))); return () => { pause.current = true } }, [])
  const upload = async (file: File) => {
    if (file.size > 2 ** 30) { setError('This build accepts files up to 1 GiB.'); return }
    setBusy(true); setError(''); pause.current = false; setPendingUpload(null); setProgress(0); setPhase('Calculating file checksum')
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
      if (transfer.state !== 'ready') setPendingUpload({ id: transfer.id, storageKey })
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
      localStorage.removeItem(storageKey); setPendingUpload(null); setPhase('Upload verified'); setProgress(1); await refresh()
    } catch (e) { setError(message(e)); setPhase('Upload stopped. Select the same file to retry or resume.') } finally { setBusy(false) }
  }
  const cancelUpload = async () => {
    if (!pendingUpload || busy) return
    setBusy(true); setError('')
    try {
      await api(`/transfers/${pendingUpload.id}/cancel`, {})
      localStorage.removeItem(pendingUpload.storageKey)
      setPendingUpload(null); setPhase('Upload cancelled. Storage cleanup will finish when Private Files is available.'); setProgress(0)
    } catch (e) { setError(message(e)) } finally { setBusy(false) }
  }
  const change = async (file: FileInfo, action: string, name = '') => { setError(''); try { await api(`/files/${file.id}/actions`, { action, name }); await refresh() } catch (e) { setError(message(e)) } }
  const visible = files.filter(f => Boolean(f.trashUntil) === trash)
  return <div className="stack">{error && <p className="form-error" role="alert">{error}</p>}<section className="panel"><div className="section-heading"><h2>Your files</h2><button onClick={() => void refresh().catch(e => setError(message(e)))}>Refresh</button></div><p>Uploads are checked before they become downloadable. If a connection breaks, select the same file again to resume. Trash retains files for seven days, then permanently removes them; trash still counts toward storage until removal finishes.</p><label className="upload-picker">Upload a file<input type="file" disabled={busy} onChange={e => { const file = e.target.files?.[0]; if (file) void upload(file); e.target.value = '' }} /></label>{phase && <div className="transfer-progress" role="status"><span>{phase}</span><progress max={1} value={progress} aria-label="Transfer progress" />{busy && <button onClick={() => { pause.current = true }}>Pause upload</button>}{!busy && pendingUpload && <button onClick={() => void cancelUpload()}>Cancel upload and discard partial file</button>}</div>}</section>
    <section className="panel"><div className="button-row"><button aria-pressed={!trash} onClick={() => setTrash(false)}>Files</button><button aria-pressed={trash} onClick={() => setTrash(true)}>Trash</button></div>{!visible.length && <p>{trash ? 'Trash is empty.' : 'No files yet. Start Private Files in Apps, then upload your first file.'}</p>}{visible.map(file => <article className="file-row" key={file.id}><div><strong>{file.name}</strong><p>{bytes(file.size)} · {new Date(file.createdAt * 1000).toLocaleDateString()}</p>{file.trashUntil !== null && <p>{file.trashUntil * 1000 <= Date.now() ? 'Retention expired; removal awaits Private Files.' : `Trash expires ${new Date(file.trashUntil * 1000).toLocaleDateString()}.`}</p>}<details><summary>Integrity</summary><code className="checksum">SHA-256: {file.sha256}</code></details></div><div className="button-row">{trash ? <button disabled={file.trashUntil !== null && file.trashUntil * 1000 <= Date.now()} onClick={() => void change(file, 'restore')}>Restore</button> : <><a className="download-link" href={`/api/v1/files/${file.id}/download`} download>Download</a><button onClick={() => { const name = window.prompt('New filename', file.name); if (name) void change(file, 'rename', name) }}>Rename</button><button onClick={() => void change(file, 'trash')}>Move to trash</button></>}</div></article>)}</section>
  </div>
}

type Job = { cleanupPending: boolean; id: string; attemptId: string; inputId: string; preset: string; state: string; outputId: string | null; errorCode: string | null; createdAt: number; updatedAt: number }
export function Jobs() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [files, setFiles] = useState<FileInfo[]>([])
  const [input, setInput] = useState('')
  const [preset, setPreset] = useState('mp4-720p')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [checkedAt, setCheckedAt] = useState<Date | null>(null)
  const refresh = async () => { const [jobList, fileList] = await Promise.all([api<Job[]>('/jobs'), api<FileInfo[]>('/files')]); setJobs(jobList); setFiles(fileList.filter(f => !f.trashUntil)); setCheckedAt(new Date()) }
  useEffect(() => { void refresh().catch(e => setError(message(e))); const timer = setInterval(() => { void refresh().catch(e => setError(message(e))) }, 2000); return () => clearInterval(timer) }, [])
  const run = async (work: () => Promise<unknown>) => { setBusy(true); setError(''); try { await work(); await refresh() } catch (e) { setError(message(e)) } finally { setBusy(false) } }
  return <div className="stack">{error && <p className="form-error" role="alert">{error}</p>}<section className="panel"><h2>Convert a video</h2><p>Choose an uploaded file and a maintained MP4 preset. Each attempt runs in a separate VM. Closing this page does not stop it.</p><form onSubmit={e => { e.preventDefault(); void run(() => action('/jobs', { inputId: input, preset, retryGroup: '' })) }}><label>Input file<select value={input} required onChange={e => setInput(e.target.value)}><option value="">Select a file</option>{files.map(file => <option key={file.id} value={file.id}>{file.name} · {bytes(file.size)}</option>)}</select></label><label>Conversion preset<select value={preset} onChange={e => setPreset(e.target.value)}><option value="mp4-720p">MP4 · Up to 720p</option><option value="mp4-1080p">MP4 · Up to 1080p</option></select></label><button className="primary" disabled={busy || !input}>Queue conversion</button></form></section>
    <section className="panel"><div className="section-heading"><h2>Job attempts</h2><button onClick={() => void refresh().catch(e => setError(message(e)))}>Refresh</button></div>{!jobs.length && <p>No jobs yet. Upload a video in Files to get started.</p>}{jobs.map(job => <article className="file-row" key={job.attemptId}><div><strong>{files.find(f => f.id === job.inputId)?.name ?? 'Video conversion'}</strong><p>{job.preset} · {new Date(job.createdAt * 1000).toLocaleString()}</p><span className="state-pill">{job.state}</span>{job.cleanupPending && <p>Temporary workload cleanup is pending; HomeNode will retry.</p>}{job.errorCode && job.errorCode !== 'CLEANUP_PENDING' && <p>{job.errorCode.replaceAll('_', ' ').toLowerCase()}</p>}<small className="attempt-label">Attempt {job.attemptId.slice(0, 10)}</small></div><div className="button-row">{['queued', 'preparing', 'running', 'finalizing'].includes(job.state) && <button disabled={busy} onClick={() => void run(() => api(`/jobs/${job.id}/cancel`, { attemptId: job.attemptId }))}>Cancel</button>}{['failed', 'cancelled', 'interrupted'].includes(job.state) && <button disabled={busy} onClick={() => void run(() => action('/jobs', { inputId: job.inputId, preset: job.preset, retryGroup: job.id }))}>Retry as new attempt</button>}{job.state === 'succeeded' && job.outputId && <a className="download-link" href={`/api/v1/files/${job.outputId}/download`} download>Download result</a>}</div></article>)}{checkedAt && <p className="timestamp">Last observed {checkedAt.toLocaleTimeString()}</p>}</section>
  </div>
}

type Conversation = { id: string; title: string; createdAt: number; deletionPending: boolean; deletionRequiresAction: boolean }
type Generation = { id: string; conversationId: string; prompt: string; output: string; state: string; createdAt: number }
export function AI({ verify }: { verify: () => Promise<void> }) {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [selected, setSelected] = useState('')
  const [history, setHistory] = useState<Generation[]>([])
  const [prompt, setPrompt] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [active, setActive] = useState<string | null>(null)
  const refresh = () => api<Conversation[]>('/ai/conversations').then(setConversations)
  const deleting = conversations.find(c => c.id === selected)?.deletionPending ?? false
  const deletionRequiresAction = conversations.find(c => c.id === selected)?.deletionRequiresAction ?? false
  const pendingDeletion = conversations.some(c => c.deletionPending)
  useEffect(() => {
    if (!pendingDeletion) return
    let current = true
    const timer = setInterval(() => { void api<Conversation[]>('/ai/conversations').then(items => {
      if (!current) return
      setConversations(items)
      setSelected(id => id && !items.some(c => c.id === id) ? '' : id)
    }).catch(e => { if (current) setError(message(e)) }) }, 2000)
    return () => { current = false; clearInterval(timer) }
  }, [pendingDeletion])
  useEffect(() => { void refresh().catch(e => setError(message(e))) }, [])
  useEffect(() => {
    let current = true; setHistory([]); setActive(null)
    if (selected && !deleting) void api<Generation[]>(`/ai/conversations/${selected}/history`).then(items => { if (current) { setHistory(items); setActive(items.find(g => ['queued', 'running', 'cancelling'].includes(g.state))?.id ?? null) } }).catch(e => { if (current) setError(message(e)) })
    return () => { current = false }
  }, [selected, deleting])
  useEffect(() => {
    if (!active) return
    let current = true
    const timer = setInterval(() => { void api<Generation>(`/ai/generations/${active}`).then(g => { if (!current) return; setHistory(items => items.map(item => item.id === g.id ? g : item)); if (!['queued', 'running', 'cancelling'].includes(g.state)) setActive(null) }).catch(e => { if (current) setError(message(e)) }) }, 1000)
    return () => { current = false; clearInterval(timer) }
  }, [active])
  const create = async () => { setBusy(true); setError(''); try { const conversation = await api<Conversation>('/ai/conversations', { title: `Conversation ${conversations.length + 1}` }); await refresh(); setSelected(conversation.id) } catch (e) { setError(message(e)) } finally { setBusy(false) } }
  const send = async () => { setBusy(true); setError(''); try { const g = await action<Generation>('/ai/generations', { conversationId: selected, prompt }); setHistory(items => [...items, g]); setPrompt(''); setActive(g.id) } catch (e) { setError(message(e)) } finally { setBusy(false) } }
  const remove = async () => { if (!window.confirm('Delete this conversation and its stored messages? This cannot be undone.')) return; setBusy(true); setError(''); try { const op = await approvedAction<Operation>(`/ai/conversations/${selected}/delete`, {}, { 'Idempotency-Key': crypto.randomUUID() });
      let completed = false;
      for (let i = 0; i < 120; i++) {
        const status = await api<Operation>(`/operations/${op.id}`);
        if (status.state === 'succeeded') { completed = true; break }
        if (status.state !== 'pending') throw new Error('Deletion needs attention. Check its operation status.');
        await new Promise(resolve => setTimeout(resolve, 1000));
      }
      if (!completed) throw new Error(`Deletion remains pending (operation ${op.id}). The server will retry cleanup; data has not been reported as deleted.`);
      setSelected(''); await refresh() } catch (e) { setError(message(e)) } finally { setBusy(false) } }
  return <div className="stack">{error && <p className="form-error" role="alert">{error}</p>}<section className="panel"><div className="section-heading"><h2>Private AI</h2><button disabled={busy} onClick={() => void create()}>New conversation</button></div><p>Start the installed CPU model profile in Apps. Chat stays on the AI guest disk. The model has no network access or administrative tools.</p><p>Context includes up to eight recent turns within a bounded size. Responses are limited to 256 tokens in this profile.</p><label className="conversation-picker">Conversation<select value={selected} onChange={e => setSelected(e.target.value)}><option value="">Choose a conversation</option>{conversations.map(c => <option key={c.id} value={c.id}>{c.title}{c.deletionPending ? ' (deletion pending)' : ''}</option>)}</select></label>{selected && <div className="button-row"><a className="download-link" href={`/api/v1/ai/conversations/${selected}/export`} download>Export history</a><button disabled={busy || Boolean(active) || deleting} onClick={() => void remove()}>Delete conversation</button><button onClick={() => void verify().catch(e => setError(message(e)))}>Verify passkey</button></div>}</section>
    {selected && deleting && <p role="status">{deletionRequiresAction ? 'Deletion needs attention because its saved cleanup record is invalid. New messages remain disabled; check the server operation before attempting recovery.' : 'Deletion is pending. The server will retry cleanup automatically. New messages are disabled until cleanup finishes.'}</p>}
    {selected && !deleting && <section className="panel chat-panel"><div className="chat-history" aria-live="polite">{!history.length && <p>Ask your first question.</p>}{history.map(g => <article key={g.id}><div className="chat-message user-message"><strong>You</strong><p>{g.prompt}</p></div><div className="chat-message assistant-message"><strong>Private AI</strong><p>{g.output || (['queued', 'running'].includes(g.state) ? 'Waiting for the local model…' : 'No response was completed.')}</p><small>{g.state}</small></div></article>)}</div><form onSubmit={e => { e.preventDefault(); void send() }}><label>Message<textarea value={prompt} onChange={e => setPrompt(e.target.value)} maxLength={2048} rows={4} required placeholder="Ask your local model…" /></label><div className="button-row"><button className="primary" disabled={busy || Boolean(active) || !prompt.trim()}>Send message</button>{active && <button type="button" onClick={() => void api(`/ai/generations/${active}/cancel`, {}).catch(e => setError(message(e)))}>Cancel generation</button>}</div></form></section>}
  </div>
}
