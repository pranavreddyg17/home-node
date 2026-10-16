import { startAuthentication, type PublicKeyCredentialRequestOptionsJSON } from '@simplewebauthn/browser'
import { APIError } from './api'

async function send<R,>(target: string, bytes: string, headers: Record<string, string> = {}): Promise<R> {
  // Bound each network exchange independently of the owner's passkey prompt.
  const signal = AbortSignal.timeout(30000)
  try {
    const response = await fetch(`/api/v1${target}`, { method: 'POST', credentials: 'same-origin', cache: 'no-store', signal, headers: { 'Content-Type': 'application/json', ...headers }, body: bytes })
    const value = await response.json()
    if (!response.ok) throw new APIError(response.status, value.error?.code ?? 'REQUEST_FAILED', value.error?.message ?? 'The action failed.')
    return value as R
  } catch (error) {
    if (signal.aborted && signal.reason instanceof DOMException && signal.reason.name === 'TimeoutError') {
      throw new APIError(0, 'ACTION_RESULT_UNKNOWN', 'The request timed out. Check its current status before retrying.')
    }
    throw error
  }
}

export async function approveExactAction(path: string, exactBody: string, requestHeaders: Record<string, string> = {}): Promise<string> {
  const begin = await send<{ options: { publicKey: PublicKeyCredentialRequestOptionsJSON }; challengeToken: string }>(`${path}/approval`, exactBody, requestHeaders)
  const assertion = await startAuthentication({ optionsJSON: begin.options.publicKey })
  const finish = await send<{ approvalToken: string }>('/auth/approval/finish', JSON.stringify(assertion), { 'X-Approval-Challenge': begin.challengeToken })
  return finish.approvalToken
}

export async function approvedAction<T>(path: string, body: unknown, requestHeaders: Record<string, string> = {}): Promise<T> {
 const exactBody=JSON.stringify(body)
 const token=await approveExactAction(path,exactBody,requestHeaders)
 return send<T>(path,exactBody,{...requestHeaders,'X-Action-Approval':token})
}
