import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Course, CourseDetail } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const bil = { id: 'd-bil', code: 'BIL', name_tr: 'Bilgisayar Mühendisliği', name_en: 'Computer Engineering' }

const course = (over: Partial<Course> = {}): Course => ({
  id: 'c-2044',
  code: 'COM2044',
  name_tr: 'Nesne Yönelimli Programlama',
  name_en: 'Object Oriented Programming',
  owner_department: bil,
  theory_hours: 3,
  practice_hours: 2,
  national_credit: 4,
  ects: 6,
  language: 'EN',
  kind: 'REGULAR',
  grading_mode: 'LETTER',
  description_tr: null,
  description_en: null,
  learning_outcomes: [],
  is_active: true,
  version: 1,
  ...over,
})

const ref = (id: string, code: string, name: string) => ({ id, code, name_tr: name, name_en: name, ects: 6 })

const detail: CourseDetail = {
  ...course(),
  learning_outcomes: ['Sınıf ve nesne kavramlarını uygular.'],
  prerequisites: [
    { course: ref('c-1002', 'COM1002', 'Bilgisayar Programlama II'), requirement: 'PASSED', group_no: 1 },
    { course: ref('c-0143', 'MTH0143', 'Matematik I'), requirement: 'ATTENDED', group_no: 2 },
    { course: ref('c-0141', 'MTH0141', 'Matematik I (eski)'), requirement: 'ATTENDED', group_no: 2 },
  ],
  required_by: [ref('c-3070', 'COM3070', 'Yazılım Mühendisliği')],
  equivalences: [
    {
      id: 'e1',
      relation: 'REPLACED_BY',
      course: ref('c-2045', 'COM2045', 'Nesne Yönelimli Programlama (yeni)'),
      is_bidirectional: true,
      valid_from_year: 2026,
      note: '2026 ders planı intibak tablosu',
    },
  ],
  elective_groups: [],
}

const session = [
  http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
  http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('course:read'))),
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

describe('ders kataloğu', () => {
  it('ders arar, bölüme göre süzer ve sayfa sayfa yükler', async () => {
    const queries: Record<string, string>[] = []
    server.use(
      ...session,
      http.get('/api/v1/courses', ({ request }) => {
        const params = Object.fromEntries(new URL(request.url).searchParams)
        queries.push(params)
        if (params.cursor) {
          return HttpResponse.json({
            items: [course({ id: 'c-4097', code: 'COM4097', name_tr: 'Staj', kind: 'INTERNSHIP' })],
          })
        }
        return HttpResponse.json({ items: [course()], next_cursor: 'sonraki' })
      }),
    )
    const { user, router } = renderApp('/dersler')

    const row = (await screen.findByRole('link', { name: 'Nesne Yönelimli Programlama' })).closest('tr')!
    expect(within(row).getByText('3-2')).toBeInTheDocument()
    expect(within(row).getByText('Bilgisayar Mühendisliği')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Daha fazla göster' }))
    const staj = (await screen.findByRole('link', { name: 'Staj' })).closest('tr')!
    expect(within(staj).getByText('Staj', { selector: '[data-slot="badge"]' })).toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: 'Birim' }))
    await user.click(await screen.findByRole('option', { name: 'Mühendislik Fakültesi' }))
    await user.click(screen.getByRole('combobox', { name: 'Bölüm' }))
    await user.click(await screen.findByRole('option', { name: 'Bilgisayar Mühendisliği' }))

    await waitFor(() => expect(queries.at(-1)).toEqual({ limit: '50', department_id: 'd-bil' }))
    expect(router.state.location.search).toBe('?birim=f-muh&bolum=d-bil')
  })

  it('ders detayında ön koşul grupları, eşdeğerlikler ve bağımlı dersler görünür', async () => {
    server.use(
      ...session,
      http.get('/api/v1/courses/:id', () => HttpResponse.json(detail)),
    )
    renderApp('/dersler/c-2044')

    expect(
      await screen.findByRole('heading', { name: /COM2044\s*Nesne Yönelimli Programlama/ }),
    ).toBeInTheDocument()
    expect(screen.getByText('Sınıf ve nesne kavramlarını uygular.')).toBeInTheDocument()

    // İki grup VE ile, ikinci gruptaki iki ders VEYA ile bağlanır.
    expect(screen.getByText('ve')).toBeInTheDocument()
    expect(screen.getByText('veya')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /COM1002/ })).toHaveAttribute('href', '/dersler/c-1002')
    expect(screen.getAllByText('almış ve devam şartını sağlamış olmak')).toHaveLength(2)

    expect(screen.getByText('Bu dersin yerini şu ders (yeni kod) almıştır:')).toBeInTheDocument()
    expect(
      screen.getByText('2026 girişlilerden itibaren · 2026 ders planı intibak tablosu'),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /COM3070/ })).toBeInTheDocument()
    expect(screen.getByText('Bir seçmeli havuzda değil.')).toBeInTheDocument()
  })
})
