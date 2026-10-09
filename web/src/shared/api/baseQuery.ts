import {
  fetchBaseQuery,
  type BaseQueryFn,
  type FetchArgs,
  type FetchBaseQueryError,
} from '@reduxjs/toolkit/query/react'

import {
  accountDisabled,
  passwordChangeRequired,
  sessionExpired,
  signedIn,
  type AuthState,
  type SessionTokens,
} from '@/features/auth/authSlice'

/** ApiError, RTK Query hata nesnesidir. retryAfter, 429 yanıtlarındaki Retry-After saniyesidir. */
export type ApiError = FetchBaseQueryError & { retryAfter?: number }

// Mutlak adres: tarayıcıda aynı origin, testlerde (Node fetch) de çözülebilir bir URL.
const origin = () => window.location.origin

const rawBaseQuery = fetchBaseQuery({
  baseUrl: '',
  prepareHeaders: (headers, { getState }) => {
    // Web istemcisi: refresh token çerezde taşınır. Başlık ayrıca tarayıcının başka
    // sitelerden gelen isteklerde CORS ön kontrolü yapmasını sağlar (CSRF koruması).
    headers.set('X-Agora-Client', 'web')
    const token = (getState() as { auth: AuthState }).auth.accessToken
    if (token && !headers.has('Authorization')) {
      headers.set('Authorization', `Bearer ${token}`)
    }
    return headers
  },
})

type RefreshResult = 'ok' | 'invalid' | 'error'
type Dispatch = (action: { type: string }) => unknown

let refreshing: Promise<RefreshResult> | null = null

const delay = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

async function postRefresh(): Promise<SessionTokens | 'invalid' | 'error'> {
  try {
    const res = await fetch(`${origin()}/api/v1/auth/refresh`, {
      method: 'POST',
      headers: { 'X-Agora-Client': 'web' },
      credentials: 'same-origin',
    })
    if (res.ok) {
      return (await res.json()) as SessionTokens
    }
    return res.status === 401 || res.status === 403 ? 'invalid' : 'error'
  } catch {
    return 'error'
  }
}

/**
 * refreshSession, refresh çereziyle yeni bir access token alır. Aynı anda gelen bütün
 * çağrılar tek bir isteği paylaşır: refresh token tek kullanımlıktır, iki paralel
 * yenileme ikincisini geçersiz kılardı.
 *
 * retry: başka bir sekme aynı anda yenileme yaptıysa bu sekmenin gönderdiği çerez az
 * önce kullanılmış olur ve sunucu reddeder. Tarayıcı yeni çerezi birazdan alacağı için
 * bir kez daha denenir.
 */
export function refreshSession(dispatch: Dispatch, { retry }: { retry: boolean }): Promise<RefreshResult> {
  refreshing ??= (async () => {
    try {
      let result = await postRefresh()
      if (result === 'invalid' && retry) {
        await delay(800)
        result = await postRefresh()
      }
      if (typeof result === 'object') {
        dispatch(signedIn(result))
        return 'ok'
      }
      return result
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

function problemCode(error: FetchBaseQueryError | undefined): string | undefined {
  const data = error?.data as { code?: unknown } | undefined
  return typeof data?.code === 'string' ? data.code : undefined
}

/**
 * baseQueryWithReauth, access token'ın süresi dolunca (401) sessizce yenileyip isteği
 * tekrarlar. Yenileme reddedilirse oturum düşer ve kullanıcı giriş ekranına yönlenir.
 */
export const baseQueryWithReauth: BaseQueryFn<string | FetchArgs, unknown, ApiError> = async (
  args,
  api,
  extra,
) => {
  const withOrigin = (a: string | FetchArgs): FetchArgs =>
    typeof a === 'string' ? { url: origin() + a } : { ...a, url: origin() + a.url }
  const path = typeof args === 'string' ? args : args.url

  let result = await rawBaseQuery(withOrigin(args), api, extra)

  const hadToken = (api.getState() as { auth: AuthState }).auth.accessToken !== null
  if (result.error?.status === 401 && hadToken && !path.startsWith('/api/v1/auth/')) {
    const refreshed = await refreshSession(api.dispatch, { retry: true })
    if (refreshed === 'ok') {
      result = await rawBaseQuery(withOrigin(args), api, extra)
    } else if (refreshed === 'invalid') {
      api.dispatch(sessionExpired())
    }
  }

  if (result.error?.status === 403) {
    const code = problemCode(result.error)
    if (code === 'PASSWORD_CHANGE_REQUIRED') {
      api.dispatch(passwordChangeRequired())
    } else if (code === 'ACCOUNT_DISABLED' && !path.startsWith('/api/v1/auth/')) {
      api.dispatch(accountDisabled())
    }
  }

  if (result.error) {
    const retryAfter = Number(result.meta?.response?.headers.get('Retry-After'))
    if (retryAfter > 0) {
      return { ...result, error: { ...result.error, retryAfter } }
    }
  }
  return result
}
