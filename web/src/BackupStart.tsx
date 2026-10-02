import { useEffect, useRef, useState } from 'react'
import { api, message } from './api'
import { startBackup } from './backup'

type Configuration = { enabled: boolean; repositoryId: string }
export function BackupStart() {
  const [configuration, setConfiguration] = useState<Configuration | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [job, setJob] = useState('')
  const password = useRef<HTMLInputElement>(null)
  useEffect(() => {
    let active = true
    void api<Configuration>('/backups/configuration').then(value => { if (active) setConfiguration(value) }).catch(e => { if (active) setError(message(e)) })
    return () => { active = false; if (password.current) password.current.value = '' }
  }, [])
  const start = async () => {
    if (!configuration?.enabled || !password.current || busy) return
    const secret = new TextEncoder().encode(password.current.value)
    password.current.value = ''
    setBusy(true); setError(''); setJob('')
    try { setJob(await startBackup(configuration.repositoryId, secret)) }
    catch (e) { setError(message(e)) }
    finally { secret.fill(0); setBusy(false) }
  }
  return <div>
    {configuration?.enabled ? <form onSubmit={e => { e.preventDefault(); void start() }}>
      <p>Attach the registered drive. Backup pauses workloads while their data is copied. Keep the repository password and recovery material somewhere else.</p>
      <label>Repository password<input ref={password} type="password" autoComplete="off" required maxLength={8192} disabled={busy} /></label>
      <button className="primary" disabled={busy}>{busy ? 'Verifying and starting…' : 'Verify passkey and start backup'}</button>
      <p>A connected writable backup drive remains vulnerable to host compromise. Safely disconnect it after backup completes.</p>
    </form> : configuration && <p>External backup execution has not been configured on this server.</p>}
    {job && <p role="status">Backup job {job} started. Refresh backup status to check its outcome.</p>}
    {error && <p role="alert" className="form-error">{error}</p>}
  </div>
}
