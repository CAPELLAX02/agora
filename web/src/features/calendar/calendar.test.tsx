import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { CalendarEvent, CalendarEventType, CalendarWindow, Term } from '@/shared/api/generated'
import { grants, problem, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const term: Term = {
  id: '01a12000-0000-7000-8000-000000000001',
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
const muh = '01a12000-0000-7000-8000-0000000000f1'

const registration: CalendarEventType = {
  code: 'COURSE_REGISTRATION',
  name_tr: 'Öğrenci ders seçme işlemleri',
  name_en: 'Course registration',
  category: 'REGISTRATION',
  is_action_window: true,
}
const addDrop: CalendarEventType = {
  ...registration,
  code: 'ADD_DROP_STUDENT',
  name_tr: 'Ekle-bırak (öğrenci)',
  name_en: 'Add-drop (student)',
}

const event = (over: Partial<CalendarEvent> = {}): CalendarEvent => ({
  id: '01a12000-0000-7000-8000-0000000000e1',
  term_id: term.id,
  type: registration,
  title_tr: null,
  title_en: null,
  starts_at: '2026-09-07T07:00:00Z',
  ends_at: '2026-09-11T21:00:00Z',
  scope_type: 'UNIVERSITY',
  scope: null,
  is_published: true,
  note: null,
  version: 1,
  ...over,
})

const extension = event({
  id: '01a12000-0000-7000-8000-0000000000e2',
  type: addDrop,
  title_tr: 'Ekle-bırak (Mühendislik, uzatma)',
  starts_at: '2026-10-05T07:00:00Z',
  ends_at: '2026-10-16T21:00:00Z',
  scope_type: 'FACULTY',
  scope: { id: muh, name: 'Mühendislik Fakültesi' },
})

const windows = (facultyId: string | null): CalendarWindow[] => [
  { type: registration, scope_type: 'UNIVERSITY', open: false, current: null, next: null, events: [event()] },
  facultyId
    ? {
        type: addDrop,
        scope_type: 'FACULTY',
        open: true,
        current: extension,
        next: null,
        events: [extension],
      }
    : { type: addDrop, scope_type: null, open: false, current: null, next: null, events: [] },
]

function calendar(perms: string[], requests: string[] = []) {
  return [
    http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
    http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('calendar:read', ...perms))),
    http.get('/api/v1/terms', () => HttpResponse.json({ items: [term] })),
    http.get('/api/v1/terms/current', () => HttpResponse.json(term)),
    http.get('/api/v1/faculties', () =>
      HttpResponse.json({
        items: [
          {
            id: muh,
            code: 'MUH',
            name_tr: 'Mühendislik Fakültesi',
            name_en: 'Faculty of Engineering',
            unit_type: 'FACULTY',
            campus: null,
            is_active: true,
          },
        ],
      }),
    ),
    http.get('/api/v1/calendar/event-types', () => HttpResponse.json({ items: [registration, addDrop] })),
    http.get('/api/v1/calendar/windows', ({ request }) => {
      const facultyId = new URL(request.url).searchParams.get('faculty_id')
      requests.push(`windows:${facultyId ?? ''}`)
      return HttpResponse.json({ term, at: '2026-10-09T09:00:00Z', items: windows(facultyId) })
    }),
    http.get('/api/v1/terms/:id/events', ({ request }) => {
      const facultyId = new URL(request.url).searchParams.get('faculty_id')
      return HttpResponse.json({ items: facultyId ? [event(), extension] : [event()] })
    }),
  ]
}

