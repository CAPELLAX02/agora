import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { GradeChangeRequest, Gradebook, Term } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const change = (over: Partial<GradeChangeRequest> = {}): GradeChangeRequest => ({
  id: 'g1',
  status: 'PENDING',
  kind: 'SCORE',
  enrollment_id: 'e1',
  student: { id: 'st', student_no: '22290102', first_name: 'Ece', last_name: 'Arslan' },
  course: { code: 'COM1001', name_tr: 'Bilgisayar Programlama I', name_en: 'Computer Programming I' },
  section: { id: 's1', code: '1' },
  term_code: '2026-FALL',
  component: { id: 'k-final', label_tr: 'Final sınavı', label_en: 'Final exam', weight: 60 },
  old: { letter: 'F2', weighted_score: 50, reason: 'FINAL_BELOW_MIN', score: 45, absent: false },
  new: { letter: 'C2', weighted_score: 66.2, reason: 'RANGE', score: 72, absent: false },
  reason: 'Final kâğıdında toplama hatası',
  requested_by: 'Ayşe Yılmaz',
  requested_at: '2026-10-14T09:00:00Z',
  reviewed_by: null,
  review_note: null,
  decided_at: null,
  can_approve: false,
  can_withdraw: true,
  ...over,
})

const term = {
  id: 't1',
  code: '2026-FALL',
  term_type: 'FALL',
  academic_year: '2026-2027',
  is_current: true,
} as unknown as Term

const gradebook = {
  section: {
    id: 's1',
    section_code: '1',
    course: {
      id: 'c',
      code: 'COM1001',
      name_tr: 'Bilgisayar Programlama I',
      name_en: 'P1',
      grading_mode: 'LETTER',
    },
    term: { id: 't1', code: '2026-FALL' },
    plan_locked: true,
    grades_published_at: '2026-10-10T10:00:00Z',
    makeup_published_at: null,
    instructor_role: 'PRIMARY',
  },
  components: [
    { id: 'k-mid', label_tr: 'Ara sınav', label_en: 'Midterm', weight: 40 },
    { id: 'k-final', label_tr: 'Final sınavı', label_en: 'Final exam', weight: 60 },
  ],
  students: [
    {
      enrollment_id: 'e1',
      student: { id: 'st', student_no: '22290102', first_name: 'Ece', last_name: 'Arslan' },
      letter: 'F2',
      scores: { 'k-final': { score: 45, absent: false } },
    },
  ],
} as unknown as Gradebook

