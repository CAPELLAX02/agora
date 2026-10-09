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
})
