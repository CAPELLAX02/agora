import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { StudentGrades } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const terms: StudentGrades['terms'] = [
  { id: 't1', code: '2025-SPRING', term_type: 'SPRING', is_current: false },
  { id: 't2', code: '2026-FALL', term_type: 'FALL', is_current: true },
]

const component = (over: Partial<StudentGrades['courses'][number]['components'][number]>) => ({
  id: 'k',
  type_code: 'MIDTERM',
  category: 'IN_TERM' as const,
  label_tr: 'Ara sınav',
  label_en: 'Midterm',
  weight: 40,
  scheduled_on: null,
  published_at: null,
  score: null,
  absent: false,
  class_average: null,
  ...over,
})

const current: StudentGrades = {
  student_program_id: 'sp-1',
  terms,
  term: terms[1]!,
  courses: [
    {
      enrollment_id: 'e1',
      course: { id: 'c1', code: 'COM2039', name_tr: 'Ayrık Yapılar', name_en: 'Discrete Structures' },
      section_code: '1',
      ects: 5,
      status: 'ENROLLED',
      attendance_status: 'CONTINUING',
      letter_grade: null,
      is_passing: null,
      weighted_score: null,
      grade_published_at: null,
      components: [
        component({ id: 'k1', published_at: '2026-11-10T10:00:00Z', score: 82, class_average: 67.5 }),
        component({ id: 'k2', type_code: 'FINAL', category: 'FINAL', label_tr: 'Final sınavı', weight: 60 }),
      ],
      class_stats: null,
    },
    {
      enrollment_id: 'e2',
      course: { id: 'c2', code: 'PHY0151', name_tr: 'Fizik Laboratuvarı', name_en: 'Physics Lab' },
      section_code: '1',
      ects: 2,
      status: 'ENROLLED',
      attendance_status: 'ATTENDANCE_FAIL',
      letter_grade: null,
      is_passing: null,
      weighted_score: null,
      grade_published_at: null,
      components: [
        component({ id: 'k3', published_at: '2026-11-10T10:00:00Z', absent: true, class_average: 70 }),
      ],
      class_stats: null,
    },
  ],
}

const spring: StudentGrades = {
  ...current,
  term: terms[0]!,
  courses: [
    {
      ...current.courses[0]!,
      enrollment_id: 'e0',
      status: 'COMPLETED',
      letter_grade: 'B1',
      is_passing: true,
      weighted_score: 86.4,
      grade_published_at: '2026-06-20T10:00:00Z',
      components: [],
      class_stats: { student_count: 48, mean: 71.25, median: 72 },
    },
  ],
}

describe('notlarım', () => {
  it('dönemin derslerini, yayımlanan bileşen notlarını ve sınıf ortalamasını gösterir; dönem değiştirilir', async () => {
    const asked: (string | null)[] = []
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
      http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('grade:read_own'))),
      http.get('/api/v1/me/programs', () =>
        HttpResponse.json({ items: [{ id: 'sp-1', program: { code: 'BIL' } }] }),
      ),
      http.get('/api/v1/student-programs/:id/grades', ({ request }) => {
        const term = new URL(request.url).searchParams.get('term_id')
        asked.push(term)
        return HttpResponse.json(term === 't1' ? spring : current)
      }),
    )
    const { user } = renderApp('/notlarim')

    const discrete = await screen.findByRole('article', { name: 'COM2039 Ayrık Yapılar' })
    expect(within(discrete).getByText('82')).toBeInTheDocument()
    expect(within(discrete).getByText('Sınıf ort. 67.50')).toBeInTheDocument()
    expect(within(discrete).getByText('Henüz yayımlanmadı')).toBeInTheDocument()
    expect(within(discrete).getByText('Sürüyor')).toBeInTheDocument()

    const lab = screen.getByRole('article', { name: 'PHY0151 Fizik Laboratuvarı' })
    expect(within(lab).getByText('Devamsız')).toBeInTheDocument()
    expect(within(lab).getByText('Girmedi')).toBeInTheDocument()

    const nav = screen.getByRole('navigation', { name: 'Dönemler' })
    expect(within(nav).getByRole('button', { name: /2026-2027 Güz/ })).toHaveAttribute('aria-current', 'page')
    await user.click(within(nav).getByRole('button', { name: '2025-2026 Bahar' }))

    await waitFor(() =>
      expect(
        within(screen.getByRole('article', { name: 'COM2039 Ayrık Yapılar' })).getByText('B1'),
      ).toBeInTheDocument(),
    )
    const past = screen.getByRole('article', { name: 'COM2039 Ayrık Yapılar' })
    expect(within(past).getByText('86.40')).toBeInTheDocument()
    expect(within(past).getByText('71.25')).toBeInTheDocument()
    expect(within(past).getByText('48 öğrenci')).toBeInTheDocument()
    expect(asked).toEqual([null, 't1'])
  })
})
