import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Hold, Student, StudentProgram } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const program = (over: Partial<StudentProgram> = {}): StudentProgram => ({
  id: 'sp-1',
  student: { id: 'st-1', student_no: '24290014', first_name: 'Onur', last_name: 'Kılıç' },
  program: {
    id: 'p-1',
    code: 'BIL-EN-NO',
    name_tr: 'Bilgisayar Mühendisliği (İngilizce)',
    department: { id: 'd', name_tr: 'Bilgisayar Mühendisliği' },
    faculty: { id: 'f', name_tr: 'Mühendislik Fakültesi' },
  },
  kind: 'MAJOR',
  admission_type: 'OSYS',
  admission_year: 2024,
  admitted_on: '2024-09-19',
  status: 'ACTIVE',
  class_level: 3,
  current_semester: 5,
  normal_semesters: 8,
  max_semesters: 14,
  gpa: 2.84,
  earned_ects: 118,
  advisor: null,
  curriculum: { id: 'c', name_tr: '2023 Ders Planı', name_en: '2023 Curriculum' },
  version: 2,
  ...over,
})

const hold: Hold = {
  id: 'h-1',
  student_program_id: 'sp-1',
  student: { id: 'st-1', student_no: '24290014', first_name: 'Onur', last_name: 'Kılıç' },
  program: { id: 'p-1', code: 'BIL-EN-NO', name_tr: 'Bilgisayar Mühendisliği (İngilizce)' },
  hold_type: 'TUITION_DEBT',
  blocks: ['REGISTRATION', 'DOCUMENT'],
  reason: '2026 güz katkı payı ödenmedi',
  placed_by: 'Fatma Çelik',
  placed_at: '2026-10-11T09:00:00Z',
  released_at: null,
  released_by: null,
  release_reason: null,
  active: true,
}

const registrar = [
  http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
  http.get('/api/v1/me/permissions', () =>
    HttpResponse.json(grants('person:read', 'hold:manage', 'student_program:manage')),
  ),
]

describe('kayıt engelleri', () => {
  it('aktif engeller listelenir ve gerekçeyle kaldırılır', async () => {
    let reason = ''
    server.use(
      ...registrar,
      http.get('/api/v1/holds', ({ request }) =>
        HttpResponse.json({
          items: new URL(request.url).searchParams.get('active') === 'false' ? [] : [hold],
          next_cursor: null,
        }),
      ),
      http.post('/api/v1/holds/:id/release', async ({ request }) => {
        reason = ((await request.json()) as { reason: string }).reason
        return HttpResponse.json({ ...hold, active: false })
      }),
    )
    const { user } = renderApp('/kayit-engelleri')
    const row = (await screen.findByRole('link', { name: 'Onur Kılıç' })).closest('tr')!
    expect(within(row).getByText('Katkı payı borcu')).toBeInTheDocument()
    await user.click(within(row).getByRole('button', { name: 'Engeli kaldır' }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Gerekçe'), 'Dekont geldi')
    await user.click(within(dialog).getByRole('button', { name: 'Engeli kaldır' }))
    await waitFor(() => expect(reason).toBe('Dekont geldi'))
  })
})

describe('öğrenci detayı', () => {
  it('program durumu gösterilir; durum gerekçe ve If-Match ile değişir, engel konur', async () => {
    const student: Student = {
      id: 'st-1',
      student_no: '24290014',
      first_name: 'Onur',
      last_name: 'Kılıç',
      email: 'onur.kilic@ogrenci.agora.test',
      programs: [program()],
    }
    let change: { ifMatch: string; body: unknown } | null = null
    let placed: unknown = null
    server.use(
      ...registrar,
      http.get('/api/v1/students/:id', () => HttpResponse.json(student)),
      http.get('/api/v1/student-programs/:id/holds', () => HttpResponse.json({ items: [hold] })),
      http.post('/api/v1/student-programs/:id/status', async ({ request }) => {
        change = { ifMatch: request.headers.get('If-Match') ?? '', body: await request.json() }
        return HttpResponse.json(program({ status: 'FROZEN', version: 3 }))
      }),
      http.post('/api/v1/student-programs/:id/holds', async ({ request }) => {
        placed = await request.json()
        return HttpResponse.json(hold, { status: 201 })
      }),
    )
    const { user } = renderApp('/ogrenciler/st-1')
    expect(await screen.findByRole('heading', { name: 'Onur Kılıç' })).toBeInTheDocument()
    expect(screen.getByText('5. yarıyıl (normal 8, azami 14)')).toBeInTheDocument()
    expect(await screen.findByText('2026 güz katkı payı ödenmedi')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Durum değiştir' }))
    let dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Karar numarası (isteğe bağlı)'), 'YK 2026/41')
    await user.type(within(dialog).getByLabelText('Gerekçe'), 'Sağlık raporu')
    await user.click(within(dialog).getByRole('button', { name: 'Kaydet' }))
    await waitFor(() =>
      expect(change).toEqual({
        ifMatch: '"2"',
        body: { status: 'FROZEN', reason: 'Sağlık raporu', decision_ref: 'YK 2026/41', graduated_on: null },
      }),
    )

    await user.click(screen.getByRole('button', { name: 'Engel koy' }))
    dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Gerekçe'), 'Kütüphane kitabı')
    await user.click(within(dialog).getByRole('button', { name: 'Engel koy' }))
    await waitFor(() =>
      expect(placed).toEqual({
        hold_type: 'TUITION_DEBT',
        blocks: ['REGISTRATION', 'DOCUMENT'],
        reason: 'Kütüphane kitabı',
      }),
    )
  })
})
