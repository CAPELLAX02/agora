import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { setupStore } from '@/app/store'
import { tokens } from '@/test/fixtures'
import { server } from '@/test/server'

import { refreshSession } from './baseQuery'
import { agoraApi } from './generated'

describe('refreshSession', () => {
  it('eşzamanlı çağrılar tek bir yenileme isteğini paylaşır', async () => {
    let calls = 0
    server.use(
      http.post('/api/v1/auth/refresh', async () => {
        calls++
        await new Promise((r) => setTimeout(r, 20))
        return HttpResponse.json(tokens())
      }),
    )
    const actions: unknown[] = []
    const dispatch = (a: { type: string }) => actions.push(a)
    const results = await Promise.all([
      refreshSession(dispatch, { retry: false }),
      refreshSession(dispatch, { retry: false }),
      refreshSession(dispatch, { retry: false }),
    ])
    expect(results).toEqual(['ok', 'ok', 'ok'])
    expect(calls).toBe(1)
    expect(actions).toHaveLength(1)
  })

  it('başka sekme aynı anda yenilediyse bir kez daha dener', async () => {
    let calls = 0
    server.use(
      http.post('/api/v1/auth/refresh', () => {
        calls++
        return calls === 1
          ? HttpResponse.json({ status: 401, code: 'INVALID_REFRESH_TOKEN' }, { status: 401 })
          : HttpResponse.json(tokens())
      }),
    )
    expect(await refreshSession(() => undefined, { retry: true })).toBe('ok')
    expect(calls).toBe(2)
  })

  it('sunucuya ulaşılamazsa oturumu düşürmez', async () => {
    server.use(http.post('/api/v1/auth/refresh', () => HttpResponse.error()))
    expect(await refreshSession(() => undefined, { retry: true })).toBe('error')
  })

  it('reddedilen yenilemede açılışta tekrar denemez', async () => {
    let calls = 0
    server.use(
      http.post('/api/v1/auth/refresh', () => {
        calls++
        return HttpResponse.json({ status: 401 }, { status: 401 })
      }),
    )
    expect(await refreshSession(() => undefined, { retry: false })).toBe('invalid')
    expect(calls).toBe(1)
  })
})

describe('CSV uçları', () => {
  it('metin gövde text/csv olarak gider, CSV yanıtı metin olarak okunur', async () => {
    let sent: { type: string | null; body: string; match: string | null } | null = null
    server.use(
      http.post('/api/v1/assessment-components/:id/scores/import', async ({ request }) => {
        sent = {
          type: request.headers.get('Content-Type'),
          body: await request.text(),
          match: request.headers.get('If-Match'),
        }
        return HttpResponse.json({ ok: true })
      }),
      http.get('/api/v1/sections/:id/gradebook.csv', () =>
        HttpResponse.text('Öğrenci No;Ara sınav\r\n22290102;75,5\r\n', {
          headers: { 'Content-Type': 'text/csv; charset=utf-8' },
        }),
      ),
    )
    const store = setupStore()
    await store
      .dispatch(
        agoraApi.endpoints.importScores.initiate({ id: 'c1', 'If-Match': '"3"', body: '22290102;70' }),
      )
      .unwrap()
    expect(sent).toEqual({ type: 'text/csv', body: '22290102;70', match: '"3"' })

    const csv = await store
      .dispatch(agoraApi.endpoints.exportGradebook.initiate({ id: 's1' }, { subscribe: false }))
      .unwrap()
    expect(csv).toBe('Öğrenci No;Ara sınav\r\n22290102;75,5\r\n')
  })
})
