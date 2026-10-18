import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Announcement, AnnouncementTargets } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const ann = (over: Partial<Announcement> = {}): Announcement => ({
  id: 'a1',
  title: 'Bölüm toplantısı',
  summary: 'Cuma 14.00, A101.',
  body_html: '<p>Gündem ekte.</p>',
  type: 'GENERAL',
  publish_from: '2026-10-18T09:00:00Z',
  publish_until: null,
  is_pinned: false,
  status: 'PUBLISHED',
  author: { id: 'u9', name: 'Mehmet Yıldız' },
  audiences: [
    { scope_type: 'DEPARTMENT', scope_id: 'd1', scope_name: 'Bilgisayar Mühendisliği', people: 'ALL' },
  ],
  notified_at: '2026-10-18T09:00:00Z',
  created_at: '2026-10-18T08:00:00Z',
  updated_at: '2026-10-18T09:00:00Z',
  version: 2,
  read_at: null,
  can_manage: false,
  ...over,
})

type Call = { method: string; path: string; body?: unknown; ifMatch: string | null }

function session(perms: string[]) {
  const calls: Call[] = []
  const record = async (request: Request) => {
    const text = await request.text()
    calls.push({
      method: request.method,
      path: new URL(request.url).pathname,
      body: text ? JSON.parse(text) : undefined,
      ifMatch: request.headers.get('If-Match'),
    })
  }
  server.use(
    http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
    http.get('/api/v1/me/permissions', () =>
      HttpResponse.json({
        items: [
          ...grants('announcement:read').items,
          ...perms.map((p) => ({
            permission: p,
            scope_type: 'DEPARTMENT',
            scope_id: 'd1',
            requires_mfa: false,
          })),
        ],
      }),
    ),
  )
  return { calls, record }
}

describe('duyurular', () => {
  it('akışta sabitlenen ve okunmamış duyurular işaretlenir; duyuru açılır', async () => {
    session([])
    server.use(
      http.get('/api/v1/me/announcements', () =>
        HttpResponse.json({
          items: [
            ann({ id: 'a2', title: 'Kampüs kapalı', type: 'URGENT', is_pinned: true }),
            ann({ read_at: '2026-10-18T10:00:00Z' }),
          ],
        }),
      ),
      http.get('/api/v1/announcements/:id', () =>
        HttpResponse.json(ann({ read_at: '2026-10-18T10:00:00Z' })),
      ),
    )
    const { user } = renderApp('/duyurular')
    const list = await screen.findByRole('list', { name: 'Duyurular' })
    const urgent = within(list).getByRole('link', { name: /Kampüs kapalı/ })
    expect(within(urgent).getByText('Acil')).toBeInTheDocument()
    expect(within(urgent).getByLabelText('Sabitlendi')).toBeInTheDocument()
    expect(within(urgent).getByText('Yeni')).toBeInTheDocument()
    const read = within(list).getByRole('link', { name: /Bölüm toplantısı/ })
    expect(within(read).queryByText('Yeni')).not.toBeInTheDocument()

    await user.click(read)
    expect(await screen.findByRole('heading', { name: 'Bölüm toplantısı', level: 1 })).toBeInTheDocument()
    expect(screen.getByText('Gündem ekte.')).toBeInTheDocument()
    expect(screen.getByText('Bilgisayar Mühendisliği · Herkes')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Yayımla' })).not.toBeInTheDocument()
  })

  it('yöneten taslağı istatistikleriyle görür ve yayımlar', async () => {
    const { calls, record } = session(['announcement:publish'])
    let current = ann({
      status: 'DRAFT',
      notified_at: null,
      can_manage: true,
      version: 1,
      stats: { audience: 40, reads: 0 },
    })
    server.use(
      http.get('/api/v1/announcements/:id', () => HttpResponse.json(current)),
      http.post('/api/v1/announcements/:id/publish', async ({ request }) => {
        await record(request)
        current = { ...current, status: 'PUBLISHED', version: 2, notified_at: '2026-10-18T09:00:00Z' }
        return HttpResponse.json(current)
      }),
    )
    const { user } = renderApp('/duyurular/a1')
    expect(await screen.findByText('Taslak')).toBeInTheDocument()
    expect(screen.getByText('Hedef kitle: 40 kişi')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Yayımla' }))
    await waitFor(() =>
      expect(calls).toEqual([
        { method: 'POST', path: '/api/v1/announcements/a1/publish', body: undefined, ifMatch: '"1"' },
      ]),
    )
    expect(await screen.findByText('Duyuru yayımlandı.')).toBeInTheDocument()
  })

  it('düzenleyicide hedef kitle seçilip duyuru yayımlanır', async () => {
    const { calls, record } = session(['announcement:publish'])
    const targets: AnnouncementTargets = {
      university: false,
      faculties: [],
      departments: [{ id: 'd1', name: 'Bilgisayar Mühendisliği', parent_id: 'f1' }],
      programs: [{ id: 'p1', name: 'Bilgisayar Mühendisliği (BIL-L)', parent_id: 'd1' }],
    }
    server.use(
      http.get('/api/v1/announcement-targets', () => HttpResponse.json(targets)),
      http.post('/api/v1/announcements', async ({ request }) => {
        await record(request)
        return HttpResponse.json(ann({ id: 'a9', can_manage: true }), { status: 201 })
      }),
      http.get('/api/v1/announcements/:id', () => HttpResponse.json(ann({ id: 'a9', can_manage: true }))),
    )
    const { user, router } = renderApp('/duyuru-yonetimi/yeni')
    await user.click(await screen.findByRole('button', { name: 'Yayımla' }))
    expect(await screen.findByText('Başlık yazın.')).toBeInTheDocument()

    await user.type(screen.getByLabelText('Başlık'), 'Staj başvuruları')
    await user.click(screen.getByRole('combobox', { name: 'Tür' }))
    await user.click(await screen.findByRole('option', { name: 'Akademik' }))
    await user.click(screen.getByRole('button', { name: 'Yayımla' }))
    expect(await screen.findByText('Her hedef için kapsam ve birim seçin.')).toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: '1. hedef: birim' }))
    await user.click(await screen.findByRole('option', { name: 'Bilgisayar Mühendisliği' }))
    await user.click(screen.getByRole('button', { name: 'Hedef ekle' }))
    await user.click(screen.getByRole('combobox', { name: '2. hedef: kapsam' }))
    await user.click(await screen.findByRole('option', { name: 'Program' }))
    await user.click(screen.getByRole('combobox', { name: '2. hedef: birim' }))
    await user.click(await screen.findByRole('option', { name: 'Bilgisayar Mühendisliği (BIL-L)' }))
    expect(screen.getByRole('combobox', { name: '2. hedef: kişiler' })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Yayımla' }))
    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toMatchObject({
      method: 'POST',
      path: '/api/v1/announcements',
      body: {
        title: 'Staj başvuruları',
        type: 'ACADEMIC',
        publish: true,
        publish_from: null,
        audiences: [
          { scope_type: 'DEPARTMENT', scope_id: 'd1', people: 'ALL' },
          { scope_type: 'PROGRAM', scope_id: 'p1', people: 'STUDENTS' },
        ],
      },
    })
    await waitFor(() => expect(router.state.location.pathname).toBe('/duyurular/a9'))
  })
})
