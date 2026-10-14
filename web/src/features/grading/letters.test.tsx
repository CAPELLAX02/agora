import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Gradebook, LetterGrades } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const closed = { open: false, starts_at: null, ends_at: null, next_starts_at: null }
const open = {
  open: true,
  starts_at: '2026-10-14T07:00:00Z',
  ends_at: '2026-11-04T07:00:00Z',
  next_starts_at: null,
}

const row = (
  id: string,
  first: string,
  last: string,
  over: Partial<LetterGrades['students'][number]> = {},
) => ({
  enrollment_id: id,
  student: { id: `st-${id}`, student_no: `2629010${id.slice(1)}`, first_name: first, last_name: last },
  attendance_status: 'CONTINUING' as const,
  complete: true,
  missing_components: [],
  weighted_score: 72.5,
  rounded_score: 73,
  computed_letter: 'C1',
  reason: 'RANGE' as const,
  used_makeup: false,
  override: null,
  letter: 'C1',
  passing: true,
  makeup_eligible: false,
  makeup_letter: null,
  previous_letter: null,
  ...over,
})

function letters(over: Partial<LetterGrades> = {}): LetterGrades {
  return {
    section_id: 's1',
    grading_mode: 'LETTER',
    published_at: null,
    published_by: null,
    can_publish: true,
    can_override: true,
    publish_window: open,
    override_window: open,
    makeup_published_at: null,
    can_publish_makeup: false,
    makeup_window: closed,
    final_min_score: 50,
    scale: [
      { letter: 'A', min_score: 90, max_score: 100, passing: true },
      { letter: 'C1', min_score: 70, max_score: 74, passing: true },
      { letter: 'F1', min_score: 50, max_score: 59, passing: false },
    ],
    students: [
      row('e1', 'Ada', 'Aydemir'),
      row('e2', 'Berk', 'Karaca', {
        weighted_score: 48,
        rounded_score: 48,
        computed_letter: 'F3',
        reason: 'FINAL_BELOW_MIN',
        letter: 'F3',
        passing: false,
      }),
      row('e3', 'Ceren', 'Doğan', {
        complete: false,
        missing_components: ['Final sınavı'],
        weighted_score: null,
        rounded_score: null,
        computed_letter: null,
        reason: null,
        letter: null,
        passing: null,
      }),
    ],
    distribution: [
      { letter: 'A', count: 0 },
      { letter: 'C1', count: 1 },
      { letter: 'F3', count: 1 },
    ],
    summary: { students: 3, complete: 2, mean: 60.25, median: 60.25, stddev: 12.25, pass_rate: 0.5 },
    ...over,
  }
}

const book = {
  section: {
    id: 's1',
    section_code: '1',
    course: {
      id: 'c',
      code: 'COM1007',
      name_tr: 'Bilgisayar Programlama I',
      name_en: 'P1',
      grading_mode: 'LETTER',
    },
    term: { id: 't1', code: '2026-FALL' },
    plan_locked: true,
    instructor_role: 'PRIMARY',
    grades_published_at: null,
    makeup_published_at: null,
  },
  components: [],
  students: [],
  can_enter: true,
  mfa_required: false,
} as unknown as Gradebook

function setup(data: LetterGrades) {
  const calls: { path: string; body: unknown }[] = []
  server.use(
    http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
    http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('course:read', 'grade:finalize'))),
    http.get('/api/v1/sections/:id/gradebook', () => HttpResponse.json(book)),
    http.get('/api/v1/sections/:id/letter-grades', () => HttpResponse.json(data)),
    http.put('/api/v1/sections/:id/letter-overrides', async ({ request }) => {
      calls.push({ path: 'overrides', body: await request.json() })
      return HttpResponse.json(data)
    }),
    http.post('/api/v1/sections/:id/letter-grades/publish', () => {
      calls.push({ path: 'publish', body: null })
      return HttpResponse.json({ ...data, published_at: '2026-10-14T10:00:00Z' })
    }),
    http.post('/api/v1/sections/:id/makeup/publish', () => {
      calls.push({ path: 'makeup', body: null })
      return HttpResponse.json(data)
    }),
  )
  return { calls, ...renderApp('/subeler/s1/harf-notlari') }
}

