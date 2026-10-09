import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Offering, OfferingDetail, Section, Term } from '@/shared/api/generated'
import { problem, tokens } from '@/test/fixtures'
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
const bil = { id: 'd-bil', code: 'BIL', name_tr: 'Bilgisayar Mühendisliği', name_en: 'Computer Engineering' }

const offering = (over: Partial<Offering> = {}): Offering => ({
  id: 'o-2039',
  term: { id: term.id, code: term.code },
  course: {
    id: 'c-2039',
    code: 'COM2039',
    name_tr: 'Ayrık Yapılar',
    name_en: 'Discrete Structures',
    theory_hours: 3,
    practice_hours: 0,
    national_credit: 3,
    ects: 5,
    language: 'EN',
  },
  department: bil,
  status: 'PLANNED',
  external_ref: null,
  note: null,
  section_count: 1,
  total_capacity: 80,
  total_enrolled: 0,
  version: 1,
  ...over,
})

const section: Section = {
  id: 's-1',
  offering_id: 'o-2039',
  section_code: '1',
  capacity: 80,
  enrolled_count: 0,
  quota_mode: 'OPEN',
  instruction_mode: 'IN_PERSON',
  language: 'EN',
  status: 'ACTIVE',
  instructors: [],
  quotas: [],
  slots: [],
  version: 1,
}

const detail = (over: Partial<Offering> = {}): OfferingDetail => ({ ...offering(over), sections: [section] })

// Bölüm başkanı: ders açma yetkileri kendi bölümüyle sınırlı.
const head = [
  http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
  http.get('/api/v1/me/permissions', () =>
    HttpResponse.json({
      items: ['course:read', 'offering:manage', 'section:manage', 'schedule:manage', 'quota:manage'].map(
        (permission) => ({
          permission,
          scope_type: permission === 'course:read' ? 'NONE' : 'DEPARTMENT',
          scope_id: permission === 'course:read' ? null : bil.id,
        }),
      ),
    }),
  ),
  http.get('/api/v1/terms', () => HttpResponse.json({ items: [term] })),
  http.get('/api/v1/terms/current', () => HttpResponse.json(term)),
  http.get('/api/v1/departments/:id', () =>
    HttpResponse.json({
      ...bil,
      faculty: { id: 'f-muh', code: 'MUH', name_tr: 'Mühendislik Fakültesi' },
      is_active: true,
    }),
  ),
  http.get('/api/v1/faculties', () =>
    HttpResponse.json({
      items: [
        {
          id: 'f-muh',
          code: 'MUH',
          name_tr: 'Mühendislik Fakültesi',
          name_en: 'Engineering',
          unit_type: 'FACULTY',
          campus: null,
          is_active: true,
        },
      ],
    }),
  ),
  http.get('/api/v1/faculties/:id/departments', () =>
    HttpResponse.json({
      items: [
        { ...bil, faculty: { id: 'f-muh', code: 'MUH', name_tr: 'Mühendislik Fakültesi' }, is_active: true },
      ],
    }),
  ),
]

