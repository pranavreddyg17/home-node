import { approvedAction } from './approvals'
import { useEffect, useState, type ReactNode } from 'react'
import { startAuthentication, startRegistration } from '@simplewebauthn/browser'
import type { PublicKeyCredentialCreationOptionsJSON, PublicKeyCredentialRequestOptionsJSON } from '@simplewebauthn/browser'
import { api, APIError, message, type Device, type Session } from './api'

async function signIn() {
  const options = await api<{ publicKey: PublicKeyCredentialRequestOptionsJSON }>('/auth/login/begin', {})
  const response = await startAuthentication({ optionsJSON: options.publicKey })
  await api('/auth/login/finish', response)
  return api<Session>('/session')
}


export function IdentityBoundary({ children }: { children: (session: Session, logout: () => Promise<void>, verify: () => Promise<void>) => ReactNode }) {
  const [session, setSession] = useState<Session | null>(null)
  const [ready, setReady] = useState(false)
  const [claimed, setClaimed] = useState(false)
  const [development, setDevelopment] = useState(false)
  const [mode, setMode] = useState<'login' | 'register' | 'recover'>('login')
  const [name, setName] = useState('My device')
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const refresh = async () => {
    const status = await api<{ claimed: boolean; development: boolean }>('/setup')
    setClaimed(status.claimed); setDevelopment(status.development)
    if (!status.claimed) setMode('register')
    try { setSession(await api<Session>('/session')) } catch (e) { if (e instanceof APIError && e.status === 401) setSession(null); else throw e }
    setReady(true)
  }
  useEffect(() => { void refresh().catch(e => setError(message(e))) }, [])
  useEffect(() => {
    if (!session) return
    const timer = window.setInterval(() => { void api<Session>('/session').then(setSession).catch(e => { if (e instanceof APIError && e.status === 401) { setSession(null); setError('Your session ended. Sign in again.') } }) }, 15000)
    return () => clearInterval(timer)
  }, [session?.device.id])
  const logout = async () => { await api('/logout', {}); setSession(null) }
  const verify = async () => { setSession(await signIn()) }
  const submit = async () => {
    setBusy(true); setError('')
    try {
      if (mode === 'login') setSession(await signIn())
      else {
        let enrollmentCode = code.trim()
        if (mode === 'recover') enrollmentCode = (await api<{ code: string }>('/auth/recover', { code: enrollmentCode })).code
        const options = await api<{ publicKey: PublicKeyCredentialCreationOptionsJSON }>('/auth/register/begin', { code: enrollmentCode, name: name.trim() })
        const response = await startRegistration({ optionsJSON: options.publicKey })
        const result = await api<{ recoveryCodes: string[] | null }>('/auth/register/finish', response)
        setCodes(result.recoveryCodes ?? []); setCode(''); await refresh()
      }
    } catch (e) { setError(message(e)) } finally { setBusy(false) }
  }
  if (codes.length) return <div className="auth-shell"><section className="auth-card"><p className="eyebrow">RECOVERY</p><h1>Save these codes</h1><p>Each code works once to replace your passkey and revoke all existing devices. Store them outside this server. They will not be shown again.</p><ul className="recovery-codes">{codes.map(c => <li key={c}><code>{c}</code></li>)}</ul><button className="primary" onClick={() => setCodes([])}>I saved my recovery codes</button></section></div>
  if (session) return children(session, logout, verify)
  return <div className="auth-shell"><section className="auth-card"><div className="brand"><span className="brand-symbol">H<span /></span><span>HomeNode<small>PERSONAL COMPUTE</small></span></div><p className="eyebrow">YOUR PRIVATE SERVER</p><h1>{!ready ? 'Connecting…' : mode === 'login' ? 'Welcome home' : mode === 'recover' ? 'Recover access' : claimed ? 'Pair this device' : 'Set up HomeNode'}</h1>
    {development && <p className="inline-notice">Local development · Workload isolation is not yet qualified.</p>}
    {error && <p className="form-error" role="alert">{error}</p>}
    {!ready ? <button onClick={() => void refresh().catch(e => setError(message(e)))}>Retry connection</button> : <form onSubmit={e => { e.preventDefault(); void submit() }}>
      {mode !== 'login' ? <><p>{mode === 'recover' ? 'A successful recovery replaces existing device access. Use one of your saved recovery codes.' : claimed ? 'Enter the invitation code from an administrator device. It expires after five minutes.' : 'Enter the enrollment code from your server’s local console. Then create a passkey to protect access.'}</p><label>Device name<input value={name} maxLength={80} onChange={e => setName(e.target.value)} required autoComplete="off" /></label><label>{mode === 'recover' ? 'Recovery code' : 'Enrollment code'}<input value={code} onChange={e => setCode(e.target.value)} required autoComplete="off" spellCheck={false} /></label></> : <p>Use your passkey to open your server. Your browser will ask you to verify your identity.</p>}
      <button className="primary" disabled={busy}>{busy ? 'Waiting for verification…' : mode === 'login' ? 'Sign in with a passkey' : 'Create a passkey'}</button>
    </form>}
    {ready && claimed && <div className="auth-links">{mode !== 'login' && <button onClick={() => setMode('login')}>Sign in instead</button>}{mode !== 'register' && <button onClick={() => setMode('register')}>Pair a device</button>}{mode !== 'recover' && <button onClick={() => setMode('recover')}>Use a recovery code</button>}</div>}
  </section></div>
}