describe('harf notları', () => {
  it('önizlemede istatistikler, dağılım, nedenler ve eksik notlar görünür; elle harf gerekçe ister', async () => {
    const { user, calls } = setup(letters())
    expect(await screen.findByRole('heading', { name: 'Harf notları' })).toBeInTheDocument()
    expect(screen.getByText('2/3')).toBeInTheDocument()
    expect(screen.getByText('%50')).toBeInTheDocument()
    const chart = screen.getByRole('list', { name: 'Harf dağılımı' })
    expect(within(chart).getByText('F3: 1 öğrenci')).toBeInTheDocument()
    expect(screen.getByText('Final barajı altı')).toBeInTheDocument()
    expect(screen.getByText('Eksik: Final sınavı')).toBeInTheDocument()
    expect(screen.getByText(/1 öğrencinin notu eksik/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Ceren Doğan için elle harf ver' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Elle harf ver' }))
    expect(within(dialog).getByRole('alert')).toHaveTextContent('Bir harf seçin')
    await user.click(within(dialog).getByRole('combobox', { name: 'Harf' }))
    await user.click(await screen.findByRole('option', { name: /F1/ }))
    await user.type(within(dialog).getByLabelText('Gerekçe'), 'Kısa')
    await user.click(within(dialog).getByRole('button', { name: 'Elle harf ver' }))
    expect(within(dialog).getByRole('alert')).toHaveTextContent('En az 5 karakterlik bir gerekçe yazın.')
    await user.type(within(dialog).getByLabelText('Gerekçe'), ' değil, final sınavına mazeretsiz girmedi')
    await user.click(within(dialog).getByRole('button', { name: 'Elle harf ver' }))
    await waitFor(() =>
      expect(calls).toEqual([
        {
          path: 'overrides',
          body: {
            items: [
              { enrollment_id: 'e3', letter: 'F1', reason: 'Kısa değil, final sınavına mazeretsiz girmedi' },
            ],
          },
        },
      ]),
    )
  })

  it('ilan onayla yapılır', async () => {
    const { user, calls } = setup(letters())
    await user.click(await screen.findByRole('button', { name: 'Harf notlarını ilan et' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/3 öğrencinin harf notu kesinleşir/)).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Harf notlarını ilan et' }))
    await waitFor(() => expect(calls.map((c) => c.path)).toEqual(['publish']))
    expect(await screen.findByText('Harf notları ilan edildi')).toBeInTheDocument()
  })

  it('ilandan sonra bütünleme hakkı olanlar ve bütünlemeyle olacak harf görünür, bütünleme ilan edilir', async () => {
    const { user, calls } = setup(
      letters({
        published_at: '2026-10-14T10:00:00Z',
        published_by: 'Ayşe Demir',
        can_publish: false,
        can_override: false,
        can_publish_makeup: true,
        makeup_window: open,
        students: [
          row('e1', 'Ada', 'Aydemir'),
          row('e2', 'Berk', 'Karaca', {
            computed_letter: 'F3',
            letter: 'F3',
            passing: false,
            makeup_eligible: true,
            makeup_letter: 'C1',
          }),
          row('e3', 'Ceren', 'Doğan', {
            computed_letter: 'F1',
            letter: 'F1',
            passing: false,
            makeup_eligible: true,
          }),
        ],
      }),
    )
    expect(await screen.findByText(/Ayşe Demir tarafından ilan edildi/)).toBeInTheDocument()
    expect(screen.getByText('2 öğrencinin bütünleme hakkı var', { exact: false })).toBeInTheDocument()
    expect(screen.getByText('1 bütünleme notu girildi', { exact: false })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Bütünleme notlarını gir' })).toHaveAttribute(
      'href',
      '/subeler/s1/notlar',
    )
    const berk = screen.getByRole('row', { name: /Berk Karaca/ })
    expect(within(berk).getByText('Bütünlemeyle')).toBeInTheDocument()
    expect(
      within(screen.getByRole('row', { name: /Ceren Doğan/ })).getByText('Bütünleme hakkı'),
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /elle harf ver/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Bütünleme sonuçlarını ilan et' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Bütünleme sonuçlarını ilan et' }))
    await waitFor(() => expect(calls.map((c) => c.path)).toEqual(['makeup']))
  })
})