describe('not düzeltme', () => {
  it('öğretim elemanı bileşen notunu düzeltme talebi açar; sonuç gerekçeden önce görünür', async () => {
    let sent: unknown = null
    const mine: GradeChangeRequest[] = []
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
      http.get('/api/v1/me/permissions', () =>
        HttpResponse.json(grants('grade_change:request', 'calendar:read', 'course:read')),
      ),
      http.get('/api/v1/grade-change-requests', () => HttpResponse.json({ items: mine })),
      http.get('/api/v1/terms', () => HttpResponse.json({ items: [term] })),
      http.get('/api/v1/terms/current', () => HttpResponse.json(term)),
      http.get('/api/v1/me/sections', () =>
        HttpResponse.json({
          items: [
            {
              term_id: 't1',
              section_id: 's1',
              section_code: '1',
              offering_id: 'o1',
              course: { id: 'c', code: 'COM1001', name_tr: 'Bilgisayar Programlama I', name_en: 'P1' },
              role: 'PRIMARY',
              students: 1,
              has_schedule: true,
            },
            {
              term_id: 't1',
              section_id: 's9',
              section_code: '1',
              offering_id: 'o9',
              course: { id: 'c9', code: 'COM2043', name_tr: 'Asistanlık', name_en: 'TA' },
              role: 'ASSISTANT',
              students: 40,
              has_schedule: true,
            },
          ],
        }),
      ),
      http.get('/api/v1/sections/:id/gradebook', () => HttpResponse.json(gradebook)),
      http.get('/api/v1/sections/:id/letter-grades', () =>
        HttpResponse.json({ scale: [{ letter: 'A' }, { letter: 'C2' }] }),
      ),
      http.post('/api/v1/grade-change-requests/preview', () =>
        HttpResponse.json({
          kind: 'SCORE',
          component: change().component,
          old: change().old,
          new: change().new,
        }),
      ),
      http.post('/api/v1/grade-change-requests', async ({ request }) => {
        sent = await request.json()
        mine.push(change())
        return HttpResponse.json(change(), { status: 201 })
      }),
    )
    const { user } = renderApp('/not-duzeltme')

    expect(await screen.findByText('Henüz not düzeltme talebiniz yok.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Yeni talep' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('combobox', { name: 'Şube' }))
    expect(screen.queryByRole('option', { name: /COM2043/ })).not.toBeInTheDocument() // asistan talep etmez
    await user.click(await screen.findByRole('option', { name: /COM1001-1/ }))
    await user.click(await within(dialog).findByRole('combobox', { name: 'Öğrenci' }))
    await user.click(await screen.findByRole('option', { name: /22290102/ }))
    await user.click(within(dialog).getByRole('combobox', { name: 'Bileşen' }))
    await user.click(await screen.findByRole('option', { name: /Final sınavı/ }))
    await user.type(within(dialog).getByLabelText('Doğru puan'), '72')

    const preview = await within(dialog).findByRole('status')
    await waitFor(() => expect(within(preview).getByText('C2')).toBeInTheDocument())
    expect(within(preview).getByText('72')).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: 'Talep et' }))
    expect(await within(dialog).findByText('En az 10 karakterlik bir gerekçe yazın.')).toBeInTheDocument()
    await user.type(within(dialog).getByLabelText('Gerekçe'), 'Final kâğıdında toplama hatası')
    await user.click(within(dialog).getByRole('button', { name: 'Talep et' }))

    await waitFor(() =>
      expect(sent).toEqual({
        enrollment_id: 'e1',
        component_id: 'k-final',
        score: 72,
        reason: 'Final kâğıdında toplama hatası',
      }),
    )
    const card = await screen.findByRole('article', { name: 'COM1001 · Ece Arslan' })
    expect(within(card).getByText('Onay bekliyor')).toBeInTheDocument()
    expect(within(card).getByRole('button', { name: 'Geri çek' })).toBeInTheDocument()
  })

  it('bölüm başkanı talebi onaylar; ret gerekçe ister', async () => {
    let approved: unknown = null
    let rejected: unknown = null
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
      http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('grade_change:approve'))),
      http.get('/api/v1/grade-change-requests', ({ request }) => {
        const status = new URL(request.url).searchParams.get('status')
        return HttpResponse.json({
          items:
            status === 'PENDING'
              ? [
                  change({ can_approve: true, can_withdraw: false }),
                  change({
                    id: 'g2',
                    kind: 'LETTER',
                    component: null,
                    can_approve: true,
                    can_withdraw: false,
                  }),
                ]
              : [],
        })
      }),
      http.post('/api/v1/grade-change-requests/:id/approve', async ({ request }) => {
        approved = await request.json()
        return HttpResponse.json(change({ status: 'APPLIED' }))
      }),
      http.post('/api/v1/grade-change-requests/:id/reject', async ({ request, params }) => {
        rejected = { id: params.id, ...((await request.json()) as object) }
        return HttpResponse.json(change({ status: 'REJECTED' }))
      }),
    )
    const { user } = renderApp('/not-duzeltme-onaylari')

    expect(await screen.findByRole('heading', { name: 'Onay bekleyen (2)' })).toBeInTheDocument()
    const [first, second] = screen.getAllByRole('article')
    expect(within(first!).getByText('Final sınavı')).toBeInTheDocument()
    expect(within(first!).getByText('C2')).toBeInTheDocument()

    await user.click(within(first!).getByRole('button', { name: 'Onayla ve uygula' }))
    let dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Onayla ve uygula' }))
    await waitFor(() => expect(approved).toEqual({}))

    await user.click(within(second!).getByRole('button', { name: 'Reddet' }))
    dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Reddet' }))
    expect(rejected).toBeNull()
    await user.type(within(dialog).getByRole('textbox'), 'Kanıt yok')
    await user.click(within(dialog).getByRole('button', { name: 'Reddet' }))
    await waitFor(() => expect(rejected).toEqual({ id: 'g2', note: 'Kanıt yok' }))
  })
})
