import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'

import { grants, me } from './fixtures'

/**
 * Varsayılan uçlar: oturum yok (refresh 401), oturum açılınca standart bir kullanıcı.
 * Testler server.use ile kendi senaryolarını ekler.
 */
export const handlers = [
  http.post('/api/v1/auth/refresh', () =>
    HttpResponse.json(
      { status: 401, code: 'INVALID_REFRESH_TOKEN', title: 'Unauthorized', type: 'about:blank' },
      { status: 401 },
    ),
  ),
  http.post('/api/v1/auth/logout', () => new HttpResponse(null, { status: 204 })),
  http.get('/api/v1/me', () => HttpResponse.json(me())),
  http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('user:read'))),
  http.get('/api/v1/roles', () => HttpResponse.json({ items: [] })),
  // Üst çubuktaki zil: okunmamış yok; canlı akış testlerde kapalı (yeniden deneme seyrek).
  http.get('/api/v1/me/notifications/unread-count', () => HttpResponse.json({ count: 0 })),
  http.get('/api/v1/me/announcements', () => HttpResponse.json({ items: [] })),
  http.get('/api/v1/me/conversations/unread-count', () => HttpResponse.json({ count: 0 })),
  http.get('/api/v1/me/notifications/stream', () =>
    HttpResponse.json(
      { status: 503, code: 'STREAM_UNAVAILABLE', title: 'Service Unavailable', type: 'about:blank' },
      { status: 503 },
    ),
  ),
]

export const server = setupServer(...handlers)