describe('akademik takvim', () => {
  it('dönemin pencerelerini gösterir; birim seçilince birimin penceresi uygulanır', async () => {
    const requests: string[] = []
    server.use(...calendar([], requests))
    const { user, router } = renderApp('/takvim')

    const tile = await screen.findByRole('listitem', { name: 'Ekle-bırak (öğrenci)' })
    expect(within(tile).getByText('Kapalı')).toBeInTheDocument()
    expect(within(tile).getByText('Bu dönem için tanımlanmadı')).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: /Öğrenci ders seçme işlemleri/ })).toBeInTheDocument()
    // Takvim yöneticisi olmayan kişi olay ekleyemez.
    expect(screen.queryByRole('button', { name: 'Yeni olay' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: 'Birim' }))
    await user.click(await screen.findByRole('option', { name: 'Mühendislik Fakültesi' }))

    const extended = await screen.findByRole('listitem', { name: 'Ekle-bırak (öğrenci)' })
    await waitFor(() => expect(within(extended).getByText('Açık')).toBeInTheDocument())
    expect(within(extended).getByText('Birim takvimi')).toBeInTheDocument()
    expect(await screen.findByText('Ekle-bırak (Mühendislik, uzatma)')).toBeInTheDocument()
    expect(router.state.location.search).toBe(`?birim=${muh}`)
    expect(requests).toContain(`windows:${muh}`)
  })

  it('takvim yöneticisi olay ekler; çakışma hatası formda gösterilir', async () => {
    let attempt = 0
    let body: Record<string, unknown> = {}
    server.use(
      ...calendar(['calendar:manage']),
      http.post('/api/v1/terms/:id/events', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        attempt++
        return attempt === 1
          ? HttpResponse.json(problem(409, 'EVENT_OVERLAP'), { status: 409 })
          : HttpResponse.json(event({ id: 'yeni' }), { status: 201 })
      }),
    )
    const { user } = renderApp('/takvim')

    await user.click(await screen.findByRole('button', { name: 'Yeni olay' }))
    const dialog = await screen.findByRole('dialog', { name: 'Yeni takvim olayı' })
    await user.click(within(dialog).getByRole('button', { name: 'Kaydet' }))
    expect(await within(dialog).findAllByText('Bu alan zorunlu.')).not.toHaveLength(0)

    await user.click(within(dialog).getByRole('combobox', { name: 'Olay türü' }))
    await user.click(await screen.findByRole('option', { name: 'Öğrenci ders seçme işlemleri' }))
    await user.type(within(dialog).getByLabelText('Başlangıç'), '2027-02-01T10:00')
    await user.type(within(dialog).getByLabelText('Bitiş'), '2027-02-06T00:00')
    await user.click(within(dialog).getByRole('button', { name: 'Kaydet' }))

    expect(
      await within(dialog).findByText('Bu kapsamda aynı türden, zaman olarak çakışan bir olay var.'),
    ).toBeInTheDocument()
    expect(body).toMatchObject({
      type: 'COURSE_REGISTRATION',
      scope_type: 'UNIVERSITY',
      is_published: true,
      starts_at: new Date('2027-02-01T10:00').toISOString(),
    })

    await user.click(within(dialog).getByRole('button', { name: 'Kaydet' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(await screen.findByText('Olay eklendi.')).toBeInTheDocument()
  })

  it('olay düzenlenirken sürüm If-Match ile gönderilir ve silme onay ister', async () => {
    let ifMatch = ''
    let deleted = false
    server.use(
      ...calendar(['calendar:manage']),
      http.put('/api/v1/calendar-events/:id', ({ request }) => {
        ifMatch = request.headers.get('If-Match') ?? ''
        return HttpResponse.json(event({ version: 2 }))
      }),
      http.delete('/api/v1/calendar-events/:id', () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { user } = renderApp('/takvim')

    await user.click(
      await screen.findByRole('button', { name: 'Öğrenci ders seçme işlemleri olayını düzenle' }),
    )
    const dialog = await screen.findByRole('dialog', { name: 'Takvim olayını düzenle' })
    expect(within(dialog).getByRole('combobox', { name: 'Olay türü' })).toBeDisabled()
    await user.click(within(dialog).getByRole('button', { name: 'Kaydet' }))
    await waitFor(() => expect(ifMatch).toBe('"1"'))

    await user.click(await screen.findByRole('button', { name: 'Öğrenci ders seçme işlemleri olayını sil' }))
    const confirm = await screen.findByRole('alertdialog', { name: 'Olay silinsin mi?' })
    await user.click(within(confirm).getByRole('button', { name: 'Sil' }))
    await waitFor(() => expect(deleted).toBe(true))
    expect(await screen.findByText('Olay silindi.')).toBeInTheDocument()
  })
})
