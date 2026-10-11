import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type {
  AddDropRequest,
  Registration,
  RegistrationOffer,
  RegistrationSection,
  RegistrationSummary,
  StudentProgram,
} from '@/shared/api/generated'
import { grants, problem, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const section = (code: string, over: Partial<RegistrationSection> = {}): RegistrationSection => ({
  id: `s-${code}`,
  offering_id: `o-${code}`,
  course: { id: `c-${code}`, code, name_tr: `${code} adı`, name_en: `${code} name`, ects: 5, credit: 3 },
  section_code: '1',
  label: `${code}-1`,
  capacity: 40,
  enrolled: 10,
  available: true,
  quota_mode: 'OPEN',
  quota_left: null,
  language: 'EN',
  instructors: ['Doç. Dr. Selin Koç'],
  slots: [{ day: 2, start: '09:00', end: '11:50', kind: 'THEORY', classroom: 'MUH-A/A104' }],
  ...over,
})

const registration = (over: Partial<Registration> = {}): Registration => ({
  id: 'r-1',
  student_program_id: 'sp-1',
  student: {
    id: 'st-1',
    student_no: '22290002',
    first_name: 'Ece',
    last_name: 'Kaya',
    program_code: 'BIL-EN-NO',
    semester: 7,
    class_level: 4,
    gpa: 2.68,
    advisor: 'Prof. Dr. Ayşe Demir',
  },
  term: { id: 't-1', code: '2026-FALL' },
  status: 'DRAFT',
  items: [],
  total_ects: 0,
  ects_limit: 40,
  max_ects_allowed: null,
  gpa_snapshot: null,
  submitted_at: null,
  decided_at: null,
  decided_by: null,
  decision_note: null,
  extension_until: null,
  extension_reason: null,
  evaluation: { blocking: true, violations: [] },
  registration_window: {
    open: true,
    extended: false,
    starts_at: '2026-10-01T07:00:00Z',
    ends_at: '2026-11-01T07:00:00Z',
    next_starts_at: null,
  },
  approval_window: { open: true, extended: false, starts_at: null, ends_at: null, next_starts_at: null },
  actions: {
    can_edit: true,
    can_submit: false,
    can_withdraw: false,
    can_approve: false,
    can_reject: false,
    can_revoke: false,
    can_extend: false,
  },
  warnings: [],
  version: 3,
  ...over,
})

const offer: RegistrationOffer = {
  term: { id: 't-1', code: '2026-FALL' },
  semester: 7,
  ects_limit: 40,
  total_ects: 0,
  items: [
    {
      course: {
        id: 'c-PHY0105',
        code: 'PHY0105',
        name_tr: 'Fizik I',
        name_en: 'Physics I',
        ects: 4,
        credit: 3,
      },
      category: 'REPEAT',
      kind: 'REPEAT_FAILED',
      line: {
        item_id: 'l-1',
        semester_no: 1,
        item_type: 'COURSE',
        code: 'PHY0101',
        name_tr: 'Fizik I',
        name_en: 'Physics I',
        is_compulsory: true,
        remaining: 0,
      },
      status: 'FAILED',
      last_letter: 'F2',
      prerequisite: null,
      sections: [
        section('PHY0105', {
          course: {
            id: 'c-PHY0105',
            code: 'PHY0105',
            name_tr: 'Fizik I',
            name_en: 'Physics I',
            ects: 4,
            credit: 3,
          },
        }),
      ],
      basket_item_id: null,
    },
    {
      course: {
        id: 'c-COM4519',
        code: 'COM4519',
        name_tr: 'Veri Madenciliği',
        name_en: 'Data Mining',
        ects: 6,
        credit: 3,
      },
      category: 'ELECTIVE',
      kind: 'NEW',
      line: {
        item_id: 'l-2',
        semester_no: 7,
        item_type: 'ELECTIVE_SLOT',
        code: 'COMTE04',
        name_tr: 'Teknik seçmeli',
        name_en: 'Technical elective',
        is_compulsory: false,
        remaining: 2,
      },
      status: 'NOT_TAKEN',
      last_letter: null,
      prerequisite: null,
      sections: [section('COM4519', { available: false, enrolled: 40 })],
      basket_item_id: null,
    },
    {
      course: {
        id: 'c-COM3067',
        code: 'COM3067',
        name_tr: 'Algoritmalar',
        name_en: 'Algorithms',
        ects: 5,
        credit: 3,
      },
      category: 'UPGRADE',
      kind: 'REPEAT_UPGRADE',
      line: {
        item_id: 'l-3',
        semester_no: 5,
        item_type: 'COURSE',
        code: 'COM3067',
        name_tr: 'Algoritmalar',
        name_en: 'Algorithms',
        is_compulsory: true,
        remaining: 0,
      },
      status: 'PASSED',
      last_letter: 'C3',
      prerequisite: null,
      sections: [section('COM3067')],
      basket_item_id: null,
    },
  ],
}

const program = {
  id: 'sp-1',
  program: {
    id: 'p-1',
    code: 'BIL-EN-NO',
    name_tr: 'Bilgisayar Mühendisliği',
    department: { id: 'd', name_tr: 'BİL' },
    faculty: { id: 'f', name_tr: 'Mühendislik Fakültesi' },
  },
} as unknown as StudentProgram

const student = (...perms: string[]) => [
  http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
  http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('registration:manage_own', ...perms))),
  http.get('/api/v1/me/programs', () => HttpResponse.json({ items: [program] })),
  http.get('/api/v1/student-programs/:id/registration/offer', () => HttpResponse.json(offer)),
]

