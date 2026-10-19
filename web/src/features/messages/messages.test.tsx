import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Conversation, ConversationMessage, MessagingPerson } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const ayse: MessagingPerson = { id: 'u9', name: 'Ayşe Demir', number: 'P10001', is_staff: true }
const me: MessagingPerson = { id: 'u1', name: 'Deniz Aksoy', number: '22290001', is_staff: false }

const msg = (over: Partial<ConversationMessage>): ConversationMessage => ({
  id: 'm1',
  sender: ayse,
  body: 'Merhaba',
  reply_to_id: null,
  created_at: '2026-10-18T09:00:00Z',
  edited_at: null,
  deleted: false,
  mine: false,
  ...over,
})

const direct = (over: Partial<Conversation> = {}): Conversation => ({
  id: 'c1',
  kind: 'DIRECT',
  title: '',
  section_id: null,
  peer: ayse,
  last_message: msg({ id: 'm2', body: 'Yarın görüşelim.' }),
  last_message_at: '2026-10-18T09:05:00Z',
  created_at: '2026-10-17T09:00:00Z',
  unread: 1,
  is_starred: false,
  muted_until: null,
  is_moderator: false,
  member_count: 2,
  members: [ayse, me],
  ...over,
})

const section: Conversation = {
  ...direct(),
  id: 'c2',
  kind: 'SECTION',
  title: 'COM1007-1 · Bilgisayar Programlama I',
  section_id: 's1',
  peer: null,
  last_message: null,
  last_message_at: null,
  unread: 0,
}

type Call = { method: string; path: string; body?: unknown }

function session(conversations: () => Conversation[]) {
  const calls: Call[] = []
  const record = async (request: Request) => {
    const text = await request.text()
    calls.push({
      method: request.method,
      path: new URL(request.url).pathname,
      body: text ? JSON.parse(text) : undefined,
    })
  }
  server.use(
    http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
    http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('registration:manage_own'))),
    http.get('/api/v1/me/conversations', () => HttpResponse.json({ items: conversations() })),
    http.get('/api/v1/me/conversations/unread-count', () =>
      HttpResponse.json({ count: conversations().reduce((n, c) => n + c.unread, 0) }),
    ),
  )
  return { calls, record }
}

describe('mesajlar', () => {
  it('konuşmalar okunmamış sayılarıyla listelenir; açılan konuşma okundu sayılır ve Enter ile ileti gider', async () => {
    let current = direct()
    const { calls, record } = session(() => [current, section])
    server.use(
      http.get('/api/v1/conversations/:id', () => HttpResponse.json(current)),
      http.get('/api/v1/conversations/:id/messages', () =>
        HttpResponse.json({
          items: [
            msg({ id: 'm2', body: 'Yarın görüşelim.' }),
            msg({ id: 'm1', body: 'Merhaba', sender: me, mine: true }),
          ],
          has_more: false,
        }),
      ),
      http.post('/api/v1/conversations/:id/read', async ({ request }) => {
        await record(request)
        current = { ...current, unread: 0 }
        return new HttpResponse(null, { status: 204 })
      }),
      http.post('/api/v1/conversations/:id/messages', async ({ request }) => {
        await record(request)
        return HttpResponse.json(msg({ id: 'm3', body: 'Tamam hocam', sender: me, mine: true }), {
          status: 201,
        })
      }),
    )
    const { user } = renderApp('/mesajlar')
    const nav = await screen.findByRole('navigation', { name: 'Ana menü' })
    expect(await within(nav).findByLabelText('1 okunmamış ileti')).toBeInTheDocument()
    const list = await screen.findByRole('region', { name: 'Konuşmalar' })
    const ayseLink = within(list).getByRole('link', { name: /Ayşe Demir/ })
    expect(within(ayseLink).getByLabelText('1 okunmamış ileti')).toBeInTheDocument()
    expect(within(list).getByRole('link', { name: /COM1007-1/ })).toBeInTheDocument()

    await user.click(ayseLink)
    const history = await screen.findByRole('list', { name: 'İletiler' })
    const bubbles = within(history).getAllByRole('listitem')
    expect(bubbles[0]).toHaveTextContent('Merhaba') // eski iletiler üstte
    expect(bubbles[1]).toHaveTextContent('Yarın görüşelim.')
    await waitFor(() =>
      expect(calls).toContainEqual({
        method: 'POST',
        path: '/api/v1/conversations/c1/read',
        body: { message_id: 'm2' },
      }),
    )

    const box = screen.getByRole('textbox', { name: 'İleti' })
    await user.type(box, 'Tamam{Shift>}{Enter}{/Shift}hocam')
    expect(box).toHaveValue('Tamam\nhocam')
    await user.clear(box)
    await user.type(box, 'Tamam hocam{Enter}')
    await waitFor(() =>
      expect(calls).toContainEqual({
        method: 'POST',
        path: '/api/v1/conversations/c1/messages',
        body: { body: 'Tamam hocam' },
      }),
    )
    await waitFor(() => expect(box).toHaveValue(''))
  })

  it('öğrenci yeni konuşmada kişilerinden birini seçer ve konuşma açılır', async () => {
    const { calls, record } = session(() => [])
    server.use(
      http.get('/api/v1/messaging/contacts', () => HttpResponse.json({ items: [ayse] })),
      http.post('/api/v1/conversations', async ({ request }) => {
        await record(request)
        return HttpResponse.json(direct({ unread: 0, last_message: null }), { status: 201 })
      }),
      http.get('/api/v1/conversations/:id', () =>
        HttpResponse.json(direct({ unread: 0, last_message: null })),
      ),
      http.get('/api/v1/conversations/:id/messages', () => HttpResponse.json({ items: [], has_more: false })),
    )
    const { user, router } = renderApp('/mesajlar')
    expect(await screen.findByText('Henüz konuşmanız yok.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Yeni konuşma' }))
    const dialog = await screen.findByRole('dialog', { name: 'Yeni konuşma' })
    expect(within(dialog).queryByRole('tab', { name: 'Grup' })).not.toBeInTheDocument()
    await user.click(await within(dialog).findByRole('button', { name: /Ayşe Demir/ }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/mesajlar/c1'))
    expect(calls).toEqual([
      { method: 'POST', path: '/api/v1/conversations', body: { kind: 'DIRECT', user_id: 'u9' } },
    ])
    expect(await screen.findByText('İlk iletiyi siz yazın.')).toBeInTheDocument()
  })

  it('canlı akıştan gelen ileti olayı konuşmaları tazeler', async () => {
    let unread = 0
    session(() => [direct({ unread })])
    server.use(
      http.get('/api/v1/me/notifications/stream', () => {
        unread = 2
        const body = new ReadableStream({
          start(controller) {
            controller.enqueue(
              new TextEncoder().encode('retry: 5000\n\nevent: message\ndata: {"conversation_id":"c1"}\n\n'),
            )
          },
        })
        return new HttpResponse(body, { headers: { 'Content-Type': 'text/event-stream' } })
      }),
    )
    renderApp('/mesajlar')
    const list = await screen.findByRole('region', { name: 'Konuşmalar' })
    expect(await within(list).findByLabelText('2 okunmamış ileti')).toBeInTheDocument()
  })
})
