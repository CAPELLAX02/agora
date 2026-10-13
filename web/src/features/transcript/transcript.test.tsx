import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { StudentProgram, Transcript, TranscriptSimulation } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const program: StudentProgram = {
  id: 'sp-1',
  student: { id: 'st-1', student_no: '23290013', first_name: 'Buse', last_name: 'Demirtaş' },
  program: {
    id: 'p-1',
    code: 'BIL-EN-NO',
    name_tr: 'Bilgisayar Mühendisliği (İngilizce)',
    department: { id: 'd', name_tr: 'Bilgisayar Mühendisliği' },
    faculty: { id: 'f', name_tr: 'Mühendislik Fakültesi' },
  },
  kind: 'MAJOR',
  admission_type: 'OSYS',
  admission_year: 2025,
  admitted_on: '2025-09-19',
  status: 'ACTIVE',
  class_level: 1,
  current_semester: 2,
  normal_semesters: 8,
  max_semesters: 14,
  gpa: 3.41,
  earned_ects: 17,
  advisor: {
    staff_id: 's',
    staff_no: 'P10001',
    title: 'Prof. Dr.',
    first_name: 'Selin',
    last_name: 'Koç',
    since: '2025-09-20T00:00:00Z',
  },
  curriculum: null,
  version: 3,
}

const course = (code: string, name: string, letter: string | null, over: object = {}) => ({
  enrollment_id: `e-${code}-${letter}`,
  course: { id: `c-${code}`, code, name_tr: name, name_en: name },
  ects: 6,
  credit: 5,
  status: letter ? ('COMPLETED' as const) : ('ENROLLED' as const),
  letter_grade: letter,
  coefficient: null,
  is_passing: letter !== 'F2',
  counts_in_gpa: true,
  attempt_no: 1,
  is_repeat: false,
  superseded: false,
  source: 'REGISTRATION' as const,
  letter_options: ['A', 'B1', 'B2', 'B3', 'C1', 'C2', 'C3', 'F1', 'F2', 'F3', 'F4'],
  ...over,
})

const transcript: Transcript = {
  student_program: program,
  program_name_en: 'Computer Engineering (English)',
  degree_level: 'BACHELOR',
  terms: [
    {
      term: { id: 't1', code: '2025-FALL', term_type: 'FALL' },
      courses: [
        course('COM1001', 'Programlama I', 'F2', { superseded: true }),
        course('MTH1001', 'Matematik I', 'B1'),
      ],
      result: {
        attempted_ects: 12,
        earned_ects: 6,
        term_gpa: 1.75,
        cumulative_ects: 6,
        cumulative_gpa: 1.75,
        standing: 'NORMAL',
      },
    },
    {
      term: { id: 't2', code: '2025-SPRING', term_type: 'SPRING' },
      courses: [course('COM1001', 'Programlama I', 'A', { attempt_no: 2, is_repeat: true })],
      result: {
        attempted_ects: 30,
        earned_ects: 30,
        term_gpa: 3.81,
        cumulative_ects: 36,
        cumulative_gpa: 3.41,
        standing: 'HIGH_HONOR',
      },
    },
    {
      term: { id: 't3', code: '2026-FALL', term_type: 'FALL' },
      courses: [course('COM2001', 'Veri Yapıları', null)],
      result: null,
    },
  ],
  summary: {
    gpa: 3.41,
    earned_ects: 36,
    required_ects: 240,
    completed_courses: 3,
    in_progress: 1,
    honor_terms: 0,
    high_honor_terms: 1,
  },
  generated_at: '2026-10-13T10:00:00Z',
}

