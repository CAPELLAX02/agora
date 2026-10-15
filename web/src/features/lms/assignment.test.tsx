import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type {
  Activity,
  AssignmentGrade,
  AssignmentLink,
  AssignmentGradeImportPreview,
  AssignmentSettings,
  MySubmission,
  Space,
  StudentWork,
  Submission,
  SubmissionRoster,
} from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const settings: AssignmentSettings = {
  opens_at: null,
  due_at: '2026-11-02T20:59:00Z',
  cutoff_at: null,
  max_attempts: 2,
  submission_types: ['FILE', 'ONLINE_TEXT'],
  max_files: 2,
  max_file_size_bytes: 10 * 1048576,
  allowed_extensions: ['.pdf'],
  max_score: 50,
  late_policy: 'PENALTY',
  late_penalty_pct_per_day: 10,
  grades_released_at: null,
  submission_state: 'OPEN',
}

const assignment: Activity = {
  id: 'a9',
  topic_id: 'tp1',
  type: 'ASSIGNMENT',
  title: 'Ödev 1: Döngüler',
  description_html: '<p>Üç soruyu çözün.</p>',
  is_visible: true,
  availability: 'OPEN',
  available_from: null,
  available_until: null,
  version: 1,
  updated_at: '2026-10-10T09:00:00Z',
  files: [],
  url: null,
  open_in_new_tab: null,
  body_html: null,
  assignment: settings,
}

const space = (role: Space['role']): Space => ({
  section_id: 's1',
  section_code: '1',
  course: {
    id: 'c1',
    code: 'COM1007',
    name_tr: 'Bilgisayar Programlama I',
    name_en: 'Computer Programming I',
  },
  term: { id: 't-fall', code: '2026-FALL' },
  role,
  can_edit: role !== 'STUDENT',
  summary_html: '',
  instructors: [],
  topics: [
    {
      id: 'tp1',
      title: 'Genel',
      summary_html: '',
      is_visible: true,
      availability: 'OPEN',
      available_from: null,
      available_until: null,
      version: 1,
      activities: [assignment],
    },
  ],
})

const attempt = (over: Partial<Submission> = {}): Submission => ({
  id: 'sub1',
  attempt_no: 1,
  status: 'DRAFT',
  online_text_html: '<p>Cevaplar</p>',
  files: [{ id: 'f1', name: 'odev.pdf', mime_type: 'application/pdf', size: 20480 }],
  submitted_at: null,
  is_late: false,
  version: 3,
  updated_at: '2026-10-14T09:00:00Z',
  ...over,
})

const grade: AssignmentGrade = {
  score: 45,
  late_penalty_pct: 10,
  final_score: 40.5,
  max_score: 50,
  feedback_html: '<p>Güzel çözüm, geç kaldın.</p>',
  attempt_no: 1,
  graded_by: 'Ayşe Demir',
  graded_at: '2026-11-05T10:00:00Z',
  version: 1,
}

const link: AssignmentLink = {
  link: null,
  components: [
    { id: 'lab', label_tr: 'Laboratuvar', label_en: 'Laboratory', weight: 20, linked_assignments: 1 },
    { id: 'mid', label_tr: 'Ara sınav', label_en: 'Midterm exam', weight: 30, linked_assignments: 0 },
  ],
  plan_locked: true,
  grades_published: false,
  sync_state: null,
}

type Call = { method: string; path: string; body?: unknown; ifMatch: string | null }

function session(role: Space['role'], perms: string[]) {
  const calls: Call[] = []
  const record = async (request: Request) => {
    const text = await request.text()
    calls.push({
      method: request.method,
      path: new URL(request.url).pathname,
      body: text ? JSON.parse(text) : undefined,
      ifMatch: request.headers.get('If-Match'),
    })
  }
  server.use(
    http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
    http.get('/api/v1/me/permissions', () => HttpResponse.json(grants(...perms))),
    http.get('/api/v1/spaces/:id', () => HttpResponse.json(space(role))),
    http.get('/api/v1/activities/:id', () => HttpResponse.json(assignment)),
    http.get('/api/v1/activities/:id/link', () => HttpResponse.json(link)),
  )
  return { calls, record }
}

