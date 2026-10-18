import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'

import type { Activity, ForumThread, ForumThreadList, Space } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

// jsdom'da ProseMirror'a klavyeyle yazılamıyor: editör (kendi testinde sınanır) burada düz bir
// metin alanıyla taklit edilir.
vi.mock('@/shared/ui/rich-text-editor', () => ({
  RichTextEditor: ({
    value,
    onChange,
    label,
  }: {
    value: string
    onChange: (v: string) => void
    label: string
  }) => <textarea aria-label={label} value={value} onChange={(e) => onChange(e.target.value)} />,
}))

const forum: Activity = {
  id: 'f1',
  topic_id: 'tp1',
  type: 'FORUM',
  title: 'Soru-cevap',
  description_html: '',
  is_visible: true,
  availability: 'OPEN',
  available_from: null,
  available_until: null,
  version: 1,
  updated_at: '2026-10-10T09:00:00Z',
  files: [],
  url: null,
  open_in_new_tab: null,
  body_html: null,
  assignment: null,
  forum: {
    kind: 'GENERAL',
    students_can_start_threads: true,
    threads: 2,
    last_post_at: '2026-10-17T09:00:00Z',
  },
  completed: false,
}

const space = (role: Space['role']): Space => ({
  section_id: 's1',
  section_code: '1',
  course: {
    id: 'c1',
    code: 'COM1007',
    name_tr: 'Bilgisayar Programlama I',
    name_en: 'Computer Programming I',
  },
  term: { id: 't', code: '2026-FALL' },
  role,
  can_edit: role !== 'STUDENT',
  summary_html: '',
  instructors: [],
  progress: null,
  topics: [
    {
      id: 'tp1',
      title: 'Genel',
      summary_html: '',
      is_visible: true,
      availability: 'OPEN',
      available_from: null,
      available_until: null,
      version: 1,
      activities: [forum],
    },
  ],
})

const ada = { id: 'u1', name: 'Ada Aydemir', is_instructor: false }
const ayse = { id: 'u9', name: 'Ayşe Demir', is_instructor: true }

const list: ForumThreadList = {
  forum: forum.forum!,
  can_start: true,
  items: [
    {
      id: 'th1',
      title: 'Döngü sorusu',
      author: ada,
      is_pinned: true,
      is_locked: false,
      post_count: 2,
      last_post_at: '2026-10-17T09:00:00Z',
      created_at: '2026-10-16T09:00:00Z',
    },
    {
      id: 'th2',
      title: 'Kaynak önerisi',
      author: ada,
      is_pinned: false,
      is_locked: false,
      post_count: 1,
      last_post_at: '2026-10-15T09:00:00Z',
      created_at: '2026-10-15T09:00:00Z',
    },
  ],
}

const thread = (over: Partial<ForumThread> = {}): ForumThread => ({
  ...list.items[0]!,
  activity_id: 'f1',
  forum: forum.forum!,
  can_reply: true,
  can_moderate: false,
  posts: [
    {
      id: 'p1',
      parent_post_id: null,
      author: ada,
      body_html: '<p>for ile while farkı?</p>',
      created_at: '2026-10-16T09:00:00Z',
      edited_at: null,
      deleted: false,
      can_edit: true,
    },
    {
      id: 'p2',
      parent_post_id: 'p1',
      author: ayse,
      body_html: '<p>while koşula bakar.</p>',
      created_at: '2026-10-17T09:00:00Z',
      edited_at: null,
      deleted: false,
      can_edit: false,
    },
  ],
  ...over,
})

type Call = { method: string; path: string; body?: unknown }

function session(role: Space['role'], perms: string[], data: ForumThread = thread()) {
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
    http.get('/api/v1/me/permissions', () => HttpResponse.json(grants(...perms))),
    http.get('/api/v1/spaces/:id', () => HttpResponse.json(space(role))),
    http.get('/api/v1/activities/:id', () => HttpResponse.json(forum)),
    http.get('/api/v1/activities/:id/threads', () => HttpResponse.json(list)),
    http.post('/api/v1/activities/:id/threads', async ({ request }) => {
      await record(request)
      return HttpResponse.json({ ...data, id: 'th3', title: 'Yeni soru' }, { status: 201 })
    }),
    http.get('/api/v1/threads/:id', () => HttpResponse.json(data)),
    http.post('/api/v1/threads/:id/posts', async ({ request }) => {
      await record(request)
      return HttpResponse.json(data, { status: 201 })
    }),
    http.put('/api/v1/threads/:id', async ({ request }) => {
      await record(request)
      return HttpResponse.json(data)
    }),
  )
  return { calls }
}

