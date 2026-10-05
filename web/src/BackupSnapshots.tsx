import { useEffect, useRef, useState } from 'react'
import { message } from './api'
import { listBackupSnapshots, previewBackupSnapshot, type SnapshotPage, type SnapshotPreview } from './backup'

export function BackupSnapshots({ repositoryId }: { repositoryId: string }) {
  const [page, setPage] = useState<SnapshotPage | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [preview, setPreview] = useState<SnapshotPreview | null>(null)
  const password = useRef<HTMLInputElement>(null)
  const pending = useRef<AbortController | null>(null)
  useEffect(() => () => {
    pending.current?.abort()
    if (password.current) password.current.value = ''
  }, [repositoryId])
  const load = async (cursor: string) => {
    if (!password.current || busy || pending.current) return
    const secret = new TextEncoder().encode(password.current.value)
    password.current.value = ''
    const controller = new AbortController()
    pending.current = controller
    setBusy(true); setError(''); setPage(null); setPreview(null)
    try {
      const result = await listBackupSnapshots(repositoryId, cursor, secret, controller.signal)
      if (!controller.signal.aborted) setPage(result)
    } catch (e) { if (!controller.signal.aborted) setError(message(e)) }
    finally { secret.fill(0); if (pending.current === controller) pending.current = null; if (!controller.signal.aborted) setBusy(false) }
  }
  const inspect = async (snapshotId: string) => {
    if (!password.current || busy || pending.current) return
    const secret = new TextEncoder().encode(password.current.value)
    password.current.value = ''
    const controller = new AbortController()
    pending.current = controller
    setBusy(true); setError(''); setPreview(null)
    try {
      const result = await previewBackupSnapshot(repositoryId, snapshotId, secret, controller.signal)
      if (!controller.signal.aborted) setPreview(result)
    } catch (e) { if (!controller.signal.aborted) setError(message(e)) }
    finally { secret.fill(0); if (pending.current === controller) pending.current = null; if (!controller.signal.aborted) setBusy(false) }
  }
  return <section aria-label="Backup snapshots">
    <h3>Browse backup snapshots</h3>
    <p>Attach the registered drive and use a recently verified owner session. Listing dates does not verify that a snapshot can be restored.</p>
    <form onSubmit={e => { e.preventDefault(); void load('') }}>
      <label>Password for snapshot browsing<input ref={password} type="password" autoComplete="off" required maxLength={8192} disabled={busy} /></label>
      <button disabled={busy}>{busy ? 'Loading snapshots…' : 'Browse newest snapshots'}</button>
      {page?.next && <button type="button" disabled={busy} onClick={() => void load(page.next)}>Load older snapshots</button>}
    </form>
    <p>Enter the password again for each page or compatibility inspection. It is cleared after each request.</p>
    {page && (page.snapshots.length ? <ul>{page.snapshots.map(snapshot => <li key={snapshot.id}><time dateTime={snapshot.createdAt}>{new Date(snapshot.createdAt).toLocaleString()}</time><br /><code className="checksum">{snapshot.id}</code><button disabled={busy} onClick={() => void inspect(snapshot.id)}>Inspect compatibility</button></li>)}</ul> : <p role="status">No snapshots on this page.</p>)}
    {preview && <div role="status"><h4>Compatible metadata</h4><code className="checksum">{preview.snapshotId}</code><p>Release {preview.release}, catalog {preview.catalogVersion}.</p><ul>{preview.files.map(file => <li key={file.workload}>{file.workload}: {file.bytes.toLocaleString()} declared bytes</li>)}</ul><p>Payload integrity and application recovery have not been tested by this preview. No files were restored.</p></div>}
    {error && <p role="alert" className="form-error">{error}</p>}
  </section>
}