describe('ödev (öğrenci)', () => {
  it('teslim durumu görünür; taslak onayla gönderilir', async () => {
    const { calls, record } = session('STUDENT', ['registration:manage_own'])
    let mine: MySubmission = {
      assignment: settings,
      attempts: [attempt()],
      can_edit: true,
      can_submit: true,
      can_new_attempt: false,
      grade: null,
      comments: [],
    }
    server.use(
      http.get('/api/v1/activities/:id/my-submission', () => HttpResponse.json(mine)),
      http.post('/api/v1/activities/:id/my-submission/submit', async ({ request }) => {
        await record(request)
        mine = {
          ...mine,
          attempts: [attempt({ status: 'SUBMITTED', submitted_at: '2026-10-15T09:00:00Z', version: 4 })],
          can_edit: false,
          can_submit: false,
          can_new_attempt: true,
        }
        return HttpResponse.json(mine)
      }),
    )
    const { user } = renderApp('/derslerim/s1/etkinlik/a9')
    expect(await screen.findByRole('heading', { name: 'Ödev 1: Döngüler', level: 1 })).toBeInTheDocument()
    expect(await screen.findByText('Taslak (gönderilmedi)')).toBeInTheDocument()
    expect(screen.getByText('1 / 2')).toBeInTheDocument()
    expect(screen.getByText('Puanlar henüz yayımlanmadı')).toBeInTheDocument()
    expect(
      screen.getByText(/Son tarihten sonraki başlamış her gün için puandan %10 düşülür/),
    ).toBeInTheDocument()
    // Sınırlar durum tablosunda ve yükleme alanında yazar.
    expect(screen.getAllByText(/En çok 2 dosya, dosya başına 10 MB \(\.pdf\)/)).toHaveLength(2)
    expect(screen.getByText('odev.pdf')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Gönder' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/1 deneme hakkınız daha var/)).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Gönder' }))
    await waitFor(() =>
      expect(calls).toEqual([
        {
          method: 'POST',
          path: '/api/v1/activities/a9/my-submission/submit',
          body: undefined,
          ifMatch: '"3"',
        },
      ]),
    )
    expect(await screen.findByText('Ödev gönderildi.')).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: 'Yeni deneme başlat' })).toBeInTheDocument()
    expect(screen.getByText('Gönderildi')).toBeInTheDocument()
  })

  it('yayımlanan puan, geç kesintisi ve geri bildirim görünür; yorum yazılır', async () => {
    const { calls, record } = session('STUDENT', ['registration:manage_own'])
    const mine: MySubmission = {
      assignment: { ...settings, grades_released_at: '2026-11-05T11:00:00Z', submission_state: 'CLOSED' },
      attempts: [attempt({ status: 'SUBMITTED', submitted_at: '2026-11-03T08:00:00Z', is_late: true })],
      can_edit: false,
      can_submit: false,
      can_new_attempt: false,
      grade,
      comments: [],
    }
    server.use(
      http.get('/api/v1/activities/:id/my-submission', () => HttpResponse.json(mine)),
      http.post('/api/v1/activities/:id/my-submission/comments', async ({ request }) => {
        await record(request)
        return HttpResponse.json(mine, { status: 201 })
      }),
    )
    const { user } = renderApp('/derslerim/s1/etkinlik/a9')
    expect(await screen.findByText('40,5')).toBeInTheDocument()
    expect(screen.getByText('Geç teslim kesintisi %10 (verilen puan 45)')).toBeInTheDocument()
    expect(screen.getByText('Güzel çözüm, geç kaldın.')).toBeInTheDocument()
    expect(screen.getByText('Teslim süresi doldu.')).toBeInTheDocument()
    expect(screen.getAllByText('Geç gönderildi').length).toBeGreaterThan(0)

    await user.type(screen.getByRole('textbox', { name: 'Yorum' }), 'Teşekkürler hocam.')
    await user.click(screen.getByRole('button', { name: 'Yorumu gönder' }))
    await waitFor(() =>
      expect(calls).toEqual([
        {
          method: 'POST',
          path: '/api/v1/activities/a9/my-submission/comments',
          body: { body: 'Teşekkürler hocam.' },
          ifMatch: null,
        },
      ]),
    )
  })
})