describe('ders seçimi', () => {
  it('alınabilir dersleri kategorilerle gösterir; ders eklenir, kurallar ve AKTS güncellenir', async () => {
    let current = registration()
    let added: { key: string | null; body: unknown } | null = null
    server.use(
      ...student(),
      http.get('/api/v1/student-programs/:id/registration', () => HttpResponse.json(current)),
      http.post('/api/v1/student-programs/:id/registration/items', async ({ request }) => {
        added = { key: request.headers.get('Idempotency-Key'), body: await request.json() }
        current = registration({
          items: [
            {
              id: 'i-1',
              kind: 'REPEAT_FAILED',
              curriculum_item_id: 'l-1',
              ects: 4,
              source: 'REGISTRATION',
              added_at: '2026-10-11T10:00:00Z',
              section: offer.items[0]!.sections[0]!,
            },
          ],
          total_ects: 4,
          evaluation: {
            blocking: true,
            violations: [
              {
                code: 'REPEAT_PRIORITY',
                severity: 'ERROR',
                course_ids: ['c-COM3035'],
                params: { courses: ['COM3035'] },
                message:
                  'Alt yarıyıllardan kaldığınız ya da almadığınız dersleri öncelikle almalısınız: COM3035.',
              },
            ],
          },
        })
        return HttpResponse.json(current, { status: 201 })
      }),
    )
    const { user } = renderApp('/ders-secimi')

    expect(await screen.findByRole('heading', { name: '2026-FALL dönemi ders kaydı' })).toBeInTheDocument()
    const repeat = await screen.findByRole('region', { name: 'Alttan dersler' })
    expect(within(repeat).getByText('PHY0101 yerine sayılır', { exact: false })).toBeInTheDocument()
    expect(within(repeat).getByText('Tekrar')).toBeInTheDocument()
    // Dolu şubenin düğmesi kapalı; not yükseltme kapalı başlar.
    expect(screen.getByRole('button', { name: 'COM4519-1: Sepete ekle' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'COM3067-1: Sepete ekle' })).not.toBeInTheDocument()

    await user.click(within(repeat).getByRole('button', { name: 'PHY0105-1: Sepete ekle' }))
    await waitFor(() => expect(added).not.toBeNull())
    expect(added!.body).toEqual({ section_id: 's-PHY0105' })
    expect(added!.key).toMatch(/^[0-9a-f-]{36}$/)

    const basket = await screen.findByRole('region', { name: 'Sepetim' })
    expect(await within(basket).findByText('PHY0105-1')).toBeInTheDocument()
    expect(within(basket).getByText(/öncelikle almalısınız: COM3035/)).toBeInTheDocument()
    expect(within(basket).getByRole('button', { name: 'Danışmana gönder' })).toBeDisabled()
    expect(within(basket).getByRole('meter', { name: 'AKTS' })).toHaveAttribute('aria-valuenow', '4')
  })

  it('kural ihlalinde eklemeyi reddeder ve nedenini gösterir', async () => {
    server.use(
      ...student(),
      http.get('/api/v1/student-programs/:id/registration', () => HttpResponse.json(registration())),
      http.post('/api/v1/student-programs/:id/registration/items', () =>
        HttpResponse.json(
          problem(422, 'RULE_VIOLATION', {
            violations: [
              {
                code: 'PREREQUISITE',
                severity: 'ERROR',
                course_ids: [],
                params: {},
                message: 'PHY0105 için ön koşul sağlanmıyor: MTH0141.',
              },
            ],
          }),
          { status: 422 },
        ),
      ),
    )
    const { user } = renderApp('/ders-secimi')
    await user.click(await screen.findByRole('button', { name: 'PHY0105-1: Sepete ekle' }))
    expect(await screen.findByText('PHY0105 için ön koşul sağlanmıyor: MTH0141.')).toBeInTheDocument()
  })

  it('sepet gönderilir (If-Match) ve gönderim geri çekilebilir', async () => {
    const item = {
      id: 'i-1',
      kind: 'NEW' as const,
      curriculum_item_id: null,
      ects: 4,
      source: 'REGISTRATION' as const,
      added_at: '2026-10-11T10:00:00Z',
      section: offer.items[0]!.sections[0]!,
    }
    let current = registration({
      items: [item],
      total_ects: 4,
      evaluation: { blocking: false, violations: [] },
      actions: { ...registration().actions, can_submit: true },
    })
    let ifMatch = ''
    server.use(
      ...student(),
      http.get('/api/v1/student-programs/:id/registration', () => HttpResponse.json(current)),
      http.post('/api/v1/student-programs/:id/registration/submit', ({ request }) => {
        ifMatch = request.headers.get('If-Match') ?? ''
        current = registration({
          items: [item],
          total_ects: 4,
          status: 'SUBMITTED',
          submitted_at: '2026-10-11T10:05:00Z',
          evaluation: { blocking: false, violations: [] },
          actions: { ...registration().actions, can_edit: false, can_withdraw: true },
          version: 4,
        })
        return HttpResponse.json(current)
      }),
    )
    const { user } = renderApp('/ders-secimi')
    await user.click(await screen.findByRole('button', { name: 'Danışmana gönder' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('1 ders (4 AKTS)')
    await user.click(within(dialog).getByRole('button', { name: 'Danışmana gönder' }))
    await waitFor(() => expect(ifMatch).toBe('"3"'))
    expect(await screen.findByRole('button', { name: 'Gönderimi geri çek' })).toBeInTheDocument()
    expect(screen.getByText(/Kaydınız danışman onayında/)).toBeInTheDocument()
  })

  it('onaylı kayıtta ekle-bırak talepleri ve onay belgesi', async () => {
    const item = {
      id: 'i-1',
      kind: 'NEW' as const,
      curriculum_item_id: null,
      ects: 4,
      source: 'REGISTRATION' as const,
      added_at: '2026-10-11T10:00:00Z',
      section: offer.items[0]!.sections[0]!,
    }
    const request: AddDropRequest = {
      id: 'ad-1',
      registration_id: 'r-1',
      student_program_id: 'sp-1',
      student: {
        id: 'st-1',
        student_no: '22290002',
        first_name: 'Ece',
        last_name: 'Kaya',
        program_code: 'BIL-EN-NO',
      },
      term_id: 't-1',
      status: 'PENDING_DEPT_HEAD',
      stage: 'DEPT_HEAD',
      student_note: 'Seçmeliyi değiştiriyorum',
      advisor: { by: 'Ayşe Demir', at: '2026-10-11T11:00:00Z', note: null },
      dept_head: null,
      items: [{ id: 'x', action: 'DROP', ects: 4, section: item.section }],
      ects_delta: -4,
      actions: { can_withdraw: true, can_approve: false, can_reject: false },
      version: 2,
      created_at: '2026-10-11T10:30:00Z',
    }
    server.use(
      ...student(),
      http.get('/api/v1/student-programs/:id/registration', () =>
        HttpResponse.json(
          registration({
            status: 'APPROVED',
            items: [item],
            total_ects: 4,
            decided_by: 'Ayşe Demir',
            decided_at: '2026-10-10T09:00:00Z',
            evaluation: { blocking: false, violations: [] },
            actions: { ...registration().actions, can_edit: false },
          }),
        ),
      ),
      http.get('/api/v1/student-programs/:id/registration/add-drop', () =>
        HttpResponse.json({ items: [request] }),
      ),
      http.get('/api/v1/calendar/windows', () =>
        HttpResponse.json({
          term: { id: 't-1', code: '2026-FALL' },
          at: '2026-10-11T12:00:00Z',
          items: [
            {
              type: {
                code: 'ADD_DROP_STUDENT',
                name_tr: 'Ekle-bırak',
                name_en: 'Add-drop',
                category: 'REGISTRATION',
                is_action_window: true,
                sort_order: 24,
              },
              scope_type: 'PROGRAM',
              open: true,
              current: { id: 'e', ends_at: '2026-11-01T07:00:00Z' },
              next: null,
              events: [],
            },
          ],
        }),
      ),
    )
    renderApp('/ders-secimi')
    expect(await screen.findByText(/Ayşe Demir kaydınızı onayladı/)).toBeInTheDocument()
    const requests = await screen.findByRole('list', { name: 'Ekle-bırak talepleri' })
    expect(within(requests).getByText('Bölüm başkanı onayında')).toBeInTheDocument()
    expect(within(requests).getByRole('button', { name: 'Geri çek' })).toBeInTheDocument()
    // Açık talep varken yeni talep açılmaz.
    expect(screen.queryByRole('button', { name: 'Yeni ekle-bırak talebi' })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Kayıt onay belgesi' })).toHaveAttribute(
      'href',
      '/ders-secimi/belge?program=sp-1',
    )
  })
})

const summary = (
  no: string,
  status: RegistrationSummary['status'],
  over: Partial<RegistrationSummary> = {},
): RegistrationSummary => ({
  student_program_id: `sp-${no}`,
  student: { id: `st-${no}`, student_no: no, first_name: 'Öğrenci', last_name: no },
  program_code: 'BIL-EN-NO',
  semester: 7,
  gpa: 3.1,
  registration_id: status === 'NOT_STARTED' ? null : `r-${no}`,
  status,
  total_ects: 30,
  item_count: 5,
  submitted_at: status === 'SUBMITTED' ? '2026-10-11T09:00:00Z' : null,
  version: 4,
  blocked: false,
  ...over,
})

describe('danışmanlık', () => {
  const advisor = [
    http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
    http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('registration:approve'))),
    http.get('/api/v1/me/add-drop-queue', () => HttpResponse.json({ items: [] })),
  ]

  it('onay bekleyen kayıtlar seçilip toplu onaylanır; başarısızlar bildirilir', async () => {
    let body: unknown = null
    server.use(
      ...advisor,
      http.get('/api/v1/me/advisee-registrations', () =>
        HttpResponse.json({
          term: { id: 't', code: '2026-FALL' },
          next_cursor: null,
          items: [
            summary('23290013', 'SUBMITTED'),
            summary('25290016', 'SUBMITTED', { version: 7 }),
            summary('24290014', 'SUBMITTED', { blocked: true }),
            summary('26290004', 'NOT_STARTED', { total_ects: 0, item_count: 0 }),
          ],
        }),
      ),
      http.post('/api/v1/registrations/bulk-approve', async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({
          approved: 1,
          failed: 1,
          results: [
            { id: 'r-23290013', ok: true, code: null, message: null },
            {
              id: 'r-25290016',
              ok: false,
              code: 'VERSION_MISMATCH',
              message: 'Kayıt siz incelerken değişmiş.',
            },
          ],
        })
      }),
    )
    const { user } = renderApp('/danismanlik')
    expect(await screen.findByRole('tab', { name: 'Onay bekliyor (3)' })).toBeInTheDocument()
    // Engelli öğrenci seçilemez.
    expect(screen.queryByRole('checkbox', { name: 'Öğrenci 24290014 kaydını seç' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('checkbox', { name: 'Öğrenci 23290013 kaydını seç' }))
    await user.click(screen.getByRole('checkbox', { name: 'Öğrenci 25290016 kaydını seç' }))
    await user.click(screen.getByRole('button', { name: 'Seçilenleri onayla (2)' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Onayla' }))
    await waitFor(() =>
      expect(body).toEqual({
        items: [
          { id: 'r-23290013', version: 4 },
          { id: 'r-25290016', version: 7 },
        ],
      }),
    )
    expect(await screen.findByText('1 kayıt onaylandı, 1 kayıt onaylanamadı.')).toBeInTheDocument()
  })

  it('kayıt incelenir; geri göndermede gerekçe zorunludur', async () => {
    let note = ''
    server.use(
      ...advisor,
      http.get('/api/v1/registrations/:id', () =>
        HttpResponse.json(
          registration({
            status: 'SUBMITTED',
            items: [
              {
                id: 'i-1',
                kind: 'NEW',
                curriculum_item_id: null,
                ects: 4,
                source: 'REGISTRATION',
                added_at: '2026-10-11T10:00:00Z',
                section: offer.items[0]!.sections[0]!,
              },
            ],
            total_ects: 4,
            evaluation: { blocking: false, violations: [] },
            actions: { ...registration().actions, can_edit: false, can_approve: true, can_reject: true },
          }),
        ),
      ),
      http.post('/api/v1/registrations/:id/reject', async ({ request }) => {
        note = ((await request.json()) as { note: string }).note
        return HttpResponse.json(registration({ status: 'REJECTED' }))
      }),
    )
    const { user } = renderApp('/danismanlik/kayitlar/r-1')
    expect(await screen.findByRole('heading', { name: 'Ece Kaya' })).toBeInTheDocument()
    expect(screen.getByText('22290002 · BIL-EN-NO · 2026-FALL')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Geri gönder' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Geri gönder' }))
    expect(await within(dialog).findByText('En az 5 karakterlik bir gerekçe yazın.')).toBeInTheDocument()
    await user.type(within(dialog).getByLabelText('Gerekçe'), 'COM3035 eksik')
    await user.click(within(dialog).getByRole('button', { name: 'Geri gönder' }))
    await waitFor(() => expect(note).toBe('COM3035 eksik'))
  })
})
