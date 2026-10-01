import { useCallback, useEffect, useState } from 'react'
import { IdentityBoundary, Devices, Settings } from './Identity'
import { api, type Session } from './api'
import { Apps, Files, Jobs, AI } from './Workloads'

type Status = 'pass' | 'warn' | 'fail'

type Check = {
  id: string
  title: string
  status: Status
  detail: string
  remediation?: string
}

type Report = {
  generatedAt: string
  host: {
    os: string
    architecture: string
    distribution?: string
    distributionVersion?: string
    memoryBytes?: number
    availableDiskBytes?: number
    diskProbePath?: string
  }
  checks: Check[]
  prerequisitesMet: boolean
  executionEnabled: boolean
}

type LoadState =
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
  | { kind: 'loaded'; report: Report }

function CheckMark({ status }: { status: Status }) {
  return <span className={`check-mark ${status}`} aria-hidden="true">{status === 'pass' ? '✓' : status === 'warn' ? '!' : '×'}</span>
}

function Workspace({ session, logout, verify }: { session: Session; logout: () => Promise<void>; verify: () => Promise<void> }) {
  const [state, setState] = useState<LoadState>({ kind: 'loading' })
  const [selected, setSelected] = useState('Overview')
  const [runtime, setRuntime] = useState({ configured: false, running: 0 })

  const refresh = useCallback(async () => {
    setState({ kind: 'loading' })
    try {
      const response = await fetch('/api/v1/host/report', { cache: 'no-store' })
      if (!response.ok) throw new Error(`Host service returned ${response.status}`)
      const report = (await response.json()) as Report
      if (!Array.isArray(report.checks)) throw new Error('Invalid report from host service')
      const apps = await api<{ apps: { state: string }[]; runtimeConfigured: boolean }>('/apps')
      setRuntime({ configured: apps.runtimeConfigured, running: apps.apps.filter(a => a.state === 'running').length })
      setState({ kind: 'loaded', report })
    } catch (error) {
      setState({ kind: 'error', message: error instanceof Error ? error.message : 'Could not read host status' })
    }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  const report = state.kind === 'loaded' ? state.report : null
  const failures = report?.checks.filter((check) => check.status === 'fail').length ?? 0
  const positives = report?.checks.filter((check) => check.status === 'pass').length ?? 0
  const hostName = report?.host.distribution
    ? `${report.host.distribution} ${report.host.distributionVersion ?? ''}`
    : report?.host.os ?? 'Host not detected'

  return (
    <div className="app-shell">
      <aside className="sidebar" aria-label="Main navigation">
        <div className="brand"><span className="brand-symbol">H<span /></span><span>HomeNode<small>PERSONAL COMPUTE</small></span></div>
        <div className="nav-heading">WORKSPACE</div>
        <nav>
          {['Overview', 'Host checks', 'Apps', 'AI', 'Jobs', 'Files', 'Devices', 'Settings'].map((item) => (
            <button key={item} type="button" className={`nav-item ${selected === item ? 'active' : ''}`} onClick={() => setSelected(item)} aria-current={selected === item ? 'page' : undefined}>
              <span className="nav-glyph" aria-hidden="true">{({ Overview: '◫', 'Host checks': '◇', Apps: '▦', AI: '✳', Jobs: '≡', Files: '▤', Devices: '⌘', Settings: '⚙' } as Record<string, string>)[item]}</span>{item}

            </button>
          ))}
        </nav>
        <div className="sidebar-bottom"><span className="tiny-dot" /> {session.device.name} <small>Passkey authenticated</small></div>
      </aside>

      <div className="main-area">
        <header className="topbar"><div className="breadcrumb">Workspace <span>/</span> {selected}</div><div className="top-meta"><span className="top-version">DEVELOPMENT PREVIEW</span><span className="top-avatar" aria-hidden="true">H</span></div></header>
        <main>
          <div className="heading-row"><div><p className="eyebrow">SERVER OVERVIEW</p><h1>{selected === 'Overview' ? 'Your home server' : selected}</h1><p className="lede">A clear view of what this machine can support.</p></div><button type="button" className="refresh-button" onClick={() => void refresh()} disabled={state.kind === 'loading'}>↻ <span>Run checks</span></button></div>

          {selected === 'AI' ? <AI verify={verify} /> : selected === 'Jobs' ? <Jobs /> : selected === 'Apps' ? <Apps session={session} verify={verify} /> : selected === 'Files' ? <Files /> : selected === 'Devices' ? <Devices session={session} verify={verify} /> : selected === 'Settings' ? <Settings logout={logout} session={session} /> : selected !== 'Overview' && selected !== 'Host checks' ? (
            <section className="empty-screen"><span className="empty-icon" aria-hidden="true">◇</span><h2>{selected} is being built</h2><p>The host must pass isolation checks before workloads and device access can be enabled. This screen will be connected to verified services as they are implemented.</p><button type="button" onClick={() => setSelected('Host checks')}>View host checks →</button></section>
          ) : state.kind === 'loading' ? (
            <section className="status-panel" role="status">Checking this machine…</section>
          ) : state.kind === 'error' ? (
            <section className="status-panel error-panel" role="alert"><strong>Host service is unavailable</strong><p>{state.message}</p><p>Start the local service with <code>go run ./cmd/homenode serve</code>, then run the checks again.</p></section>
          ) : report && (
            <>
              <section className="hero-panel" aria-label="Host status"><div className="hero-main"><div className="hero-icon" aria-hidden="true">⌂</div><div><span className="hero-kicker">THIS MACHINE</span><h2>{hostName}<span className="os-arch"> · {report.host.architecture}</span></h2><p>{report.prerequisitesMet ? 'Basic prerequisites detected. Hardware qualification remains a separate release gate.' : `${failures} prerequisite${failures === 1 ? '' : 's'} need attention before workload execution.`}</p></div></div><span className={`hero-badge ${report.prerequisitesMet ? 'amber' : 'red'}`}>{report.prerequisitesMet ? 'PREREQUISITES MET' : 'NOT READY'}</span></section>

              <div className="metric-grid"><div className="metric"><span>CHECKS PASSED</span><strong>{positives}<em> / {report.checks.length}</em></strong><small>Host prerequisites</small></div><div className="metric"><span>RUNNING APPS</span><strong>{runtime.running}</strong><small>{runtime.configured ? 'Workload services configured' : 'Workload services not configured'}</small></div><div className="metric"><span>AVAILABLE STORAGE</span><strong>{report.host.availableDiskBytes ? `${(report.host.availableDiskBytes / 2 ** 30).toFixed(1)} GB` : 'Unknown'}</strong><small>{report.host.diskProbePath || 'Future data volume'}</small></div></div>

              <div className="content-grid"><section className="checks-panel"><div className="section-heading"><div><p className="eyebrow">PREFLIGHT</p><h2>Host checks</h2></div><span>{report.checks.length} CHECKS</span></div><div className="checks-list">{report.checks.map((check) => <div className="check-row" key={check.id}><CheckMark status={check.status} /><div><strong>{check.title}</strong><p>{check.detail}</p>{check.status !== 'pass' && check.remediation && <p className="remediation">{check.remediation}</p>}</div><span className={`check-label ${check.status}`}>{check.status.toUpperCase()}</span></div>)}</div></section>
                <aside className="next-panel"><p className="eyebrow">BUILD STATUS</p><h2>What comes next</h2><p>Passkey identity, isolated workload services, and resumable file transfers are implemented. Physical-host qualification and the remaining product workflows are in progress.</p><div className="next-divider" /><div className="next-item"><span className="step-number">01</span><div><strong>Qualify the host</strong><small>Confirm KVM, libvirt, AppArmor, memory and storage.</small></div></div><div className="next-item"><span className="step-number">02</span><div><strong>Prove isolation</strong><small>Test that neither VM can reach host data or the other VM.</small></div></div><div className="next-item"><span className="step-number">03</span><div><strong>Enable workloads</strong><small>Only after enforcement and recovery are verified.</small></div></div><div className="notice"><span aria-hidden="true">●</span><span>Production qualification is still pending.</span></div></aside></div>
              <p className="timestamp">Checked {new Date(report.generatedAt).toLocaleString()} · This report measures prerequisites, not a complete security certification.</p>
            </>
          )}
        </main>
      </div>
    </div>
  )
}

export default function App() { return <IdentityBoundary>{(session, logout, verify) => <Workspace session={session} logout={logout} verify={verify} />}</IdentityBoundary> }
