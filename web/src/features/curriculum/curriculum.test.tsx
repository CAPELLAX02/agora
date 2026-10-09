import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Curriculum, CurriculumDetail, CurriculumItem } from '@/shared/api/generated'
import { grants, problem, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const program = {
  id: 'p-bil',
  code: 'BIL-EN-NO',
  name_tr: 'Bilgisayar Mühendisliği (İngilizce)',
  name_en: 'Computer Engineering',
}

const version = (over: Partial<Curriculum> = {}): Curriculum => ({
  id: 'c-2026',
  program,
  name_tr: '2026 Ders Planı',
  name_en: '2026 Curriculum',
  effective_from_year: 2026,
  effective_to_year: null,
  total_ects_required: 240,
  status: 'ACTIVE',
  decision_ref: null,
  copied_from_id: null,
  activated_at: '2026-09-01T00:00:00Z',
  archived_at: null,
  student_count: 290,
  version: 1,
  ...over,
})

const course = (code: string, name: string, semester: number, ects: number): CurriculumItem => ({
  id: `i-${code}`,
  semester_no: semester,
  item_type: 'COURSE',
  code,
  name_tr: name,
  name_en: name,
  course: { id: `c-${code}`, code, name_tr: name, name_en: name, ects },
  course_kind: 'REGULAR',
  has_prerequisites: code === 'COM2044',
  elective_group: null,
  theory_hours: 3,
  practice_hours: 2,
  national_credit: 4,
  ects,
  course_count: 1,
  is_compulsory: true,
  position: 0,
})

const slot: CurriculumItem = {
  id: 'i-comte04',
  semester_no: 7,
  item_type: 'ELECTIVE_SLOT',
  code: 'COMTE04',
  name_tr: '4. Sınıf Teknik Seçmeli Dersler',
  name_en: '4th Year Technical Electives',
  course: null,
  course_kind: null,
  has_prerequisites: false,
  elective_group: {
    id: 'g-comte04',
    code: 'COMTE04',
    name_tr: '4. Sınıf Teknik Seçmeli Dersler',
    name_en: '4th Year Technical Electives',
    kind: 'TECHNICAL',
  },
  theory_hours: 12,
  practice_hours: 0,
  national_credit: 12,
  ects: 16,
  course_count: 4,
  is_compulsory: false,
  position: 2,
}

const detail = (over: Partial<Curriculum> = {}): CurriculumDetail => ({
  ...version(over),
  items: [
    course('COM1007', 'Bilgisayar Programlama I', 1, 7),
    course('COM2044', 'Nesne Yönelimli Programlama', 4, 6),
    slot,
  ],
  summary: {
    total_ects: 29,
    total_credit: 20,
    compulsory_ects: 13,
    elective_ects: 16,
    semesters: [
      { semester_no: 1, ects: 7, national_credit: 4, item_count: 1 },
      { semester_no: 4, ects: 6, national_credit: 4, item_count: 1 },
      { semester_no: 7, ects: 16, national_credit: 12, item_count: 1 },
    ],
    elective_kinds: [{ kind: 'TECHNICAL', ects: 16 }],
  },
})

const pool = http.get('/api/v1/elective-groups/:id', () =>
  HttpResponse.json({
    id: 'g-comte04',
    code: 'COMTE04',
    name_tr: '4. Sınıf Teknik Seçmeli Dersler',
    name_en: '4th Year Technical Electives',
    owner_department: null,
    kind: 'TECHNICAL',
    is_active: true,
    course_count: 1,
    version: 1,
    courses: [
      { id: 'c-4569', code: 'COM4569', name_tr: 'Veri Madenciliği', name_en: 'Data Mining', ects: 4 },
    ],
  }),
)

const session = (...perms: string[]) => [
  http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
  http.get('/api/v1/me/permissions', () =>
    HttpResponse.json(grants('curriculum:read', 'course:read', ...perms)),
  ),
]

describe('ders planı', () => {
  it('öğrenci her program kaydı için ders planını görür; seçmeli havuz açılır', async () => {
    server.use(
      ...session('registration:manage_own'),
      pool,
      http.get('/api/v1/me/curricula', () =>
        HttpResponse.json({
          items: [
            {
              student_program_id: 'sp-1',
              enrollment_kind: 'MAJOR',
              admission_year: 2026,
              program,
              curriculum: detail(),
            },
            {
              student_program_id: 'sp-2',
              enrollment_kind: 'MINOR',
              admission_year: 2027,
              program: { id: 'p-mat', code: 'MAT-NO', name_tr: 'Matematik', name_en: 'Mathematics' },
              curriculum: null,
            },
          ],
        }),
      ),
    )
    const { user } = renderApp('/ders-planim')

    expect(await screen.findByRole('heading', { name: /2026 Ders Planı/ })).toBeInTheDocument()
    expect(screen.getByText(/Giriş yılınız 2026/)).toBeInTheDocument()
    const semester7 = screen.getByRole('region', { name: '7. yarıyıl' })
    expect(within(semester7).getByText('4 ders')).toBeInTheDocument()
    expect(
      within(screen.getByRole('region', { name: '4. yarıyıl' })).getByLabelText('Ön koşulu var'),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Mezuniyet özeti')).toHaveTextContent('Toplam29 / 240 AKTS')

    await user.click(within(semester7).getByRole('button', { name: /4. Sınıf Teknik Seçmeli Dersler/ }))
    expect(await screen.findByRole('link', { name: /COM4569 Veri Madenciliği/ })).toHaveAttribute(
      'href',
      '/dersler/c-4569',
    )

    await user.click(screen.getByRole('tab', { name: 'Matematik · Yandal' }))
    expect(
      await screen.findByText(/Matematik kaydınız henüz bir ders planı sürümüne bağlanmamış/),
    ).toBeInTheDocument()
  })

  it('müfredat yöneticisi taslağı yürürlüğe koyar; sunucunun AKTS uyarısı gösterilir', async () => {
    let activations = 0
    let removed = ''
    server.use(
      ...session('curriculum:manage'),
      pool,
      http.get('/api/v1/programs/:id/curricula', () =>
        HttpResponse.json({
          items: [
            version({
              id: 'c-2027',
              name_tr: '2027 Ders Planı',
              status: 'DRAFT',
              effective_from_year: 2027,
              student_count: 0,
            }),
            version(),
          ],
        }),
      ),
      http.get('/api/v1/curricula/:id', ({ params }) =>
        HttpResponse.json(
          params.id === 'c-2027'
            ? detail({
                id: 'c-2027',
                name_tr: '2027 Ders Planı',
                status: 'DRAFT',
                effective_from_year: 2027,
                student_count: 0,
              })
            : detail(),
        ),
      ),
      http.post('/api/v1/curricula/:id/activate', () => {
        activations++
        return HttpResponse.json(
          problem(409, 'ECTS_TOTAL_MISMATCH', {
            detail: 'Satırların AKTS toplamı 29.0, müfredatın gerektirdiği 240.0.',
          }),
          { status: 409 },
        )
      }),
      http.delete('/api/v1/curricula/:id/items/:itemId', ({ params }) => {
        removed = String(params.itemId)
        return HttpResponse.json(detail({ id: 'c-2027', status: 'DRAFT' }))
      }),
    )
    const { user, router } = renderApp('/ders-planlari?birim=f&bolum=d&program=p-bil')

    // Varsayılan olarak yürürlükteki sürüm açılır; düzenleme düğmeleri yalnızca taslakta.
    expect(await screen.findByRole('heading', { name: /2026 Ders Planı/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Yürürlüğe koy' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Arşivle' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /2027 Ders Planı/ }))
    expect(await screen.findByRole('heading', { name: /2027 Ders Planı/ })).toBeInTheDocument()
    expect(router.state.location.search).toContain('surum=c-2027')

    await user.click(screen.getByRole('button', { name: 'COM1007 satırını kaldır' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Kaldır' }))
    await waitFor(() => expect(removed).toBe('i-COM1007'))

    await user.click(screen.getByRole('button', { name: 'Yürürlüğe koy' }))
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Yürürlüğe koy' }),
    )
    expect(
      await screen.findByText('Satırların AKTS toplamı 29.0, müfredatın gerektirdiği 240.0.'),
    ).toBeInTheDocument()
    expect(activations).toBe(1)
  })

  it('yeni sürüm bir öncekinden kopyalanarak açılır', async () => {
    let body: Record<string, unknown> = {}
    server.use(
      ...session('curriculum:manage'),
      pool,
      http.get('/api/v1/programs/:id/curricula', () => HttpResponse.json({ items: [version()] })),
      http.get('/api/v1/curricula/:id', () => HttpResponse.json(detail())),
      http.post('/api/v1/programs/:id/curricula', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(detail({ id: 'c-yeni', status: 'DRAFT' }), { status: 201 })
      }),
    )
    const { user, router } = renderApp('/ders-planlari?birim=f&bolum=d&program=p-bil')

    await user.click(await screen.findByRole('button', { name: 'Yeni sürüm' }))
    const dialog = await screen.findByRole('dialog', { name: 'Yeni ders planı sürümü' })
    await user.type(within(dialog).getByLabelText('Ad (Türkçe)'), '2027 Ders Planı')
    await user.type(within(dialog).getByLabelText('Ad (İngilizce)'), '2027 Curriculum')
    await user.clear(within(dialog).getByLabelText('İlk giriş yılı'))
    await user.type(within(dialog).getByLabelText('İlk giriş yılı'), '2027')
    await user.click(within(dialog).getByRole('button', { name: 'Kaydet' }))

    await waitFor(() =>
      expect(body).toEqual({
        name_tr: '2027 Ders Planı',
        name_en: '2027 Curriculum',
        effective_from_year: 2027,
        effective_to_year: null,
        total_ects_required: 240,
        copy_from_id: 'c-2026',
      }),
    )
    await waitFor(() => expect(router.state.location.search).toContain('surum=c-yeni'))
  })
})
