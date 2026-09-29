export type Device = { id: string; name: string; capabilities: string[]; createdAt: number; revokedAt: number | null }
export type Session = { device: Device; verifiedAt: number; expiresAt: number }
export class APIError extends Error {
  constructor(public status: number, public code: string, message: string) { super(message) }
}
export async function api<T>(path: string, body?: unknown): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    method: body === undefined ? 'GET' : 'POST',
    credentials: 'same-origin', cache: 'no-store',
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const value = await response.json()
  if (!response.ok) throw new APIError(response.status, value.error?.code ?? 'REQUEST_FAILED', value.error?.message ?? `Request failed (${response.status})`)
  return value as T
}
export function message(error: unknown): string { return error instanceof Error ? error.message : 'The operation failed. Try again.' }
