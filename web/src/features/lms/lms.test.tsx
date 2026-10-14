import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Activity, Space, SpaceSummary, Term, Topic } from '@/shared/api/generated'
import { grants, problem, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const term: Term = {
  id: 't-fall',
  code: '2026-FALL',
  academic_year: '2026-2027',
  start_year: 2026,
  term_type: 'FALL',
  starts_on: '2026-09-14',
  ends_on: '2027-02-05',
  status: 'ACTIVE',
  is_current: true,
  version: 1,
}

const course = {
  id: 'c1',
  code: 'COM1007',
  name_tr: 'Bilgisayar Programlama I',
  name_en: 'Computer Programming I',
}
const instructor = {
  user_id: 'u1',
  title: 'Dr. Öğr. Üyesi',
  first_name: 'Ayşe',
  last_name: 'Demir',
  email: 'ayse.demir@agora.test',
  role: 'PRIMARY' as const,
}

const activity = (id: string, over: Partial<Activity> = {}): Activity => ({
  id,
  topic_id: 'tp1',
  type: 'RESOURCE',
  title: `Etkinlik ${id}`,
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
  ...over,
})

const topic = (id: string, over: Partial<Topic> = {}): Topic => ({
  id,
  title: `Bölüm ${id}`,
  summary_html: '',
  is_visible: true,
  availability: 'OPEN',
  available_from: null,
  available_until: null,
  version: 1,
  activities: [],
  ...over,
})

function space(over: Partial<Space> = {}): Space {
  return {
    section_id: 's1',
    section_code: '1',
    course,
    term: { id: term.id, code: term.code },
    role: 'STUDENT',
    can_edit: false,
    summary_html: '<p>Haftalık iki saat laboratuvar.</p>',
    instructors: [instructor],
    topics: [
      topic('tp1', {
        title: 'Genel',
        activities: [
          activity('a1', {
            title: 'Ders izlencesi',
            files: [{ id: 'f1', name: 'izlence.pdf', mime_type: 'application/pdf', size: 245760 }],
          }),
          activity('a2', {
            type: 'LABEL',
            title: 'Duyuru',
            description_html: '<p>Ara sınav 12 Kasım’da.</p>',
          }),
          activity('a3', {
            type: 'URL',
            title: 'Go turu',
            url: 'https://go.dev/tour',
            open_in_new_tab: true,
          }),
        ],
      }),
      topic('tp2', {
        title: '2. Hafta',
        activities: [activity('a4', { topic_id: 'tp2', type: 'PAGE', title: 'Döngüler' })],
      }),
      topic('tp3', {
        title: '3. Hafta',
        availability: 'NOT_YET',
        available_from: '2026-10-28T06:00:00Z',
      }),
    ],
    ...over,
  }
}

type Call = { method: string; path: string; body?: unknown; ifMatch?: string | null }

function setup(path: string, data: Space, perms: string[] = ['registration:manage_own']) {
  const calls: Call[] = []
  const record = async (request: Request) => {
    const url = new URL(request.url)
    calls.push({
      method: request.method,
      path: url.pathname,
      body: request.method === 'GET' || request.method === 'DELETE' ? undefined : await request.json(),
      ifMatch: request.headers.get('If-Match'),
    })
  }
  const summary: SpaceSummary = {
    section_id: data.section_id,
    section_code: data.section_code,
    course: data.course,
    term: data.term,
    role: data.role,
    instructors: data.instructors,
    activities: 4,
  }
  server.use(
    http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
    http.get('/api/v1/me/permissions', () => HttpResponse.json(grants(...perms))),
    http.get('/api/v1/terms', () => HttpResponse.json({ items: [term] })),
    http.get('/api/v1/terms/current', () => HttpResponse.json(term)),
    http.get('/api/v1/me/spaces', () => HttpResponse.json({ items: [summary] })),
    http.get('/api/v1/spaces/:id', () => HttpResponse.json(data)),
    http.get('/api/v1/activities/:id', ({ params }) => {
      const a = data.topics.flatMap((tp) => tp.activities).find((x) => x.id === params.id)
      return a
        ? HttpResponse.json({
            ...a,
            body_html: a.type === 'PAGE' ? '<h2>For döngüsü</h2><p>Sayaçla tekrar.</p>' : null,
          })
        : HttpResponse.json(problem(404, 'NOT_FOUND'), { status: 404 })
    }),
    http.get('/api/v1/activities/:id/files/:fileId/download-url', ({ params }) =>
      HttpResponse.json({
        url: `https://s3.agora.test/agora/${String(params.fileId)}?sig=1`,
        expires_at: '2026-10-14T10:05:00Z',
      }),
    ),
    http.put('/api/v1/spaces/:id/layout', async ({ request }) => {
      await record(request)
      return HttpResponse.json(data)
    }),
    http.post('/api/v1/spaces/:id/topics', async ({ request }) => {
      await record(request)
      return HttpResponse.json(data, { status: 201 })
    }),
    http.post('/api/v1/topics/:id/activities', async ({ request }) => {
      await record(request)
      return HttpResponse.json(data, { status: 201 })
    }),
    http.put('/api/v1/activities/:id', async ({ request }) => {
      await record(request)
      return HttpResponse.json(activity('a1'))
    }),
    http.delete('/api/v1/activities/:id', async ({ request }) => {
      await record(request)
      return new HttpResponse(null, { status: 204 })
    }),
  )
  return { calls, ...renderApp(path) }
}

// İndirme bağlantısı tıklanınca jsdom sayfadan çıkmaya çalışır: tıklanan adres kaydedilir.
const clicked: string[] = []
beforeEach(() => {
  clicked.length = 0
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
    clicked.push(this.href)
  })
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('derslerim', () => {
  it('dönemdeki ders alanları listelenir ve menüde görünür', async () => {
    setup('/derslerim', space())
    expect(await screen.findByRole('heading', { name: 'Derslerim', level: 1 })).toBeInTheDocument()
    const card = await screen.findByRole('link', { name: /Bilgisayar Programlama I/ })
    expect(card).toHaveAttribute('href', '/derslerim/s1')
    expect(within(card).getByText('COM1007-1')).toBeInTheDocument()
    expect(within(card).getByText('Dr. Öğr. Üyesi Ayşe Demir')).toBeInTheDocument()
    expect(within(card).getByText('4 etkinlik')).toBeInTheDocument()
    expect(screen.getByRole('navigation').querySelector('a[href="/derslerim"]')).not.toBeNull()
  })
})

describe('ders alanı (öğrenci)', () => {
  it('bölümler, etkinlikler, ders bilgisi ve öğretim elemanı görünür; düzenleme yoktur', async () => {
    setup('/derslerim/s1', space())
    expect(await screen.findByRole('heading', { name: 'Bilgisayar Programlama I' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Genel' })).toBeInTheDocument()
    expect(screen.getByText('Ara sınav 12 Kasım’da.')).toBeInTheDocument()
    expect(screen.getByText('izlence.pdf · 240 KB')).toBeInTheDocument()
    expect(screen.getByText('go.dev')).toBeInTheDocument()
    expect(screen.getByText('Haftalık iki saat laboratuvar.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'E-posta gönder: Ayşe Demir' })).toHaveAttribute(
      'href',
      'mailto:ayse.demir@agora.test',
    )
    // Açılmamış bölüm: içerik yerine açılış tarihi.
    const week3 = screen.getByRole('region', { name: '3. Hafta' })
    expect(within(week3).getByText('Bu bölüm henüz açılmadı.')).toBeInTheDocument()
    expect(within(week3).getByText(/Açılış/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Düzenlemeyi aç' })).not.toBeInTheDocument()
  })

  it('dosya imzalı adresle iner; sayfa etkinliği kendi sayfasında açılır', async () => {
    const { user, router } = setup('/derslerim/s1', space())
    await user.click(await screen.findByRole('button', { name: /Ders izlencesi/ }))
    await waitFor(() => expect(clicked).toEqual(['https://s3.agora.test/agora/f1?sig=1']))

    await user.click(screen.getByRole('button', { name: /Döngüler/ }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/derslerim/s1/etkinlik/a4'))
    expect(await screen.findByRole('heading', { name: 'For döngüsü' })).toBeInTheDocument()
    expect(screen.getByText('Sayaçla tekrar.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /COM1007-1/ })).toHaveAttribute('href', '/derslerim/s1')
  })
})

describe('ders alanı (öğretim elemanı)', () => {
  const teacher = () => space({ role: 'PRIMARY', can_edit: true })

  it('düzenleme modunda etkinlik taşınır, gizlenir ve silinir', async () => {
    const { user, calls } = setup('/derslerim/s1', teacher(), ['assessment_plan:manage'])
    await user.click(await screen.findByRole('button', { name: 'Düzenlemeyi aç' }))
    expect(screen.getByRole('button', { name: 'Düzenleme açık' })).toHaveAttribute('aria-pressed', 'true')

    // İlk bölümün son etkinliği aşağı: sonraki bölümün başına geçer.
    await user.click(screen.getByRole('button', { name: 'Go turu öğesini aşağı taşı' }))
    await waitFor(() =>
      expect(calls.at(-1)).toEqual({
        method: 'PUT',
        path: '/api/v1/spaces/s1/layout',
        body: {
          topics: [
            { id: 'tp1', activity_ids: ['a1', 'a2'] },
            { id: 'tp2', activity_ids: ['a3', 'a4'] },
            { id: 'tp3', activity_ids: [] },
          ],
        },
        ifMatch: null,
      }),
    )

    await user.click(screen.getByRole('button', { name: 'Ders izlencesi için işlemler' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Öğrencilerden gizle' }))
    await waitFor(() => expect(calls.at(-1)?.path).toBe('/api/v1/activities/a1'))
    expect(calls.at(-1)).toMatchObject({
      method: 'PUT',
      ifMatch: '"1"',
      body: { type: 'RESOURCE', title: 'Ders izlencesi', is_visible: false, file_ids: ['f1'] },
    })

    await user.click(screen.getByRole('button', { name: 'Duyuru için işlemler' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Sil' }))
    const confirm = await screen.findByRole('alertdialog')
    expect(within(confirm).getByText('“Duyuru” silinsin mi?')).toBeInTheDocument()
    await user.click(within(confirm).getByRole('button', { name: 'Sil' }))
    await waitFor(() =>
      expect(calls.at(-1)).toMatchObject({ method: 'DELETE', path: '/api/v1/activities/a2' }),
    )
  })

  it('bağlantı etkinliği eklenir; başlık zorunludur', async () => {
    const { user, calls } = setup('/derslerim/s1', teacher(), ['assessment_plan:manage'])
    await user.click(await screen.findByRole('button', { name: 'Düzenlemeyi aç' }))
    const general = screen.getByRole('region', { name: 'Genel' })
    await user.click(within(general).getByRole('button', { name: 'Etkinlik ya da kaynak ekle' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Bağlantı' }))

    const dialog = await screen.findByRole('dialog', { name: 'Yeni Bağlantı' })
    await user.click(within(dialog).getByRole('button', { name: 'Kaydet' }))
    expect(within(dialog).getByText('Başlık girin.')).toBeInTheDocument()

    await user.type(within(dialog).getByLabelText('Başlık'), 'Go belgeleri')
    const url = within(dialog).getByLabelText('Bağlantı adresi')
    await user.clear(url)
    await user.type(url, 'https://go.dev/doc')
    await user.click(within(dialog).getByRole('button', { name: 'Kaydet' }))
    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toMatchObject({
      method: 'POST',
      path: '/api/v1/topics/tp1/activities',
      body: {
        type: 'URL',
        title: 'Go belgeleri',
        url: 'https://go.dev/doc',
        open_in_new_tab: true,
        is_visible: true,
        available_from: null,
        available_until: null,
        file_ids: [],
      },
    })
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(await screen.findByText('Etkinlik eklendi.')).toBeInTheDocument()
  })

  it('dosya yüklenir ve dosya etkinliği eklenir', async () => {
    // jsdom 30'un FormData'sındaki dosyayı vitest'in fetch uyumluluk katmanı okuyamıyor (MSW
    // XHR'ı fetch isteğine çevirirken): yükleme ucu sahte bir XMLHttpRequest ile karşılanır.
    const uploads: { url: string; name: string; auth: string | null }[] = []
    class FakeXHR {
      upload: { onprogress: ((e: ProgressEvent) => void) | null } = { onprogress: null }
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      onabort: (() => void) | null = null
      status = 0
      responseText = ''
      private url = ''
      private headers: Record<string, string> = {}
      open(_method: string, url: string) {
        this.url = url
      }
      setRequestHeader(name: string, value: string) {
        this.headers[name] = value
      }
      abort() {
        this.onabort?.()
      }
      send(form: FormData) {
        const file = form.get('file') as File
        uploads.push({ url: this.url, name: file.name, auth: this.headers.Authorization ?? null })
        this.upload.onprogress?.({
          loaded: file.size,
          total: file.size,
          lengthComputable: true,
        } as ProgressEvent)
        this.status = 201
        this.responseText = JSON.stringify({
          id: 'f9',
          name: file.name,
          mime_type: 'application/pdf',
          size: 2048,
          uploaded_at: '2026-10-14T09:00:00Z',
        })
        setTimeout(() => this.onload?.(), 0)
      }
    }
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
    const { user, calls } = setup('/derslerim/s1', teacher(), ['assessment_plan:manage'])
    await user.click(await screen.findByRole('button', { name: 'Düzenlemeyi aç' }))
    const week2 = screen.getByRole('region', { name: '2. Hafta' })
    await user.click(within(week2).getByRole('button', { name: 'Etkinlik ya da kaynak ekle' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Dosya' }))
    const dialog = await screen.findByRole('dialog', { name: 'Yeni Dosya' })

    await user.type(within(dialog).getByLabelText('Başlık'), 'Hafta 2 slaytları')
    await user.click(within(dialog).getByRole('button', { name: 'Kaydet' }))
    expect(within(dialog).getByText('En az bir dosya yükleyin.')).toBeInTheDocument()

    await user.upload(
      within(dialog).getByLabelText('Dosya seç'),
      new File(['%PDF-1.7'], 'hafta2.pdf', { type: 'application/pdf' }),
    )
    expect(
      await within(dialog).findByRole('button', { name: 'hafta2.pdf dosyasını kaldır' }),
    ).toBeInTheDocument()
    expect(uploads).toEqual([{ url: '/api/v1/files', name: 'hafta2.pdf', auth: 'Bearer access-1' }])
    expect(within(dialog).getByText('2 KB')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Kaydet' }))
    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toMatchObject({
      path: '/api/v1/topics/tp2/activities',
      body: { type: 'RESOURCE', title: 'Hafta 2 slaytları', file_ids: ['f9'] },
    })
  })
})
