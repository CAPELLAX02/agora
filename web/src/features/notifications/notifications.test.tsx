import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Notification, NotificationPreference } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const note = (over: Partial<Notification> = {}): Notification => ({
  id: 1,
  created_at: '2026-10-18T09:00:00Z',
  type: 'registration.decision',
  category: 'REGISTRATION',
  title_tr: 'Ders kaydınız onaylandı',
  title_en: 'Your course registration was approved',
  body_tr: '2026-FALL dönemi ders kaydınız onaylandı: 5 ders, 30 AKTS.',
  body_en: 'Your 2026-FALL course registration was approved.',
  link: '/ders-secimi',
  read_at: null,
  archived_at: null,
  ...over,
})

const security = note({
  id: 2,
  type: 'security.password_changed',
  category: 'SECURITY',
  title_tr: 'Parolanız değiştirildi',
  title_en: 'Your password was changed',
  body_tr: 'Hesabınızın parolası değiştirildi.',
  link: '/guvenlik',
  read_at: '2026-10-17T10:00:00Z',
})

type Call = { method: string; path: string; search: string; body?: unknown }

function session() {
  const calls: Call[] = []
  const record = async (request: Request) => {
    const text = await request.text()
    const url = new URL(request.url)
    calls.push({
      method: request.method,
      path: url.pathname,
      search: url.search,
      body: text ? JSON.parse(text) : undefined,
    })
  }
  server.use(
    http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
    http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('registration:manage_own'))),
  )
  return { calls, record }
}

describe('bildirimler', () => {
  it('zil okunmamış sayısını ve son bildirimleri gösterir; bildirim okundu sayılıp sayfasına gider', async () => {
    const { calls, record } = session()
    server.use(
      http.get('/api/v1/me/notifications/unread-count', () => HttpResponse.json({ count: 1 })),
      http.get('/api/v1/me/notifications', () => HttpResponse.json({ items: [note(), security] })),
      http.post('/api/v1/me/notifications/:id/read', async ({ request }) => {
        await record(request)
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { user, router } = renderApp('/guvenlik')
    const bell = await screen.findByRole('button', { name: 'Bildirimler, 1 okunmamış' })
    await user.click(bell)
    const menu = await screen.findByRole('menu')
    const item = await within(menu).findByRole('menuitem', { name: /Ders kaydınız onaylandı/ })
    expect(within(item).getByLabelText('Okunmamış')).toBeInTheDocument()
    expect(within(menu).getByRole('menuitem', { name: /Parolanız değiştirildi/ })).toBeInTheDocument()
    await user.click(item)
    await waitFor(() => expect(router.state.location.pathname).toBe('/ders-secimi'))
    expect(calls).toEqual([
      { method: 'POST', path: '/api/v1/me/notifications/1/read', search: '', body: undefined },
    ])
  })

  it('canlı akıştan gelen bildirim uyarı olarak görünür ve sayıyı tazeler', async () => {
    session()
    let unread = 0
    server.use(
      http.get('/api/v1/me/notifications/unread-count', () => HttpResponse.json({ count: unread })),
      http.get('/api/v1/me/notifications/stream', () => {
        unread = 1
        const body = new ReadableStream({
          start(controller) {
            controller.enqueue(
              new TextEncoder().encode(
                `retry: 5000\n: bağlandı\n\nid: 1\nevent: notification\ndata: ${JSON.stringify(note())}\n\n`,
              ),
            )
          },
        })
        return new HttpResponse(body, { headers: { 'Content-Type': 'text/event-stream' } })
      }),
    )
    renderApp('/guvenlik')
    expect(await screen.findByText('Ders kaydınız onaylandı')).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: 'Bildirimler, 1 okunmamış' })).toBeInTheDocument()
  })

  it('bildirim sayfası süzülür, sayfa sayfa yüklenir; bildirim arşive kaldırılır', async () => {
    const { calls, record } = session()
    server.use(
      http.get('/api/v1/me/notifications', async ({ request }) => {
        await record(request)
        const q = new URL(request.url).searchParams
        if (q.get('archived') === 'true') {
          return HttpResponse.json({ items: [] })
        }
        if (q.get('cursor') === 'c2') {
          return HttpResponse.json({ items: [security] })
        }
        return HttpResponse.json({ items: [note()], next_cursor: 'c2' })
      }),
      http.post('/api/v1/me/notifications/:id/archive', async ({ request }) => {
        await record(request)
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { user } = renderApp('/bildirimler')
    expect(await screen.findByRole('heading', { name: 'Bildirimler', level: 1 })).toBeInTheDocument()
    expect(await screen.findByRole('link', { name: 'Ders kaydınız onaylandı' })).toHaveAttribute(
      'href',
      '/ders-secimi',
    )
    await user.click(screen.getByRole('button', { name: 'Daha fazla göster' }))
    expect(await screen.findByText('Parolanız değiştirildi')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Ders kaydınız onaylandı: arşive kaldır' }))
    await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/me/notifications/1/archive')).toBe(true))

    await user.click(screen.getByRole('tab', { name: 'Arşiv' }))
    expect(await screen.findByText('Arşiviniz boş.')).toBeInTheDocument()
    expect(
      calls.filter((c) => c.method === 'GET').map((c) => new URLSearchParams(c.search).get('archived')),
    ).toContain('true')
  })

  it('tercih matrisinde değişen kanallar kaydedilir; güvenlik bildirimleri kapatılamaz', async () => {
    const { calls, record } = session()
    const prefs: NotificationPreference[] = [
      {
        type: 'security.password_changed',
        category: 'SECURITY',
        name_tr: 'Parolanız değiştirildi',
        name_en: 'Your password was changed',
        mandatory: true,
        in_app: true,
        email: true,
      },
      {
        type: 'registration.decision',
        category: 'REGISTRATION',
        name_tr: 'Ders kaydı kararı',
        name_en: 'Course registration decision',
        mandatory: false,
        in_app: true,
        email: true,
      },
    ]
    server.use(
      http.get('/api/v1/me/notification-preferences', () => HttpResponse.json({ items: prefs })),
      http.put('/api/v1/me/notification-preferences', async ({ request }) => {
        await record(request)
        return HttpResponse.json({ items: prefs })
      }),
    )
    const { user } = renderApp('/bildirimler/tercihler')
    const mandatory = await screen.findByRole('checkbox', { name: 'Parolanız değiştirildi: E-posta' })
    expect(mandatory).toBeDisabled()
    expect(screen.getByText('Kapatılamaz')).toBeInTheDocument()
    const save = screen.getByRole('button', { name: 'Kaydet' })
    expect(save).toBeDisabled()

    await user.click(screen.getByRole('checkbox', { name: 'Ders kaydı kararı: E-posta' }))
    expect(screen.getByText('1 kaydedilmemiş değişiklik')).toBeInTheDocument()
    await user.click(save)
    await waitFor(() =>
      expect(calls).toEqual([
        {
          method: 'PUT',
          path: '/api/v1/me/notification-preferences',
          search: '',
          body: { items: [{ type: 'registration.decision', in_app: true, email: false }] },
        },
      ]),
    )
    expect(await screen.findByText('Bildirim tercihleriniz kaydedildi.')).toBeInTheDocument()
  })
})
