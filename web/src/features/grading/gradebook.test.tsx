import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Gradebook, ScoreImportPreview } from '@/shared/api/generated'
import { grants, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const window = (open: boolean) => ({
  open,
  starts_at: open ? '2026-10-14T07:00:00Z' : null,
  ends_at: open ? '2026-11-04T07:00:00Z' : null,
  next_starts_at: open ? null : '2026-12-28T05:00:00Z',
})

const component = (
  id: string,
  code: string,
  tr: string,
  en: string,
  weight: number,
  editable: boolean,
  over = {},
) => ({
  id,
  type: { code, name_tr: tr, name_en: en, category: code === 'FINAL' ? 'FINAL' : 'IN_TERM' },
  sequence_no: 1,
  label_tr: tr,
  label_en: en,
  weight,
  version: 3,
  published_at: null,
  entry_window: window(editable),
  editable,
  scored_count: 1,
  absent_count: 0,
  average: 70,
  ...over,
})

const student = (enrollment: string, no: string, first: string, last: string, scores = {}) => ({
  enrollment_id: enrollment,
  student: { id: `st-${no}`, student_no: no, first_name: first, last_name: last },
  program: { code: 'BIL-EN-NO', name_tr: 'Bilgisayar Mühendisliği', name_en: 'Computer Engineering' },
  attempt_no: 1,
  attendance_status: 'CONTINUING',
  scores,
  letter: null,
  makeup_eligible: false,
})

function book(over: Partial<Gradebook> = {}): Gradebook {
  return {
    section: {
      id: 's1',
      section_code: '1',
      course: {
        id: 'c',
        code: 'COM1007',
        name_tr: 'Bilgisayar Programlama I',
        name_en: 'Programming I',
        grading_mode: 'LETTER',
      },
      term: { id: 't1', code: '2026-FALL' },
      plan_locked: true,
      instructor_role: 'PRIMARY',
      grades_published_at: null,
      makeup_published_at: null,
    },
    components: [
      component('k-mid', 'MIDTERM', 'Ara sınav', 'Midterm', 25, true, {
        published_at: '2026-10-14T08:00:00Z',
      }),
      component('k-lab', 'LAB', 'Laboratuvar', 'Lab', 15, true),
      component('k-fin', 'FINAL', 'Final sınavı', 'Final', 60, false, { scored_count: 0, average: null }),
    ],
    students: [
      student('e1', '26290101', 'Ada', 'Aydemir', { 'k-mid': { score: 70, absent: false } }),
      student('e2', '26290102', 'Berk', 'Karaca'),
    ],
    can_enter: true,
    mfa_required: false,
    ...over,
  } as unknown as Gradebook
}

function setup(data: Gradebook) {
  const puts: { id: string; ifMatch: string | null; body: unknown }[] = []
  server.use(
    http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
    http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('course:read', 'score:enter'))),
    http.get('/api/v1/sections/:id/gradebook', () => HttpResponse.json(data)),
    http.put('/api/v1/assessment-components/:id/scores', async ({ request, params }) => {
      puts.push({
        id: params.id as string,
        ifMatch: request.headers.get('If-Match'),
        body: await request.json(),
      })
      return HttpResponse.json(data)
    }),
  )
  return { puts, ...renderApp('/subeler/s1/notlar') }
}