describe('ders açma', () => {
  it('bölüm başkanının bölümü varsayılan seçilir; ders açılınca ayrıntıya gidilir', async () => {
    let created: Record<string, unknown> = {}
    server.use(
      ...head,
      http.get('/api/v1/terms/:id/offerings', ({ request }) => {
        expect(new URL(request.url).searchParams.get('department_id')).toBe(bil.id)
        return HttpResponse.json({ items: [offering()] })
      }),
      http.get('/api/v1/courses', () =>
        HttpResponse.json({
          items: [
            {
              id: 'c-2043',
              code: 'COM2043',
              name_tr: 'Programlama Dili Kavramları',
              name_en: 'Programming Language Concepts',
              owner_department: bil,
              theory_hours: 3,
              practice_hours: 2,
              national_credit: 4,
              ects: 5,
              language: 'EN',
              kind: 'REGULAR',
              grading_mode: 'LETTER',
              description_tr: null,
              description_en: null,
              learning_outcomes: [],
              is_active: true,
              version: 1,
            },
          ],
        }),
      ),
      http.post('/api/v1/terms/:id/offerings', async ({ request }) => {
        created = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(detail({ id: 'o-yeni' }), { status: 201 })
      }),
      http.get('/api/v1/offerings/:id', () => HttpResponse.json(detail({ id: 'o-yeni' }))),
    )
    const { user, router } = renderApp('/ders-acma')

    const row = (await screen.findByRole('link', { name: 'Ayrık Yapılar' })).closest('tr')!
    expect(within(row).getByText('Planlanıyor')).toBeInTheDocument()
    expect(within(row).getByText('0 / 80')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Ders aç' }))
    const dialog = await screen.findByRole('dialog', { name: 'Ders aç' })
    await user.type(within(dialog).getByRole('searchbox'), 'com2043')
    await user.click(await within(dialog).findByRole('button', { name: /COM2043/ }))
    await user.click(within(dialog).getByRole('button', { name: 'Dersi aç' }))

    await waitFor(() => expect(router.state.location.pathname).toBe('/ders-acma/o-yeni'))
    expect(created).toEqual({ course_id: 'c-2043', department_id: bil.id })
  })

  it('oturum çakışmasında sunucunun mesajı gösterilir; öğretim elemanı atanır; ders seçmeye açılır', async () => {
    let slot: Record<string, unknown> = {}
    let instructors: unknown = null
    let update: { ifMatch: string; body: unknown } | null = null
    server.use(
      ...head,
      http.get('/api/v1/offerings/:id', () => HttpResponse.json(detail())),
      http.get('/api/v1/buildings', () =>
        HttpResponse.json({ items: [{ id: 'b-a', code: 'MUH-A', name: 'Mühendislik A Blok' }] }),
      ),
      http.get('/api/v1/classrooms', () =>
        HttpResponse.json({ items: [{ id: 'r-a105', code: 'A105', name: 'Derslik 105', capacity: 80 }] }),
      ),
      http.post('/api/v1/sections/:id/slots', async ({ request }) => {
        slot = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(
          problem(409, 'CLASSROOM_CONFLICT', {
            detail: 'Derslik bu saatte dolu: COM2067-1 (Pazartesi 09:00-11:50).',
          }),
          { status: 409 },
        )
      }),
      http.get('/api/v1/instructors', () =>
        HttpResponse.json({
          items: [
            {
              staff_id: 'st-5',
              staff_no: 'P10005',
              title: 'Doç. Dr.',
              first_name: 'Selin',
              last_name: 'Koç',
              department: bil,
            },
          ],
        }),
      ),
      http.put('/api/v1/sections/:id/instructors', async ({ request }) => {
        instructors = await request.json()
        return HttpResponse.json(section)
      }),
      http.put('/api/v1/offerings/:id', async ({ request }) => {
        update = { ifMatch: request.headers.get('If-Match') ?? '', body: await request.json() }
        return HttpResponse.json(detail({ status: 'OPEN', version: 2 }))
      }),
    )
    const { user } = renderApp('/ders-acma/o-2039')

    const card = await screen.findByRole('region', { name: 'Şube 1' })
    await user.click(within(card).getByRole('button', { name: 'Oturum ekle' }))
    const dialog = await screen.findByRole('dialog', { name: 'Oturum ekle' })
    await user.click(within(dialog).getByRole('combobox', { name: 'Bina' }))
    await user.click(await screen.findByRole('option', { name: 'MUH-A · Mühendislik A Blok' }))
    await user.click(within(dialog).getByRole('combobox', { name: 'Derslik' }))
    await user.click(await screen.findByRole('option', { name: /A105/ }))
    await user.click(within(dialog).getByRole('button', { name: 'Oturum ekle' }))

    expect(
      await within(dialog).findByText('Derslik bu saatte dolu: COM2067-1 (Pazartesi 09:00-11:50).'),
    ).toBeInTheDocument()
    expect(slot).toEqual({
      day_of_week: 1,
      start_time: '09:00',
      end_time: '11:50',
      session_type: 'THEORY',
      classroom_id: 'r-a105',
    })
    await user.click(within(dialog).getByRole('button', { name: 'Vazgeç' }))

    await user.click(within(card).getAllByRole('button', { name: 'Düzenle' })[1]!)
    const people = await screen.findByRole('dialog', { name: 'Şube 1 öğretim elemanları' })
    await user.type(within(people).getByRole('searchbox', { name: 'Öğretim elemanı ekle' }), 'selin')
    await user.click(await within(people).findByRole('button', { name: /Doç. Dr. Selin Koç/ }))
    expect(within(people).getByRole('combobox', { name: 'Doç. Dr. Selin Koç rolü' })).toHaveTextContent(
      'Sorumlu',
    )
    await user.click(within(people).getByRole('button', { name: 'Kaydet' }))
    await waitFor(() => expect(instructors).toEqual({ items: [{ staff_id: 'st-5', role: 'PRIMARY' }] }))

    await user.click(screen.getByRole('button', { name: 'Ders seçmeye aç' }))
    await waitFor(() => expect(update).toEqual({ ifMatch: '"1"', body: { status: 'OPEN' } }))
  })
})