describe('ödev (öğretim elemanı)', () => {
  const roster: SubmissionRoster = {
    assignment: settings,
    can_grade: true,
    summary: { students: 2, submitted: 1, drafts: 0, not_started: 1, late: 0, graded: 0, needs_grading: 1 },
    items: [
      {
        student: { user_id: 'u1', student_no: '26290101', first_name: 'Ada', last_name: 'Aydemir' },
        latest: {
          attempt_no: 1,
          status: 'SUBMITTED',
          submitted_at: '2026-10-14T12:00:00Z',
          is_late: false,
          file_count: 1,
          updated_at: '2026-10-14T12:00:00Z',
        },
        grade: null,
        comments: 0,
        needs_grading: true,
      },
      {
        student: { user_id: 'u2', student_no: '26290102', first_name: 'Berk', last_name: 'Karaca' },
        latest: null,
        grade: null,
        comments: 0,
        needs_grading: false,
      },
    ],
  }
  const work: StudentWork = {
    student: roster.items[0]!.student,
    assignment: settings,
    can_grade: true,
    attempts: [attempt({ status: 'SUBMITTED', submitted_at: '2026-10-14T12:00:00Z' })],
    grade: null,
    comments: [],
  }

  it('özet ve süzme; teslim puanlanır ve puanlar yayımlanır', async () => {
    const { calls, record } = session('PRIMARY', ['assessment_plan:manage'])
    server.use(
      http.get('/api/v1/activities/:id/submissions', () => HttpResponse.json(roster)),
      http.get('/api/v1/activities/:id/submissions/:userId', () => HttpResponse.json(work)),
      http.put('/api/v1/activities/:id/submissions/:userId/grade', async ({ request }) => {
        await record(request)
        return HttpResponse.json({
          ...work,
          grade: { ...grade, score: 42.5, final_score: 42.5, late_penalty_pct: 0 },
        })
      }),
      http.post('/api/v1/activities/:id/grades/release', async ({ request }) => {
        await record(request)
        return HttpResponse.json({
          ...roster,
          assignment: { ...settings, grades_released_at: '2026-10-15T10:00:00Z' },
        })
      }),
    )
    const { user } = renderApp('/derslerim/s1/etkinlik/a9')
    const tiles = await screen.findByText('Puan bekleyen')
    expect(tiles.nextSibling).toHaveTextContent('1')
    expect(screen.getByRole('row', { name: /Berk Karaca/ })).toHaveTextContent('Henüz teslim edilmedi')

    await user.click(screen.getByRole('combobox', { name: 'Gösterilen öğrenciler' }))
    await user.click(await screen.findByRole('option', { name: 'Puanlanmayı bekleyenler' }))
    expect(screen.queryByRole('row', { name: /Berk Karaca/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Ada Aydemir teslimini puanla' }))
    const sheet = await screen.findByRole('dialog', { name: 'Ada Aydemir' })
    expect(await within(sheet).findByText('odev.pdf')).toBeInTheDocument()
    const score = within(sheet).getByLabelText('Puan (en çok 50)')
    await user.type(score, '55')
    await user.click(within(sheet).getByRole('button', { name: 'Puanı kaydet' }))
    expect(within(sheet).getByRole('alert')).toHaveTextContent('Puan 0 ile 50 arasında')
    await user.clear(score)
    await user.type(score, '42,5')
    await user.click(within(sheet).getByRole('button', { name: 'Puanı kaydet' }))
    await waitFor(() =>
      expect(calls.at(-1)).toEqual({
        method: 'PUT',
        path: '/api/v1/activities/a9/submissions/u1/grade',
        body: { score: 42.5, feedback_html: '' },
        ifMatch: null,
      }),
    )
    expect(await screen.findByText('Puan kaydedildi.')).toBeInTheDocument()
    await user.keyboard('{Escape}')

    await user.click(screen.getByRole('button', { name: 'Puanları yayımla' }))
    const confirm = await screen.findByRole('alertdialog')
    expect(within(confirm).getByText(/1 teslim hâlâ puanlanmayı bekliyor/)).toBeInTheDocument()
    await user.click(within(confirm).getByRole('button', { name: 'Puanları yayımla' }))
    await waitFor(() =>
      expect(calls.at(-1)).toMatchObject({ method: 'POST', path: '/api/v1/activities/a9/grades/release' }),
    )
    expect(await screen.findByText('Puanlar yayımlandı.')).toBeInTheDocument()
  })

  it('puanlar Excel şablonuyla içe aktarılır: önizleme, hatalı satır, yazma', async () => {
    session('PRIMARY', ['assessment_plan:manage'])
    const previews: (string | null)[] = []
    let applied: { ifMatch: string | null; type: string | null } | null = null
    const preview = (errors: boolean): AssignmentGradeImportPreview => ({
      version: 0,
      score_column: 'Puan',
      feedback_column: 'Geri Bildirim',
      rows: [
        {
          line: 5,
          student_no: '26290101',
          student: roster.items[0]!.student,
          status: 'CHANGED',
          current_score: null,
          next_score: 42.5,
          feedback_changed: true,
          error_code: null,
          error: null,
        },
        ...(errors
          ? [
              {
                line: 6,
                student_no: '99999999',
                student: null,
                status: 'ERROR' as const,
                current_score: null,
                next_score: null,
                feedback_changed: false,
                error_code: 'NOT_IN_SECTION' as const,
                error: 'Bu numarada şubenin kesin kayıtlı öğrencisi yok.',
              },
            ]
          : []),
      ],
      missing: [roster.items[1]!.student],
      summary: { changed: 1, unchanged: 0, empty: 0, errors: errors ? 1 : 0 },
    })
    server.use(
      http.get('/api/v1/activities/:id/submissions', () => HttpResponse.json(roster)),
      http.post('/api/v1/activities/:id/grades/import/preview', ({ request }) => {
        previews.push(request.headers.get('Content-Type'))
        return HttpResponse.json(preview(previews.length === 1))
      }),
      http.post('/api/v1/activities/:id/grades/import', ({ request }) => {
        applied = { ifMatch: request.headers.get('If-Match'), type: request.headers.get('Content-Type') }
        return HttpResponse.json(roster)
      }),
    )
    const { user } = renderApp('/derslerim/s1/etkinlik/a9')
    await user.click(await screen.findByRole('button', { name: "Excel'den içe aktar" }))
    const dialog = await screen.findByRole('dialog', { name: "Puanları Excel'den içe aktar" })
    const file = new File(['PK'], 'COM1007-1_Odev_1_puanlar.xlsx')
    await user.upload(within(dialog).getByLabelText('Puan dosyası'), file)
    const summary = await within(dialog).findByRole('region', { name: 'Önizleme' })
    expect(previews[0]).toBe('application/vnd.openxmlformats-officedocument.spreadsheetml.sheet')
    expect(within(summary).getByText('Şubede bu numarada öğrenci yok')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Önce hatalı satırları düzeltin' })).toBeDisabled()
    expect(within(summary).getByText('1 öğrenci dosyada yok (puanı değişmez)')).toBeInTheDocument()

    await user.upload(within(dialog).getByLabelText('Puan dosyası'), file)
    await waitFor(() => expect(previews).toHaveLength(2))
    expect(await within(dialog).findByText('Geri bildirim')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: '1 puanı yaz' }))
    await waitFor(() =>
      expect(applied).toEqual({
        ifMatch: '"0"',
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      }),
    )
    expect(await screen.findByText('1 öğrencinin puanı yazıldı.')).toBeInTheDocument()
  })

  it('ödev bir değerlendirme bileşenine ağırlıkla bağlanır', async () => {
    const { calls, record } = session('PRIMARY', ['assessment_plan:manage'])
    server.use(
      http.get('/api/v1/activities/:id/submissions', () => HttpResponse.json(roster)),
      http.put('/api/v1/activities/:id/link', async ({ request }) => {
        await record(request)
        return HttpResponse.json({ ...link, link: { component_id: 'lab', weight: 2 }, sync_state: 'SYNCED' })
      }),
    )
    const { user } = renderApp('/derslerim/s1/etkinlik/a9')
    const card = await screen.findByRole('region', { name: 'Not tablosu bağlantısı' })
    expect(within(card).getByText('Bağlı değil: ödev puanları not tablosuna akmaz.')).toBeInTheDocument()
    await user.click(within(card).getByRole('combobox', { name: 'Değerlendirme bileşeni' }))
    await user.click(await screen.findByRole('option', { name: /Laboratuvar · %20 · 1 ödev bağlı/ }))
    const weight = within(card).getByLabelText('Bileşen içi ağırlık')
    await user.clear(weight)
    await user.type(weight, '2')
    await user.click(within(card).getByRole('button', { name: 'Bağlantıyı kaydet' }))
    await waitFor(() =>
      expect(calls).toEqual([
        {
          method: 'PUT',
          path: '/api/v1/activities/a9/link',
          body: { component_id: 'lab', weight: 2 },
          ifMatch: null,
        },
      ]),
    )
    expect(await screen.findByText('Bağlantı kaydedildi; bileşen notları güncellendi.')).toBeInTheDocument()
  })
})
