import { useCallback } from 'react'
import { useStore } from 'react-redux'

import type { AuthState } from '@/features/auth/authSlice'

import { refreshSession } from './baseQuery'
import type { FileInfo } from './generated'

/** UploadError, sunucunun reddettiği yüklemedir: problem yanıtının kodu ve açıklaması. */
export class UploadError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, detail: string) {
    super(detail)
    this.status = status
    this.code = code
  }
}

type Progress = (loaded: number, total: number) => void

function send(file: File, token: string | null, onProgress?: Progress, signal?: AbortSignal) {
  return new Promise<{ status: number; body: unknown }>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', '/api/v1/files')
    xhr.setRequestHeader('X-Agora-Client', 'web')
    if (token) {
      xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    }
    xhr.upload.onprogress = (e) => onProgress?.(e.loaded, e.lengthComputable ? e.total : file.size)
    xhr.onload = () => {
      let body: unknown
      try {
        body = xhr.responseText ? JSON.parse(xhr.responseText) : null
      } catch {
        body = null
      }
      resolve({ status: xhr.status, body })
    }
    xhr.onerror = () => reject(new UploadError(0, 'NETWORK', 'Bağlantı kesildi.'))
    xhr.onabort = () => reject(new DOMException('Yükleme iptal edildi.', 'AbortError'))
    signal?.addEventListener('abort', () => xhr.abort())
    const form = new FormData()
    form.append('file', file)
    xhr.send(form)
  })
}

/**
 * useUpload, dosyayı ilerleme bildirerek yükler (XMLHttpRequest: fetch yükleme ilerlemesi
 * vermez). Access token'ın süresi dolmuşsa oturum sessizce yenilenip bir kez daha denenir.
 */
export function useUpload() {
  const store = useStore<{ auth: AuthState }>()
  return useCallback(
    async (file: File, onProgress?: Progress, signal?: AbortSignal): Promise<FileInfo> => {
      let res = await send(file, store.getState().auth.accessToken, onProgress, signal)
      if (res.status === 401 && (await refreshSession(store.dispatch, { retry: true })) === 'ok') {
        res = await send(file, store.getState().auth.accessToken, onProgress, signal)
      }
      if (res.status === 201) {
        return res.body as FileInfo
      }
      const problem = (res.body ?? {}) as { code?: string; detail?: string }
      throw new UploadError(
        res.status,
        problem.code ?? 'UPLOAD_FAILED',
        problem.detail ?? 'Dosya yüklenemedi.',
      )
    },
    [store],
  )
}

/** formatBytes, boyutu okunur biçimde yazar (1,2 MB). */
export function formatBytes(n: number, locale = 'tr-TR'): string {
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toLocaleString(locale, { maximumFractionDigits: i === 0 ? 0 : 1 })} ${units[i]}`
}