export function Devices({ session, verify }: { session: Session; verify: () => Promise<void> }) {
  const [devices, setDevices] = useState<Device[]>([])
  const [name, setName] = useState('My phone')
  const [caps, setCaps] = useState(['files', 'jobs', 'ai'])
  const [invitation, setInvitation] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const refresh = () => api<Device[]>('/devices').then(setDevices)
  useEffect(() => { void refresh().catch(e => setError(message(e))) }, [])
  const run = async (work: () => Promise<void>) => { setBusy(true); setError(''); try { await work(); await refresh() } catch (e) { setError(message(e)) } finally { setBusy(false) } }
  if (!session.device.capabilities.includes('admin')) return <section className="panel"><h2>Device administration</h2><p>This device does not have administrative access.</p></section>
  return <div className="stack">{error && <p role="alert" className="form-error">{error}</p>}<section className="panel"><div className="section-heading"><h2>Your devices</h2><button disabled={busy} onClick={() => void run(verify)}>Verify passkey again</button></div><p>Passkeys may sync through your password manager. A listed device represents its registered passkey and sessions, rather than verified physical hardware.</p>{devices.map(d => <div className="device-row" key={d.id}><div><strong>{d.name} {d.id === session.device.id && '(this session)'}</strong><p>{d.capabilities.join(' · ')} · {d.revokedAt ? 'Revoked' : 'Authorized'}</p></div>{!d.revokedAt && <button className="danger" disabled={busy} onClick={() => { if (window.confirm(`Revoke ${d.name}? Its passkey and sessions will lose access.`)) void run(async () => { await approvedAction(`/devices/${d.id}/revoke`, {}) }) }}>Revoke</button>}</div>)}</section>
    <section className="panel"><h2>Pair another device</h2><form onSubmit={e => { e.preventDefault(); void run(async () => { const result = await approvedAction<{ code: string }>('/devices/pair', { name, capabilities: caps }); setInvitation(result.code) }) }}><label>Device name<input required maxLength={80} value={name} onChange={e => setName(e.target.value)} /></label><fieldset><legend>Access granted</legend>{['files', 'jobs', 'ai', 'admin'].map(cap => <label className="check-field" key={cap}><input type="checkbox" checked={caps.includes(cap)} onChange={e => setCaps(e.target.checked ? [...caps, cap] : caps.filter(c => c !== cap))} />{cap === 'admin' ? 'Administration (pair and revoke devices)' : cap.toUpperCase()}</label>)}</fieldset><button className="primary" disabled={busy || !caps.length}>Create invitation</button></form>{invitation && <div className="invitation"><p>Open this server on your other device, choose “Pair a device”, and enter this code within five minutes.</p><code>{invitation}</code><button onClick={() => setInvitation('')}>Hide code</button></div>}</section>
  </div>
}

export function Settings({ logout }: { logout: () => Promise<void> }) {
  const [error, setError] = useState('')
  const [diagnostics, setDiagnostics] = useState('')
  const preview = async () => { try { setDiagnostics(JSON.stringify(await api('/diagnostics'), null, 2)) } catch (e) { setError(message(e)) } }
  const download = () => { const url = URL.createObjectURL(new Blob([diagnostics], { type: 'application/json' })); const a = document.createElement('a'); a.href = url; a.download = 'homenode-diagnostics.json'; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000) }
  return <div className="stack">{error && <p className="form-error" role="alert">{error}</p>}<section className="panel"><h2>Diagnostics</h2><p>Preview the report before exporting it. File contents, chats, credentials, device names, and host storage paths are excluded.</p><button onClick={() => void preview()}>Preview report</button>{diagnostics && <><pre className="diagnostics">{diagnostics}</pre><button onClick={download}>Download report</button></>}</section><section className="panel"><h2>Session</h2><p>Sessions expire after 30 minutes of inactivity, or 12 hours after sign-in.</p><button onClick={() => void logout().catch(e => setError(message(e)))}>Sign out</button></section></div>
}
