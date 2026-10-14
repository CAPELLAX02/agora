import { screen, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { CalendarEvent, CalendarEventType } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const type = (code: string, name: string): CalendarEventType => ({
  code,
  name_tr: name,
  name_en: name,
  category: 'REGISTRATION',
  is_action_window: true,
})

const event = (t: CalendarEventType, starts: string, ends: string): CalendarEvent => ({
  id: t.code,
  term_id: 't',
  type: t,
  title_tr: null,
  title_en: null,
  starts_at: starts,
  ends_at: ends,
  scope_type: 'FACULTY',
  scope: { id: 'f-muh', name: 'Mühendislik Fakültesi' },
  is_published: true,
  note: null,
  version: 1,
})

describe('ana sayfa', () => {
  it('öğrencinin programına göre açık ve yaklaşan işlem pencerelerini gösterir', async () => {
    let programQuery = ''
    const addDrop = type('ADD_DROP_STUDENT', 'Ekle-bırak (öğrenci)')
    const midterm = type('MIDTERM_GRADE_ENTRY', 'Ara sınav not girişleri')
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
      http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('calendar:read'))),
      http.get('/api/v1/me/programs', () =>
        HttpResponse.json({
          items: [
            { id: 'sp', program: { id: 'p-bil', code: 'BIL-EN-NO', name_tr: 'Bilgisayar Mühendisliği' } },
          ],
        }),
      ),
      http.get('/api/v1/calendar/windows', ({ request }) => {
        programQuery = new URL(request.url).searchParams.get('program_id') ?? ''
        return HttpResponse.json({
          term: { id: 't', code: '2026-FALL' },
          at: '2026-10-09T09:00:00Z',
          items: [
            {
              type: addDrop,
              scope_type: 'FACULTY',
              open: true,
              current: event(addDrop, '2026-10-05T07:00:00Z', '2026-10-16T21:00:00Z'),
              next: null,
              events: [],
            },
            {
              type: midterm,
              scope_type: 'UNIVERSITY',
              open: false,
              current: null,
              next: event(midterm, '2026-11-09T05:00:00Z', '2026-12-04T21:00:00Z'),
              events: [],
            },
          ],
        })
      }),
    )
    renderApp('/')

    const open = await screen.findByRole('list', { name: 'Açık pencereler' })
    expect(within(open).getByText('Ekle-bırak (öğrenci)')).toBeInTheDocument()
    expect(
      within(screen.getByRole('list', { name: 'Yaklaşan pencereler' })).getByText('Ara sınav not girişleri'),
    ).toBeInTheDocument()
    expect(screen.getByText('2026-FALL dönemi')).toBeInTheDocument()
    expect(programQuery).toBe('p-bil')
  })

  it('öğrenci bu dönem son ilan edilen notlarını görür: harf notu ve sınıf ortalamalı bileşen notu', async () => {
    const component = (id: string, label: string, published: string | null, score: number | null) => ({
      id,
      type_code: 'MIDTERM',
      category: 'IN_TERM',
      label_tr: label,
      label_en: label,
      weight: 40,
      scheduled_on: null,
      published_at: published,
      score,
      absent: false,
      class_average: published ? 61.1 : null,
    })
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
      http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('grade:read_own'))),
      http.get('/api/v1/me/programs', () =>
        HttpResponse.json({ items: [{ id: 'sp', program: { id: 'p', code: 'BIL-EN-NO', name_tr: 'BM' } }] }),
      ),
      http.get('/api/v1/student-programs/:id/grades', () =>
        HttpResponse.json({
          student_program_id: 'sp',
          terms: [],
          term: { id: 't', code: '2026-FALL' },
          courses: [
            {
              enrollment_id: 'e1',
              course: { id: 'c1', code: 'COM1007', name_tr: 'Bilgisayar Programlama I', name_en: 'P1' },
              section_code: '1',
              ects: 6,
              status: 'ENROLLED',
              attendance_status: 'CONTINUING',
              letter_grade: null,
              is_passing: null,
              weighted_score: null,
              grade_published_at: null,
              components: [
                component('k1', 'Ara sınav', '2026-10-14T08:00:00Z', 38),
                component('k2', 'Laboratuvar', null, null),
              ],
              class_stats: null,
            },
            {
              enrollment_id: 'e2',
              course: { id: 'c2', code: 'MTH0143', name_tr: 'Matematik I', name_en: 'Math I' },
              section_code: '1',
              ects: 6,
              status: 'COMPLETED',
              attendance_status: 'CONTINUING',
              letter_grade: 'B2',
              is_passing: true,
              weighted_score: 78,
              grade_published_at: '2026-10-13T08:00:00Z',
              components: [],
              class_stats: null,
            },
          ],
        }),
      ),
    )
    renderApp('/')
    const card = (await screen.findByText('Son notlarım')).closest('[data-slot="card"]') as HTMLElement
    const rows = await within(card).findAllByRole('listitem')
    expect(rows).toHaveLength(2)
    expect(within(rows[0]!).getByText('Bilgisayar Programlama I')).toBeInTheDocument()
    expect(within(rows[0]!).getByText('38')).toBeInTheDocument()
    expect(within(rows[0]!).getByText('sınıf ort. 61,1')).toBeInTheDocument()
    expect(within(rows[1]!).getByText('B2')).toBeInTheDocument()
    expect(within(card).queryByText('Laboratuvar', { exact: false })).not.toBeInTheDocument()
  })

  it('öğretim elemanı not girişi bekleyen şubelerini ve eksik not sayısını görür', async () => {
    const window = {
      open: true,
      starts_at: '2026-10-14T07:00:00Z',
      ends_at: '2026-11-04T07:00:00Z',
      next_starts_at: null,
    }
    const closed = { open: false, starts_at: null, ends_at: null, next_starts_at: '2026-12-28T05:00:00Z' }
    type Window = {
      open: boolean
      starts_at: string | null
      ends_at: string | null
      next_starts_at: string | null
    }
    const comp = (
      id: string,
      label: string,
      category: string,
      entered: number,
      entryWindow: Window = window,
    ) => ({
      id,
      type: { code: id, name_tr: label, name_en: label, category },
      sequence_no: 1,
      label_tr: label,
      label_en: label,
      weight: 30,
      version: 1,
      published_at: null,
      entry_window: entryWindow,
      editable: false,
      scored_count: entered,
      absent_count: 0,
      average: null,
    })
    const book = (id: string, components: ReturnType<typeof comp>[]) => ({
      section: {
        id,
        section_code: '1',
        course: { id: 'c', code: 'X', name_tr: 'X', name_en: 'X', grading_mode: 'LETTER' },
        term: { id: 't', code: '2026-FALL' },
        plan_locked: true,
        instructor_role: 'PRIMARY',
        grades_published_at: null,
        makeup_published_at: null,
      },
      components,
      students: Array.from({ length: 20 }, (_, i) => ({
        enrollment_id: `e${i}`,
        makeup_eligible: false,
        scores: {},
      })),
      can_enter: false,
      mfa_required: true,
    })
    const entry = (section: string, code: string, name: string) => ({
      section_id: section,
      section_code: '1',
      course: { id: code, code, name_tr: name, name_en: name },
      instructors: [],
      day_of_week: 1,
      start_time: '09:00',
      end_time: '10:50',
    })
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
      http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('assessment_plan:manage'))),
      http.get('/api/v1/terms/current', () => HttpResponse.json({ id: 't', code: '2026-FALL' })),
      http.get('/api/v1/me/teaching', () =>
        HttpResponse.json({
          items: [
            entry('s1', 'COM1007', 'Bilgisayar Programlama I'),
            entry('s1', 'COM1007', 'Bilgisayar Programlama I'),
            entry('s2', 'COM3067', 'Algoritmalar'),
          ],
        }),
      ),
      http.get('/api/v1/sections/:id/gradebook', ({ params }) =>
        HttpResponse.json(
          params.id === 's1'
            ? book('s1', [
                comp('MIDTERM', 'Ara sınav', 'IN_TERM', 20),
                comp('FINAL', 'Final sınavı', 'FINAL', 3),
                comp('MAKEUP', 'Bütünleme', 'MAKEUP', 0),
              ])
            : book('s2', [comp('MIDTERM', 'Ara sınav', 'IN_TERM', 0, closed)]),
        ),
      ),
    )
    renderApp('/')
    const s1 = await screen.findByRole('link', { name: /COM1007-1/ })
    expect(await within(s1).findByText(/Final sınavı: 17 eksik/)).toBeInTheDocument()
    expect(within(s1).getByText('MFA gerekli')).toBeInTheDocument()
    expect(s1).toHaveAttribute('href', '/subeler/s1/notlar')
    const s2 = screen.getByRole('link', { name: /COM3067-1/ })
    expect(await within(s2).findByText('Bekleyen not girişi yok')).toBeInTheDocument()
  })
})
