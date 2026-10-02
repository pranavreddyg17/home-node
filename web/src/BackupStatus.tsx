import { useEffect, useRef, useState } from 'react'
import { api, message } from './api'
import { BackupStart } from './BackupStart'

type Outcome = { status: 'unknown' | 'published'; publishedAt: number }
type Outcomes = { schema: number; current: Outcome | null; lastPublished: Outcome | null; workerCompletion: 'none' | 'uncertain' | 'complete' }

export function BackupStatus() {
  const [outcomes, setOutcomes] = useState<Outcomes | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [observedAt, setObservedAt] = useState<Date | null>(null)
  const requestId = useRef(0)
  const refresh = async () => {
    const id = ++requestId.current
    setBusy(true); setError(''); setOutcomes(null); setObservedAt(null)
    try {
      const result = await api<Outcomes>('/backups/outcomes')
      if (id !== requestId.current) return
      setOutcomes(result); setObservedAt(new Date())
    } catch (e) { if (id === requestId.current) setError(message(e)) }
    finally { if (id === requestId.current) setBusy(false) }
  }
  useEffect(() => { void refresh(); return () => { requestId.current++ } }, [])
  return <section className="panel" aria-labelledby="backup-status-heading">
    <div className="section-heading"><h2 id="backup-status-heading">External backup</h2><button disabled={busy} onClick={() => void refresh()}>Refresh backup status</button></div>
    {busy && <p role="status">Checking backup publication status…</p>}
    {error && <p className="form-error" role="alert">Backup status is unavailable. {error}</p>}
    {outcomes && <>
      {outcomes.workerCompletion === 'uncertain' && <p role="status">Backup worker completion is uncertain. New work remains paused until reconciliation.</p>}
      {outcomes.current?.status === 'unknown' && <p role="status">The latest backup outcome is uncertain. It needs reconciliation before another backup can run.</p>}
      {outcomes.lastPublished ? <p>Last acknowledged publication: <time dateTime={new Date(outcomes.lastPublished.publishedAt * 1000).toISOString()}>{new Date(outcomes.lastPublished.publishedAt * 1000).toLocaleString()}</time>.</p> : <p>No acknowledged external backup is recorded.</p>}
      <p>Acknowledged publication does not verify repository health or a successful restore. Restore and recovery controls are not yet available in this build.</p>
    </>}
    <BackupStart />
    {observedAt && <p className="timestamp">Last observed {observedAt.toLocaleTimeString()}</p>}
  </section>
}
