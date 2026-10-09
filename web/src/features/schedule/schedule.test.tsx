import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { ScheduleEntry, Term } from '@/shared/api/generated'
import { grants, me, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

import { placeEntries } from './layout'

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

const entry = (over: Partial<ScheduleEntry> & { id: string }): ScheduleEntry => ({
  day_of_week: 1,
  start_time: '09:00',
  end_time: '11:50',
  session_type: 'THEORY',
  classroom: { id: 'r-a105', code: 'A105', name: 'Derslik 105', building_code: 'MUH-A', capacity: 80 },
  term_id: term.id,
  offering_id: 'o-1',
  section_id: 's-1',
  section_code: '1',
  course: { id: 'c-2039', code: 'COM2039', name_tr: 'Ayrık Yapılar', name_en: 'Discrete Structures' },
  instructors: [
    {
      staff_id: 'st-5',
      staff_no: 'P10005',
      title: 'Doç. Dr.',
      first_name: 'Selin',
      last_name: 'Koç',
      role: 'PRIMARY',
    },
  ],
  ...over,
})

const entries = [
  entry({ id: 'a' }),
  entry({
    id: 'b',
    start_time: '10:00',
    end_time: '10:50',
    section_id: 's-2',
    course: {
      id: 'c-2501',
      code: 'COM2501',
      name_tr: 'Bilimsel Programlama',
      name_en: 'Scientific Programming',
    },
  }),
  entry({
    id: 'c',
    day_of_week: 3,
    start_time: '13:00',
    end_time: '14:50',
    session_type: 'LAB',
    classroom: null,
  }),
]

describe('haftalık program yerleşimi', () => {
  it('çakışan oturumlar yan yana şeritlere, bitişikler aynı şeride yerleşir', () => {
    const placed = placeEntries([
      ...entries,
      entry({ id: 'd', start_time: '11:50', end_time: '12:40' }), // a bitince başlar
    ])
    const of = (id: string) => placed.find((p) => p.entry.id === id)
    expect(of('a')).toMatchObject({ lane: 0, lanes: 2 })
    expect(of('b')).toMatchObject({ lane: 1, lanes: 2 })
    expect(of('d')).toMatchObject({ lane: 0, lanes: 1 })
    expect(of('c')).toMatchObject({ lane: 0, lanes: 1 })
  })
})

const session = (...perms: string[]) => [
  http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
  http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('course:read', ...perms))),
  http.get('/api/v1/terms', () => HttpResponse.json({ items: [term] })),
  http.get('/api/v1/terms/current', () => HttpResponse.json(term)),
]

describe('haftalık program', () => {
  it('bölüm programı gün ve saate göre çizilir; derslik görünümüne geçilir', async () => {
    const filters: Record<string, string>[] = []
    server.use(
      ...session(),
      http.get('/api/v1/faculties', () => HttpResponse.json({ items: [] })),
      http.get('/api/v1/faculties/:id/departments', () => HttpResponse.json({ items: [] })),
      http.get('/api/v1/buildings', () =>
        HttpResponse.json({ items: [{ id: 'b-muha', code: 'MUH-A', name: 'Mühendislik A Blok' }] }),
      ),
      http.get('/api/v1/classrooms', () =>
        HttpResponse.json({ items: [{ id: 'r-a105', code: 'A105', name: 'Derslik 105', capacity: 80 }] }),
      ),
      http.get('/api/v1/terms/:id/schedule', ({ request }) => {
        filters.push(Object.fromEntries(new URL(request.url).searchParams))
        return HttpResponse.json({ items: entries })
      }),
    )
    const { user } = renderApp('/program?birim=f-muh&bolum=d-bil')

    // Bölüm görünümü varsayılan olarak gün gün listedir.
    const mondayList = await screen.findByRole('region', { name: 'Pazartesi' })
    expect(
      within(mondayList).getByRole('row', {
        name: /09:00–11:50 COM2039-1 Ayrık Yapılar MUH-A A105 Doç. Dr. Selin Koç/,
      }),
    ).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Izgara' }))
    const monday = await screen.findByRole('cell', { name: 'Pazartesi' })
    const block = within(monday).getByRole('article', {
      name: /COM2039-1 Ayrık Yapılar, Pazartesi 09:00–11:50, MUH-A A105/,
    })
    expect(block).toHaveTextContent('Doç. Dr. Selin Koç')
    expect(within(monday).getByRole('article', { name: /COM2501-1/ })).toBeInTheDocument()
    const wednesday = screen.getByRole('cell', { name: 'Çarşamba' })
    expect(within(wednesday).getByRole('article', { name: /Çevrim içi/ })).toHaveTextContent('Laboratuvar')
    expect(filters.at(-1)).toEqual({ department_id: 'd-bil' })
    // Öğretim elemanı görünümü şube yönetimi yetkisi ister.
    expect(screen.queryByRole('tab', { name: 'Öğretim elemanı' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Derslik' }))
    expect(await screen.findByText('Doluluğunu görmek için bir derslik seçin.')).toBeInTheDocument()
    await user.click(screen.getByRole('combobox', { name: 'Bina' }))
    await user.click(await screen.findByRole('option', { name: 'MUH-A · Mühendislik A Blok' }))
    await user.click(screen.getByRole('combobox', { name: 'Derslik' }))
    await user.click(await screen.findByRole('option', { name: /A105 · Derslik 105/ }))
    await waitFor(() => expect(filters.at(-1)).toEqual({ classroom_id: 'r-a105' }))
  })

  it('öğretim elemanı şubelerini ve kendi rolünü görür', async () => {
    server.use(
      ...session('assessment_plan:manage'),
      http.get('/api/v1/me', () => HttpResponse.json(me({ username: 'P10005' }))),
      http.get('/api/v1/me/teaching', () => HttpResponse.json({ items: entries })),
    )
    renderApp('/verdigim-dersler')

    // İki şube listelenir (blokların yanında); kullanıcının her ikisinde de rolü sorumluluk.
    expect(await screen.findAllByText('Sorumlu')).toHaveLength(2)
    expect(screen.getByText(/Pzt 09:00–11:50 MUH-A A105/)).toBeInTheDocument()
  })
})
