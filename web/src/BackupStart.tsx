import { useEffect, useRef, useState } from 'react'
import { api, message } from './api'
import { startBackup } from './backup'

type Configuration = { enabled: boolean; repositoryId: string; availability: 'available' | 'paused' | 'not-configured' | 'unavailable' }
export function BackupStart() {
  const [configuration, setConfiguration] = useState<Configuration | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [job, setJob] = useState('')
  const [checking,setChecking]=useState(false)
  const requestId=useRef(0)
  const password = useRef<HTMLInputElement>(null)
  const refreshConfiguration = async () => {
    const id=++requestId.current
    if(password.current) password.current.value=''
    setChecking(true);setConfiguration(null);setError('')
    try {const value=await api<Configuration>('/backups/configuration');if(id===requestId.current)setConfiguration(value)}
    catch(e){if(id===requestId.current)setError(message(e))}
    finally{if(id===requestId.current)setChecking(false)}
  }
  useEffect(() => {
    void refreshConfiguration()
    return () => {requestId.current++;if(password.current)password.current.value=''}
  }, [])
  const start = async () => {
    if (!configuration?.enabled || configuration.availability !== 'available' || !password.current || busy || checking) return
    const secret = new TextEncoder().encode(password.current.value)
    password.current.value = ''
    setBusy(true); setError(''); setJob('')
    try { setJob(await startBackup(configuration.repositoryId, secret)) }
    catch (e) { setError(message(e)) }
    finally { secret.fill(0); setBusy(false) }
  }
  return <div>
    <button disabled={busy || checking} onClick={()=>void refreshConfiguration()}>Check backup availability</button>
    {checking && <p role="status">Checking whether a backup can start…</p>}
    {configuration?.enabled && configuration.availability === 'available' ? <form onSubmit={e => { e.preventDefault(); void start() }}>
      <p>Attach the registered drive. Backup pauses workloads while their data is copied. Keep the repository password and recovery material somewhere else.</p>
      <label>Repository password<input ref={password} type="password" autoComplete="off" required maxLength={8192} disabled={busy} /></label>
      <button className="primary" disabled={busy}>{busy ? 'Verifying and starting…' : 'Verify passkey and start backup'}</button>
      <p>A connected writable backup drive remains vulnerable to host compromise. Safely disconnect it after backup completes.</p>
    </form> : configuration && <p>{configuration.enabled ? (configuration.availability === 'paused' ? "New backups are paused while work or maintenance is active. Check availability after it finishes." : "Backup availability could not be verified. Check again before entering the repository password.") : "External backup execution has not been configured on this server."}</p>}
    {job && <p role="status">Backup job {job} started. Refresh backup status to check its outcome.</p>}
    {error && <p role="alert" className="form-error">{error}</p>}
  </div>
}
