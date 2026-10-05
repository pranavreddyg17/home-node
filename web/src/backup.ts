import { APIError } from './api'
import { approveExactAction } from './approvals'

// Owns credential bytes and clears them on success, refusal, or passkey cancel.
export async function startBackup(repositoryId: string, password: Uint8Array): Promise<string> {
  let frame: Uint8Array | undefined
  try {
    if (!/^[a-f0-9]{64}$/.test(repositoryId) || password.length < 1 || password.length > 8192 || password.some(byte => byte === 0 || byte === 10 || byte === 13)) throw new Error('Enter a valid repository password (up to 8192 UTF-8 bytes).')
    const exactBody = JSON.stringify({ repositoryId })
    const key = crypto.randomUUID()
    const approval = await approveExactAction('/backups', exactBody, { 'Idempotency-Key': key })
    const metadata = new TextEncoder().encode(exactBody)
    frame = new Uint8Array(4 + metadata.length + password.length)
    new DataView(frame.buffer).setUint32(0, metadata.length, false)
    frame.set(metadata, 4); frame.set(password, 4 + metadata.length)
    password.fill(0)
    const response = await fetch('/api/v1/backups', { method: 'POST', credentials: 'same-origin', cache: 'no-store', headers: { 'Content-Type': 'application/vnd.homenode.backup-credential', 'Idempotency-Key': key, 'X-Action-Approval': approval }, body: frame.buffer as ArrayBuffer })
    const value = await response.json()
    if (!response.ok) throw new APIError(response.status, value.error?.code ?? 'REQUEST_FAILED', value.error?.message ?? 'Backup could not start. Refresh status before trying again.')
    if (typeof value.jobId !== 'string' || !value.jobId) throw new Error('Backup acknowledgement is unavailable. Refresh status before trying again.')
    return value.jobId
  } finally { password.fill(0); frame?.fill(0) }
}

export type SnapshotPage = { snapshots: { id: string; createdAt: string }[]; next: string }

export async function listBackupSnapshots(repositoryId: string, cursor: string, password: Uint8Array, signal: AbortSignal): Promise<SnapshotPage> {
  let frame: Uint8Array | undefined
  try {
    if (!/^[a-f0-9]{64}$/.test(repositoryId) || (cursor !== '' && !/^[a-f0-9]{64}$/.test(cursor)) || password.length < 1 || password.length > 8192 || password.some(byte => byte === 0 || byte === 10 || byte === 13)) throw new Error('Enter a valid repository password (up to 8192 UTF-8 bytes).')
    const metadata = new TextEncoder().encode(JSON.stringify({ repositoryId, cursor }))
    frame = new Uint8Array(4 + metadata.length + password.length)
    new DataView(frame.buffer).setUint32(0, metadata.length, false)
    frame.set(metadata, 4); frame.set(password, 4 + metadata.length)
    password.fill(0)
    const response = await fetch('/api/v1/backups/snapshots', { method: 'POST', credentials: 'same-origin', cache: 'no-store', signal, headers: { 'Content-Type': 'application/vnd.homenode.backup-credential' }, body: frame.buffer as ArrayBuffer })
    const value = await response.json()
    if (!response.ok) throw new APIError(response.status, value.error?.code ?? 'REQUEST_FAILED', value.error?.message ?? 'Snapshots could not be listed.')
    if (!Array.isArray(value.snapshots) || value.snapshots.length > 25 || typeof value.next !== 'string' || (value.next !== '' && !/^[a-f0-9]{64}$/.test(value.next))) throw new Error('The snapshot page could not be verified.')
    const ids = new Set<string>()
    for (const snapshot of value.snapshots) {
      if (!snapshot || typeof snapshot.id !== 'string' || !/^[a-f0-9]{64}$/.test(snapshot.id) || ids.has(snapshot.id) || typeof snapshot.createdAt !== 'string' || !Number.isFinite(Date.parse(snapshot.createdAt))) throw new Error('The snapshot page could not be verified.')
      ids.add(snapshot.id)
    }
    if (value.next !== '' && (value.snapshots.length !== 25 || value.snapshots[24].id !== value.next)) throw new Error('The snapshot continuation could not be verified.')
    return value as SnapshotPage
  } finally { password.fill(0); frame?.fill(0) }
}
