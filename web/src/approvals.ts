import { startAuthentication, type PublicKeyCredentialRequestOptionsJSON } from '@simplewebauthn/browser'
import { APIError } from './api'

export async function approvedAction<T>(path: string, body: unknown, requestHeaders: Record<string, string> = {}): Promise<T> {
  const exactBody = JSON.stringify(body)
  const send = async <R,>(target: string, bytes: string, headers: Record<string, string> = {}): Promise<R> => {
    const response = await fetch(`/api/v1${target}`, { method: 'POST', credentials: 'same-origin', cache: 'no-store', headers: { 'Content-Type': 'application/json', ...headers }, body: bytes })
    const value = await response.json()
    if (!response.ok) throw new APIError(response.status, value.error?.code ?? 'REQUEST_FAILED', value.error?.message ?? 'The action failed.')
    return value as R
  }
  const begin = await send<{ options: { publicKey: PublicKeyCredentialRequestOptionsJSON }; challengeToken: string }>(`${path}/approval`, exactBody, requestHeaders)
  const assertion = await startAuthentication({ optionsJSON: begin.options.publicKey })
  const finish = await send<{ approvalToken: string }>('/auth/approval/finish', JSON.stringify(assertion), { 'X-Approval-Challenge': begin.challengeToken })
  return send<T>(path, exactBody, { ...requestHeaders, 'X-Action-Approval': finish.approvalToken })
}
