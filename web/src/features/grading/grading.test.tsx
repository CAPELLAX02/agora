import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { AssessmentPlan, AssessmentType, OfferingDetail, Section } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const types: AssessmentType[] = [
  { code: 'MIDTERM', name_tr: 'Ara sınav', name_en: 'Midterm exam', category: 'IN_TERM' },
  { code: 'HOMEWORK', name_tr: 'Ödev', name_en: 'Homework', category: 'IN_TERM' },
  { code: 'FINAL', name_tr: 'Final sınavı', name_en: 'Final exam', category: 'FINAL' },
  { code: 'MAKEUP', name_tr: 'Bütünleme', name_en: 'Make-up exam', category: 'MAKEUP' },
]
const type = (code: string) => types.find((t) => t.code === code)!

const section: Section = {
  id: 's-1',
  offering_id: 'o-1',
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

const offering: OfferingDetail = {
  id: 'o-1',
  term: { id: 't', code: '2026-FALL' },
  course: {
    id: 'c',
    code: 'COM2039',
    name_tr: 'Ayrık Yapılar',
    name_en: 'Discrete Structures',
    theory_hours: 3,
    practice_hours: 0,
    national_credit: 3,
    ects: 5,
    language: 'EN',
  },
  department: { id: 'd', code: 'BIL', name_tr: 'Bilgisayar Mühendisliği', name_en: 'Computer Engineering' },
  status: 'OPEN',
  external_ref: null,
  note: null,
  section_count: 1,
  total_capacity: 80,
  total_enrolled: 0,
  version: 1,
  sections: [section],
}

const component = (code: string, weight: number, label: string) => ({
  id: `k-${code}`,
  type: type(code),
  sequence_no: 1,
  name_tr: null,
  name_en: null,
  label_tr: label,
  label_en: label,
  weight,
  scheduled_on: null,
})

const plan = (over: Partial<AssessmentPlan> = {}): AssessmentPlan => ({
  section_id: 's-1',
  components: [
    component('MIDTERM', 40, 'Ara sınav'),
    component('FINAL', 60, 'Final sınavı'),
    component('MAKEUP', 60, 'Bütünleme'),
  ],
  in_term_weight: 40,
  final_weight: 60,
  is_complete: true,
  locked_at: null,
  locked_by: null,
  editable: true,
  can_unlock: false,
  version: 2,
  ...over,
})

const common = (...perms: string[]) => [
  http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
  http.get('/api/v1/me/permissions', () =>
    HttpResponse.json(grants('course:read', 'curriculum:read', ...perms)),
  ),
  http.get('/api/v1/sections/:id', () => HttpResponse.json(section)),
  http.get('/api/v1/offerings/:id', () => HttpResponse.json(offering)),
  http.get('/api/v1/assessment-types', () => HttpResponse.json({ items: types })),
]

describe('değerlendirme planı', () => {
  it('öğretim elemanı planı düzenler ve kilitler; toplam yazarken denetlenir', async () => {
    let saved: { ifMatch: string; body: unknown } | null = null
    let locked = false
    server.use(
      ...common('assessment_plan:manage'),
      http.get('/api/v1/sections/:id/assessment-plan', () => HttpResponse.json(plan())),
      http.put('/api/v1/sections/:id/assessment-plan', async ({ request }) => {
        saved = { ifMatch: request.headers.get('If-Match') ?? '', body: await request.json() }
        return HttpResponse.json(plan({ version: 3 }))
      }),
      http.post('/api/v1/sections/:id/assessment-plan/lock', () => {
        locked = true
        return HttpResponse.json(
          plan({ locked_at: '2026-10-09T10:00:00Z', locked_by: 'Selin Koç', editable: false }),
        )
      }),
    )
    const { user } = renderApp('/subeler/s-1/degerlendirme')

    expect(await screen.findByText(/COM2039-1 Ayrık Yapılar · 2026-FALL/)).toBeInTheDocument()
    expect(screen.getByText('finalin yerine')).toBeInTheDocument()
    expect(screen.getByText('%100')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Planı düzenle' }))
    await user.click(screen.getByRole('button', { name: 'Bileşen ekle' }))
    expect(screen.getByRole('status')).toHaveTextContent('Toplam %110 · 1 final')
    const weights = screen.getAllByLabelText('Ağırlık')
    await user.clear(weights[0]!)
    await user.type(weights[0]!, '30')
    expect(screen.getByRole('status')).toHaveTextContent('Toplam %100 · 1 final')
    await user.click(screen.getByRole('button', { name: 'Kaydet' }))

    await waitFor(() =>
      expect(saved).toEqual({
        ifMatch: '"2"',
        body: {
          components: [
            { type: 'MIDTERM', weight: 30 },
            { type: 'FINAL', weight: 60 },
            { type: 'HOMEWORK', weight: 10 },
          ],
        },
      }),
    )

    await user.click(await screen.findByRole('button', { name: 'Planı kilitle' }))
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Planı kilitle' }),
    )
    await waitFor(() => expect(locked).toBe(true))
  })

  it('kilitli planın kilidini bölüm gerekçeyle açar', async () => {
    let reason = ''
    server.use(
      ...common('section:manage'),
      http.get('/api/v1/sections/:id/assessment-plan', () =>
        HttpResponse.json(
          plan({
            locked_at: '2026-10-01T10:00:00Z',
            locked_by: 'Selin Koç',
            editable: false,
            can_unlock: true,
          }),
        ),
      ),
      http.post('/api/v1/sections/:id/assessment-plan/unlock', async ({ request }) => {
        reason = ((await request.json()) as { reason: string }).reason
        return HttpResponse.json(plan())
      }),
    )
    const { user } = renderApp('/subeler/s-1/degerlendirme')

    expect(await screen.findByText(/Selin Koç tarafından kilitlendi/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Planı düzenle' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Kilidi aç' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Kilidi aç' }))
    expect(await within(dialog).findByText('En az 5 karakterlik bir gerekçe yazın.')).toBeInTheDocument()
    await user.type(within(dialog).getByLabelText('Gerekçe'), 'Ara sınav tarihi değişti')
    await user.click(within(dialog).getByRole('button', { name: 'Kilidi aç' }))
    await waitFor(() => expect(reason).toBe('Ara sınav tarihi değişti'))
  })
})

describe('not ölçeği', () => {
  it('harfleri, puan aralıklarını ve yönetmelik parametrelerini gösterir', async () => {
    server.use(
      ...common(),
      http.get('/api/v1/grade-scales', () =>
        HttpResponse.json({
          items: [
            {
              id: 'gs',
              code: 'AU-LISANS',
              name_tr: 'Ankara Üniversitesi lisans not ölçeği',
              name_en: 'Ankara University undergraduate grading scale',
              effective_from_year: 2016,
              is_default: true,
              version: 1,
              items: [
                {
                  letter: 'B1',
                  coefficient: 3.5,
                  min_score: 80,
                  max_score: 89,
                  is_passing: true,
                  counts_in_gpa: true,
                  earns_ects: true,
                  is_attendance_fail: false,
                },
                {
                  letter: 'F1',
                  coefficient: 0,
                  min_score: null,
                  max_score: null,
                  is_passing: false,
                  counts_in_gpa: true,
                  earns_ects: false,
                  is_attendance_fail: true,
                },
              ],
            },
          ],
        }),
      ),
      http.get('/api/v1/regulation-parameters', () =>
        HttpResponse.json({
          items: [
            {
              key: 'ects_limit_by_gpa',
              value: [{ max_gpa: 1.99, ects: 30 }],
              effective_from: '2016-09-01',
              effective_to: null,
              description_tr: 'Genel not ortalamasına göre bir yarıyılda alınabilecek en fazla AKTS',
              note: null,
            },
          ],
        }),
      ),
    )
    renderApp('/not-olcegi')

    const b1 = (await screen.findByRole('cell', { name: 'B1' })).closest('tr')!
    expect(within(b1).getByText('3.50')).toBeInTheDocument()
    expect(within(b1).getByText('80–89')).toBeInTheDocument()
    const f1 = screen.getByRole('cell', { name: 'F1' }).closest('tr')!
    expect(within(f1).getByText('Devamsızlık')).toBeInTheDocument()
    expect(within(f1).getByLabelText('AKTS kazandırır: Hayır')).toBeInTheDocument()
    expect(screen.getByText('max_gpa: 1.99, ects: 30')).toBeInTheDocument()
  })
})
