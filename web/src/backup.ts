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