describe('forum', () => {
  it('konular sabitlenen önce listelenir; öğrenci konu açar ve konuya geçer', async () => {
    const { calls } = session('STUDENT', ['registration:manage_own'])
    const { user, router } = renderApp('/derslerim/s1/etkinlik/f1')
    const links = await screen.findAllByRole('link', { name: /Döngü sorusu|Kaynak önerisi/ })
    expect(links[0]).toHaveTextContent('Döngü sorusu')
    expect(within(links[0]!).getByLabelText('Sabitlendi')).toBeInTheDocument()
    expect(within(links[0]!).getByText('2 gönderi')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Yeni konu' }))
    const dialog = await screen.findByRole('dialog', { name: 'Yeni konu' })
    await user.click(within(dialog).getByRole('button', { name: 'Yayımla' }))
    expect(within(dialog).getByText('Başlık ve ileti yazın.')).toBeInTheDocument()
    await user.type(within(dialog).getByLabelText('Başlık'), 'Yeni soru')
    await user.type(await within(dialog).findByRole('textbox', { name: 'İleti' }), 'Diziler nasıl sıralanır?')
    await user.click(within(dialog).getByRole('button', { name: 'Yayımla' }))
    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toMatchObject({
      method: 'POST',
      path: '/api/v1/activities/f1/threads',
      body: { title: 'Yeni soru' },
    })
    expect((calls[0]!.body as { body_html: string }).body_html).toContain('Diziler nasıl sıralanır?')
    await waitFor(() => expect(router.state.location.pathname).toBe('/derslerim/s1/etkinlik/f1/konu/th3'))
  })

  it('konu sayfası: öğretim elemanı işareti, yanıtlanan gönderi, belirli gönderiye yanıt', async () => {
    const { calls } = session('STUDENT', ['registration:manage_own'])
    const { user } = renderApp('/derslerim/s1/etkinlik/f1/konu/th1')
    expect(await screen.findByRole('heading', { name: 'Döngü sorusu', level: 1 })).toBeInTheDocument()
    const posts = screen.getAllByRole('listitem').filter((li) => li.id.startsWith('post-'))
    expect(within(posts[1]!).getByText('Öğretim elemanı')).toBeInTheDocument()
    expect(within(posts[1]!).getByText('Ada Aydemir adlı kişiye yanıt')).toBeInTheDocument()
    // Kendi gönderisini düzenleyebilir, başkasınınkini düzenleyemez.
    expect(within(posts[0]!).getByRole('button', { name: 'Düzenle' })).toBeInTheDocument()
    expect(within(posts[1]!).queryByRole('button', { name: 'Düzenle' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Kilitle' })).not.toBeInTheDocument()

    await user.click(within(posts[1]!).getByRole('button', { name: 'Yanıtla' }))
    const box = screen.getByRole('region', { name: 'Yanıt yaz' })
    expect(within(box).getByText('Ayşe Demir adlı kişiye yanıt')).toBeInTheDocument()
    await user.type(await within(box).findByRole('textbox', { name: 'Yanıtınız' }), 'Teşekkürler!')
    await user.click(within(box).getByRole('button', { name: 'Gönder' }))
    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toMatchObject({
      method: 'POST',
      path: '/api/v1/threads/th1/posts',
      body: { parent_post_id: 'p2' },
    })
    expect(await screen.findByText('Yanıt gönderildi.')).toBeInTheDocument()
  })

  it('öğretim elemanı konuyu kilitler; kilitli konuda öğrenci yanıt yazamaz', async () => {
    const { calls } = session('PRIMARY', ['assessment_plan:manage'], thread({ can_moderate: true }))
    const { user } = renderApp('/derslerim/s1/etkinlik/f1/konu/th1')
    await user.click(await screen.findByRole('button', { name: 'Kilitle' }))
    await waitFor(() =>
      expect(calls).toEqual([{ method: 'PUT', path: '/api/v1/threads/th1', body: { is_locked: true } }]),
    )
  })

  it('kilitli konu', async () => {
    session('STUDENT', ['registration:manage_own'], thread({ is_locked: true, can_reply: false }))
    renderApp('/derslerim/s1/etkinlik/f1/konu/th1')
    expect(await screen.findByText('Konu kilitli: yeni yanıt yazılamaz.')).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Yanıt yaz' })).not.toBeInTheDocument()
  })
})
