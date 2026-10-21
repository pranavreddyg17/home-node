export type Device = { id: string; name: string; capabilities: string[]; createdAt: number; revokedAt: number | null }
export type Session = { device: Device; verifiedAt: number; expiresAt: number }
export class APIError extends Error {
  constructor(public status: number, public code: string, message: string) { super(message) }
}
export async function api<T>(path: string, body?: unknown, cancellation?: AbortSignal, headers: Record<string, string> = {}): Promise<T> {
  // Whole-object verification may use 85 seconds in the transfer service.
  // Bound a lost mutation response without cutting that operation short.
  const deadline = AbortSignal.timeout(body === undefined ? 15000 : 120000)
  const signal = cancellation ? AbortSignal.any([deadline, cancellation]) : deadline
  try {
    const response = await fetch(`/api/v1${path}`, {
      method: body === undefined ? 'GET' : 'POST',
      credentials: 'same-origin', cache: 'no-store', signal,
      headers: { ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...headers },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const value = await response.json()
    if (!response.ok) throw new APIError(response.status, value.error?.code ?? 'REQUEST_FAILED', value.error?.message ?? `Request failed (${response.status})`)
    return value as T
  } catch (error) {
    // Body reads may report AbortError rather than TimeoutError; the signal's
    // reason remains authoritative for the same whole-request deadline.
    if (signal?.aborted && signal.reason instanceof DOMException && signal.reason.name === 'TimeoutError') {
      throw new APIError(0, 'HOST_TIMEOUT', 'The server did not respond. Check its power and your private network connection, then retry.')
    }
    throw error
  }
}
export function message(error: unknown): string { return error instanceof Error ? error.message : 'The operation failed. Try again.' }
