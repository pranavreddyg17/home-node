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

export type SnapshotPreview = { snapshotId: string; createdAt: string; release: string; catalogVersion: number; validation: 'metadata-compatible'; files: { workload: string; bytes: number }[] }

export async function previewBackupSnapshot(repositoryId: string, snapshotId: string, password: Uint8Array, signal: AbortSignal): Promise<SnapshotPreview> {
  let frame: Uint8Array | undefined
  try {
    if (!/^[a-f0-9]{64}$/.test(repositoryId) || !/^[a-f0-9]{64}$/.test(snapshotId) || password.length < 1 || password.length > 8192 || password.some(byte => byte === 0 || byte === 10 || byte === 13)) throw new Error('Enter a valid repository password (up to 8192 UTF-8 bytes).')
    const metadata = new TextEncoder().encode(JSON.stringify({ repositoryId, snapshotId }))
    frame = new Uint8Array(4 + metadata.length + password.length)
    new DataView(frame.buffer).setUint32(0, metadata.length, false)
    frame.set(metadata, 4); frame.set(password, 4 + metadata.length); password.fill(0)
    const response = await fetch('/api/v1/backups/preview', { method: 'POST', credentials: 'same-origin', cache: 'no-store', signal, headers: { 'Content-Type': 'application/vnd.homenode.backup-credential' }, body: frame.buffer as ArrayBuffer })
    const value = await response.json()
    if (!response.ok) throw new APIError(response.status, value.error?.code ?? 'REQUEST_FAILED', value.error?.message ?? 'The snapshot could not be inspected.')
    if (value.snapshotId !== snapshotId || value.validation !== 'metadata-compatible' || typeof value.createdAt !== 'string' || !Number.isFinite(Date.parse(value.createdAt)) || typeof value.release !== 'string' || !/^[0-9][a-zA-Z0-9.+~-]{0,63}$/.test(value.release) || !Number.isSafeInteger(value.catalogVersion) || value.catalogVersion < 1 || !Array.isArray(value.files) || value.files.length < 1 || value.files.length > 3) throw new Error('The compatibility preview could not be verified.')
    const seen = new Set<string>()
    for (const file of value.files) {
      if (!file || !['management', 'files', 'ai'].includes(file.workload) || seen.has(file.workload) || !Number.isSafeInteger(file.bytes) || file.bytes <= 0 || file.bytes > (file.workload === 'management' ? 256 * 2 ** 20 : 512 * 2 ** 30)) throw new Error('The compatibility preview could not be verified.')
      seen.add(file.workload)
    }
    if (!seen.has('management')) throw new Error('The compatibility preview could not be verified.')
    return value as SnapshotPreview
  } finally { password.fill(0); frame?.fill(0) }
}
