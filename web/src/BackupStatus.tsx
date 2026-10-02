import { useEffect, useRef, useState } from 'react'
import { api, message } from './api'
import { BackupStart } from './BackupStart'
import { approvedAction } from './approvals'

type Outcome = { status: 'unknown' | 'published'; publishedAt: number }
type Outcomes = { schema: number; current: Outcome | null; lastPublished: Outcome | null; workerCompletion: 'none' | 'uncertain' | 'complete' | 'refused'; resumeJobId?: string; reminder?: { state: 'never-published' | 'current' | 'overdue' | 'unavailable'; intervalDays: number; dueAt?: number } }

export function BackupStatus() {
  const [outcomes, setOutcomes] = useState<Outcomes | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [resuming, setResuming] = useState(false)
  const [resumeError, setResumeError] = useState('')
  const [reminderDays,setReminderDays]=useState('7')
  const [reminderError,setReminderError]=useState('')
  const [reminderSaved,setReminderSaved]=useState(false)
  const [resumeMessage, setResumeMessage] = useState('')
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
  useEffect(() => { if (outcomes?.reminder) setReminderDays(String(outcomes.reminder.intervalDays >= 1 && outcomes.reminder.intervalDays <= 90 ? outcomes.reminder.intervalDays : 7)) }, [outcomes?.reminder?.intervalDays])
  const saveReminder = async () => {
    setBusy(true); setReminderError(''); setReminderSaved(false)
    try { await api('/backups/reminder', { intervalDays: Number(reminderDays) }); setReminderSaved(true); await refresh() }
    catch (e) { setReminderError(message(e)) }
    finally { setBusy(false) }
  }
  const resume = async () => {
    const id = outcomes?.resumeJobId
    if (!id || resuming || busy) return
    setResuming(true); setResumeError(''); setResumeMessage('')
    try {
      await approvedAction(`/backups/${encodeURIComponent(id)}/resume`, {}, { 'Idempotency-Key': crypto.randomUUID() })
      setResumeMessage('Workload restoration started. Refresh status to check progress.')
      await refresh()
    } catch (e) { setResumeError(message(e)) }
    finally { setResuming(false) }
  }
  useEffect(() => { void refresh(); return () => { requestId.current++ } }, [])
  return <section className="panel" aria-labelledby="backup-status-heading">
    <div className="section-heading"><h2 id="backup-status-heading">External backup</h2><button disabled={busy || resuming} onClick={() => void refresh()}>Refresh backup status</button></div>
    {busy && <p role="status">Checking backup publication status…</p>}
    {error && <p className="form-error" role="alert">Backup status is unavailable. {error}</p>}
    {outcomes && <>
      {outcomes.reminder?.state === 'never-published' && <p role="status">{outcomes.reminder.intervalDays === 7 ? 'Weekly backup reminder' : 'Backup reminder'}: create your first external backup.</p>}
      {outcomes.reminder?.state === 'overdue' && <p role="status">Backup reminder: the last acknowledged publication is at least {outcomes.reminder.intervalDays} days old.</p>}
      {outcomes.reminder?.state === 'current' && outcomes.reminder.dueAt && <p>Next backup reminder: <time dateTime={new Date(outcomes.reminder.dueAt * 1000).toISOString()}>{new Date(outcomes.reminder.dueAt * 1000).toLocaleString()}</time>.</p>}
      {outcomes.reminder?.state === 'unavailable' && <p>Backup reminder timing is unavailable. Check backup status before relying on it.</p>}
      {outcomes.workerCompletion === 'refused' && <p role="status">The backup repository was refused before runtime acquisition. Workload restoration remains paused; no new snapshot was published.</p>}
      {outcomes.workerCompletion === 'uncertain' && <p role="status">Backup worker completion is uncertain. New work remains paused until reconciliation.</p>}
      {outcomes.current?.status === 'unknown' && <p role="status">The latest backup outcome is uncertain. It needs reconciliation before another backup can run.</p>}
      {outcomes.lastPublished ? <p>Last acknowledged publication: <time dateTime={new Date(outcomes.lastPublished.publishedAt * 1000).toISOString()}>{new Date(outcomes.lastPublished.publishedAt * 1000).toLocaleString()}</time>.</p> : <p>No acknowledged external backup is recorded.</p>}
      <p>Acknowledged publication does not verify repository health or a successful restore. Restoring backup data and replacement-host recovery are not yet available in this build.</p>
    </>}
    {outcomes?.resumeJobId && <div><p>{outcomes.workerCompletion === 'refused' ? 'The backup worker stopped before runtime acquisition. Workloads still need restoration.' : 'The backup worker has completed and released its runtime barrier. Workloads still need restoration.'}</p><button disabled={busy || resuming} onClick={() => void resume()}>{resuming ? 'Verifying and resuming…' : 'Verify passkey and resume workloads'}</button></div>}
    {resumeError && <p role="alert" className="form-error">{resumeError}</p>}
    {resumeMessage && <p role="status">{resumeMessage}</p>}
    {outcomes?.reminder && <form onSubmit={e => { e.preventDefault(); void saveReminder() }}><label>Backup reminder interval (days)<input type="number" required min={1} max={90} value={reminderDays} onChange={e => setReminderDays(e.target.value)} disabled={busy || resuming} /></label><button disabled={busy || resuming}>Save reminder interval</button><p>Reminders appear here when you check status. They do not run unattended backups.</p></form>}
    {reminderError && <p role="alert" className="form-error">{reminderError}</p>}
    {reminderSaved && <p role="status">Backup reminder interval saved.</p>}
    <BackupStart />
    {observedAt && <p className="timestamp">Last observed {observedAt.toLocaleTimeString()}</p>}
  </section>
}
