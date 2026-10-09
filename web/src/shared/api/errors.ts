import type { SerializedError } from '@reduxjs/toolkit'

import type { ApiError } from './baseQuery'
import type { Problem } from './generated'

export type AnyError = ApiError | SerializedError | undefined

function isApiError(error: AnyError): error is ApiError {
  return error !== undefined && 'status' in error
}

/** problemOf, hatadaki RFC 9457 problem gövdesini döndürür. */
export function problemOf(error: AnyError): Problem | undefined {
  if (!isApiError(error) || typeof error.status !== 'number') {
    return undefined
  }
  const data = error.data as Partial<Problem> | undefined
  return data && typeof data === 'object' && typeof data.status === 'number' ? (data as Problem) : undefined
}

/** errorCode, sunucunun makine okunur hata kodudur (ör. INVALID_CREDENTIALS). */
export function errorCode(error: AnyError): string | undefined {
  return problemOf(error)?.code
}

export function httpStatus(error: AnyError): number | undefined {
  return isApiError(error) && typeof error.status === 'number' ? error.status : undefined
}

export function retryAfter(error: AnyError): number | undefined {
  return isApiError(error) ? error.retryAfter : undefined
}

/** isNetworkError, sunucuya hiç ulaşılamadığını söyler. */
export function isNetworkError(error: AnyError): boolean {
  return isApiError(error) && (error.status === 'FETCH_ERROR' || error.status === 'TIMEOUT_ERROR')
}

export type FieldErrors = Record<string, { message: string; code?: string }[]>

/** fieldErrors, VALIDATION_FAILED yanıtındaki alan hatalarını alana göre gruplar. */
export function fieldErrors(error: AnyError): FieldErrors {
  const out: FieldErrors = {}
  for (const e of problemOf(error)?.errors ?? []) {
    ;(out[e.field] ??= []).push(e.code ? { message: e.message, code: e.code } : { message: e.message })
  }
  return out
}