describe('transkript', () => {
  it('öğrenci dönem dönem notlarını, ortalamalarını ve işaretleri görür', async () => {
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
      http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('transcript:read_own'))),
      http.get('/api/v1/me/programs', () => HttpResponse.json({ items: [program] })),
      http.get('/api/v1/student-programs/:id/transcript', () => HttpResponse.json(transcript)),
    )
    renderApp('/transkript')

    expect(await screen.findByRole('heading', { name: 'Buse Demirtaş' })).toBeInTheDocument()
    expect(screen.getByText('Prof. Dr. Selin Koç')).toBeInTheDocument()
    expect(screen.getByText('Lisans')).toBeInTheDocument()

    const fall = screen.getByRole('region', { name: '2025-2026 Güz' })
    const failed = within(fall).getByRole('row', { name: /COM1001/ })
    expect(within(failed).getByText('F2')).toHaveClass('line-through')
    expect(within(fall).getAllByText('1.75')).toHaveLength(2) // YANO ve GANO

    const spring = screen.getByRole('region', { name: '2025-2026 Bahar' })
    expect(within(spring).getByText('Yüksek onur')).toBeInTheDocument()
    expect(
      within(within(spring).getByRole('row', { name: /COM1001/ })).getByText('tekrar'),
    ).toBeInTheDocument()

    const current = screen.getByRole('region', { name: '2026-2027 Güz' })
    expect(within(current).getByText('Notlar henüz ilan edilmedi')).toBeInTheDocument()
    expect(within(current).getByText('Sürüyor')).toBeInTheDocument()

    expect(screen.getByRole('meter', { name: 'Kazanılan AKTS' })).toHaveAttribute('aria-valuenow', '36')
    expect(screen.getByRole('button', { name: 'Yazdır' })).toBeEnabled()
  })

  it('sanal transkriptte derse varsayımsal not seçilince GANO yeniden hesaplanır, bir şey kaydedilmez', async () => {
    let body: unknown = null
    const result = (gpa: number): TranscriptSimulation => ({
      terms: transcript.terms.map((x) => ({
        term: x.term,
        actual: x.result,
        simulated:
          x.term.id === 't3'
            ? {
                attempted_ects: 6,
                earned_ects: 6,
                term_gpa: 4,
                cumulative_ects: 42,
                cumulative_gpa: gpa,
                standing: 'NORMAL',
              }
            : x.result,
      })),
      summary: { actual_gpa: 3.41, simulated_gpa: gpa, actual_earned_ects: 36, simulated_earned_ects: 42 },
    })
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
      http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('transcript:read_own'))),
      http.get('/api/v1/me/programs', () => HttpResponse.json({ items: [program] })),
      http.get('/api/v1/student-programs/:id/transcript', () => HttpResponse.json(transcript)),
      http.post('/api/v1/student-programs/:id/transcript/simulate', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json(result(3.57))
      }),
    )
    const { user } = renderApp('/transkript')

    await screen.findByRole('heading', { name: 'Buse Demirtaş' })
    await user.click(screen.getByRole('button', { name: 'Ne olur?' }))
    expect(screen.getByRole('heading', { name: 'Sanal transkript' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Yazdır' })).not.toBeInTheDocument()

    await user.selectOptions(screen.getByRole('combobox', { name: 'COM2001 için varsayımsal not' }), 'A')
    await waitFor(() =>
      expect(body).toEqual({ grades: [{ enrollment_id: 'e-COM2001-null', letter_grade: 'A' }] }),
    )
    const bar = screen.getByRole('status', { name: 'Sanal transkript' })
    expect(await within(bar).findByText('+0.16')).toBeInTheDocument()
    expect(within(bar).getByText('3.57')).toBeInTheDocument()
    expect(within(bar).getByText('1 derse varsayımsal not verildi')).toBeInTheDocument()
    const current = screen.getByRole('region', { name: '2026-2027 Güz' })
    expect(within(current).getByText('Sanal')).toBeInTheDocument()

    await user.click(within(bar).getByRole('button', { name: 'Sıfırla' }))
    expect(within(bar).queryByText('+0.16')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Sanal transkripti kapat' }))
    expect(screen.getByRole('button', { name: 'Yazdır' })).toBeInTheDocument()
  })
})