describe('not tablosu', () => {
  it('öğretim elemanı notları taslak olarak girer, geçersizi düzeltir ve bileşen bileşen kaydeder', async () => {
    const { user, puts } = setup(book())

    expect(await screen.findByRole('heading', { name: 'Not tablosu' })).toBeInTheDocument()
    expect(screen.getByText('Not girişi kapalı · açılış', { exact: false })).toBeInTheDocument()
    // Final kapalı: giriş alanı yok.
    expect(screen.queryByLabelText('Ada Aydemir · Final sınavı')).not.toBeInTheDocument()

    const adaMid = screen.getByLabelText('Ada Aydemir · Ara sınav')
    expect(adaMid).toHaveValue('70')
    await user.clear(adaMid)
    await user.type(adaMid, '72,5')
    await user.type(screen.getByLabelText('Berk Karaca · Ara sınav'), 'g')
    await user.type(screen.getByLabelText('Berk Karaca · Laboratuvar'), '150')

    const bar = screen.getByRole('region', { name: 'Kaydedilmemiş değişiklikler' })
    expect(within(bar).getByText('3 kaydedilmemiş değişiklik')).toBeInTheDocument()
    expect(within(bar).getByText('1 hücre geçersiz')).toBeInTheDocument()
    expect(within(bar).getByRole('button', { name: 'Kaydet' })).toBeDisabled()
    expect(screen.getByLabelText('Berk Karaca · Laboratuvar')).toHaveAttribute('aria-invalid', 'true')

    await user.clear(screen.getByLabelText('Berk Karaca · Laboratuvar'))
    await user.type(screen.getByLabelText('Berk Karaca · Laboratuvar'), '88')
    await user.click(within(bar).getByRole('button', { name: 'Kaydet' }))

    await waitFor(() => expect(puts).toHaveLength(2))
    expect(puts).toEqual([
      {
        id: 'k-mid',
        ifMatch: '"3"',
        body: {
          items: [
            { enrollment_id: 'e1', score: 72.5, absent: false },
            { enrollment_id: 'e2', score: null, absent: true },
          ],
        },
      },
      { id: 'k-lab', ifMatch: '"3"', body: { items: [{ enrollment_id: 'e2', score: 88, absent: false }] } },
    ])
    expect(await screen.findByText('3 not kaydedildi')).toBeInTheDocument()
  })

  it('klavyeyle satır ve sütun değiştirilir, Esc hücreyi geri alır', async () => {
    const { user } = setup(book())
    const adaMid = await screen.findByLabelText('Ada Aydemir · Ara sınav')
    await user.click(adaMid)
    await user.keyboard('{Enter}')
    expect(screen.getByLabelText('Berk Karaca · Ara sınav')).toHaveFocus()
    await user.keyboard('{ArrowRight}')
    expect(screen.getByLabelText('Berk Karaca · Laboratuvar')).toHaveFocus()
    await user.keyboard('{ArrowUp}')
    expect(screen.getByLabelText('Ada Aydemir · Laboratuvar')).toHaveFocus()
    await user.keyboard('55{Escape}')
    expect(screen.getByLabelText('Ada Aydemir · Laboratuvar')).toHaveValue('')
    expect(screen.queryByRole('region', { name: 'Kaydedilmemiş değişiklikler' })).not.toBeInTheDocument()
  })

  it('MFA olmadan ve ilandan sonra tablo salt okunurdur', async () => {
    const readonly = book({ can_enter: false, mfa_required: true })
    for (const c of readonly.components) {
      c.editable = false
    }
    setup(readonly)
    expect(await screen.findByText(/iki adımlı doğrulamayla açılmış bir oturum gerekir/)).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: "Excel'den içe aktar" })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Öğrencilere yayımla' })).not.toBeInTheDocument()
  })

  it('Excel dosyası önce önizlenir; hatalı satır varken yazılmaz, düzeltilmiş dosya tek seferde yazılır', async () => {
    const previews: { type: string | null }[] = []
    let applied: { ifMatch: string | null; type: string | null } | null = null
    const preview = (errors: boolean): ScoreImportPreview => ({
      component_id: 'k-lab',
      version: 4,
      column: 'Laboratuvar',
      rows: [
        {
          line: 8,
          student_no: '26290101',
          student: { id: 'st1', student_no: '26290101', first_name: 'Ada', last_name: 'Aydemir' },
          enrollment_id: 'e1',
          status: 'CHANGED',
          current: null,
          next: { score: 81, absent: false },
          error_code: null,
          error: null,
        },
        ...(errors
          ? [
              {
                line: 9,
                student_no: '99999999',
                student: null,
                enrollment_id: null,
                status: 'ERROR' as const,
                current: null,
                next: null,
                error_code: 'NOT_IN_SECTION' as const,
                error: 'Bu numarada şubenin kesin kayıtlı öğrencisi yok.',
              },
            ]
          : []),
      ],
      missing: [{ id: 'st2', student_no: '26290102', first_name: 'Berk', last_name: 'Karaca' }],
      summary: { changed: 1, unchanged: 0, empty: 0, errors: errors ? 1 : 0 },
    })
    const { user } = setup(book())
    server.use(
      http.post('/api/v1/assessment-components/:id/scores/import/preview', ({ request }) => {
        previews.push({ type: request.headers.get('Content-Type') })
        return HttpResponse.json(preview(previews.length === 1))
      }),
      http.post('/api/v1/assessment-components/:id/scores/import', ({ request }) => {
        applied = { ifMatch: request.headers.get('If-Match'), type: request.headers.get('Content-Type') }
        return HttpResponse.json(book())
      }),
    )

    await user.click(await screen.findByRole('button', { name: "Excel'den içe aktar" }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('combobox', { name: 'Bileşen' }))
    await user.click(await screen.findByRole('option', { name: /Laboratuvar/ }))

    const file = new File(['PK'], 'COM1007-1_notlar.xlsx')
    await user.upload(within(dialog).getByLabelText('Not dosyası'), file)
    const summary = await within(dialog).findByRole('region', { name: 'Önizleme' })
    expect(previews[0]?.type).toBe('application/vnd.openxmlformats-officedocument.spreadsheetml.sheet')
    // Hatalı satır varsa hatalılar sekmesi açılır ve yazma kapalıdır.
    expect(within(summary).getByText('Bu şubenin öğrencisi değil')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Önce hataları düzeltin' })).toBeDisabled()
    expect(within(summary).getByText(/Dosyada olmayan 1 öğrenci/)).toBeInTheDocument()

    await user.upload(within(dialog).getByLabelText('Not dosyası'), file)
    await waitFor(() => expect(previews).toHaveLength(2))
    const write = await within(dialog).findByRole('button', { name: '1 notu yaz' })
    expect(within(dialog).getByText('Ada Aydemir')).toBeInTheDocument()
    await user.click(write)
    await waitFor(() =>
      expect(applied).toEqual({
        ifMatch: '"4"',
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      }),
    )
    expect(await screen.findByText('Laboratuvar: 1 not içe aktarıldı')).toBeInTheDocument()
  })

  it('desteklenmeyen dosya türü gönderilmez', async () => {
    const { user } = setup(book())
    await user.click(await screen.findByRole('button', { name: "Excel'den içe aktar" }))
    const dialog = await screen.findByRole('dialog')
    // Tarayıcının dosya seçicisi accept'e uymayanı da seçtirebilir (“Tüm dosyalar”).
    fireEvent.change(within(dialog).getByLabelText('Not dosyası'), {
      target: { files: [new File(['x'], 'notlar.pdf')] },
    })
    expect(
      await within(dialog).findByText('Sadece .xlsx ve .csv dosyaları yüklenebilir.'),
    ).toBeInTheDocument()
  })
})
