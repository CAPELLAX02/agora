import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Assignment, Profile, UserSummary } from '@/shared/api/generated'
import { grants, problem, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const userId = '01a11d00-0000-7000-8000-0000000000bb'

const summary = (over: Partial<UserSummary> = {}): UserSummary => ({
  id: userId,
  username: '22290001',
  email: 'ogrenci@agora.test',
  first_name: 'Elif',
  last_name: 'Şahin',
  status: 'ACTIVE',
  kind: 'STUDENT',
  last_login_at: null,
  created_at: '2026-10-01T09:00:00Z',
  ...over,
})

const profile: Profile = {
  id: userId,
  username: '22290001',
  email: 'ogrenci@agora.test',
  first_name: 'Elif',
  last_name: 'Şahin',
  status: 'ACTIVE',
  must_change_password: false,
  mfa_enabled: false,
  last_login_at: null,
  roles: [],
}

const admin = (...perms: string[]) => [
  http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
  http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('user:read', ...perms))),
]

describe('kullanıcılar', () => {
  it('arama ve durum süzgeci sunucuya gider ve adreste tutulur', async () => {
    const queries: string[] = []
    server.use(
      ...admin(),
      http.get('/api/v1/users', ({ request }) => {
        const url = new URL(request.url)
        queries.push(url.search)
        return HttpResponse.json({
          items:
            url.searchParams.get('q') === 'elif'
              ? [summary()]
              : [summary(), summary({ id: 'x2', first_name: 'Can' })],
        })
      }),
    )
    const { user, router } = renderApp('/yonetim/kullanicilar')
    expect(await screen.findByRole('link', { name: 'Can Şahin' })).toBeInTheDocument()
    // Yönetim yetkisi olmayan kişi "yeni kullanıcı" düğmesini görmez.
    expect(screen.queryByRole('button', { name: 'Yeni kullanıcı' })).not.toBeInTheDocument()

    await user.type(screen.getByRole('searchbox', { name: 'Ara' }), 'elif')
    // Arama 300 ms bekletilir, sonra yeni liste yüklenir: önce eski satır gider, sonra yenisi gelir.
    await waitFor(() => expect(screen.queryByRole('link', { name: 'Can Şahin' })).not.toBeInTheDocument())
    expect(await screen.findByRole('link', { name: 'Elif Şahin' })).toHaveAttribute(
      'href',
      `/yonetim/kullanicilar/${userId}`,
    )
    expect(router.state.location.search).toBe('?q=elif')
    expect(Object.fromEntries(new URLSearchParams(queries.at(-1)))).toEqual({ limit: '25', q: 'elif' })
  })

  it('yeni hesap oluşturur ve sunucunun çakışma hatasını gösterir', async () => {
    let created = 0
    server.use(
      ...admin('user:manage'),
      http.get('/api/v1/users', () => HttpResponse.json({ items: [] })),
      http.post('/api/v1/users', async ({ request }) => {
        const b = (await request.json()) as { number: string }
        if (b.number === '22290001') {
          return HttpResponse.json(problem(409, 'ACCOUNT_EXISTS'), { status: 409 })
        }
        created++
        return HttpResponse.json({ ...profile, status: 'PENDING' }, { status: 201 })
      }),
      http.get(`/api/v1/users/${userId}`, () => HttpResponse.json({ ...profile, status: 'PENDING' })),
      http.get(`/api/v1/users/${userId}/roles`, () => HttpResponse.json({ items: [] })),
    )
    const { user, router } = renderApp('/yonetim/kullanicilar')
    await user.click(await screen.findByRole('button', { name: 'Yeni kullanıcı' }))
    const dialog = await screen.findByRole('dialog')

    await user.click(within(dialog).getByRole('button', { name: 'Hesabı oluştur' }))
    expect(await within(dialog).findAllByText('Bu alan zorunlu.')).toHaveLength(3)

    await user.type(within(dialog).getByLabelText('Öğrenci numarası'), '22290001')
    await user.type(within(dialog).getByLabelText('Ad'), 'Elif')
    await user.type(within(dialog).getByLabelText('Soyad'), 'Şahin')
    await user.type(within(dialog).getByLabelText('E-posta'), 'elif@agora.test')
    await user.click(within(dialog).getByRole('button', { name: 'Hesabı oluştur' }))
    expect(
      await within(dialog).findByText('Bu numara ya da e-posta başka bir hesapta kayıtlı.'),
    ).toBeInTheDocument()

    await user.clear(within(dialog).getByLabelText('Öğrenci numarası'))
    await user.type(within(dialog).getByLabelText('Öğrenci numarası'), '22290099')
    await user.click(within(dialog).getByRole('button', { name: 'Hesabı oluştur' }))
    await waitFor(() => expect(router.state.location.pathname).toBe(`/yonetim/kullanicilar/${userId}`))
    expect(created).toBe(1)
    expect(
      await screen.findByRole('button', { name: 'Aktivasyon e-postasını yeniden gönder' }),
    ).toBeInTheDocument()
  })

  it('bölüm kapsamlı rol atar: birim ve bölüm seçilmeden gönderilemez', async () => {
    let assigned: unknown
    const roles: Assignment[] = []
    server.use(
      ...admin('role:assign'),
      http.get(`/api/v1/users/${userId}`, () => HttpResponse.json(profile)),
      http.get(`/api/v1/users/${userId}/roles`, () => HttpResponse.json({ items: roles })),
      http.get('/api/v1/roles', () =>
        HttpResponse.json({
          items: [
            {
              code: 'ADVISOR',
              name_tr: 'Danışman',
              name_en: 'Advisor',
              scope_type: 'DEPARTMENT',
              description: 'Danışmanlık',
              permissions: [],
            },
          ],
        }),
      ),
      http.get('/api/v1/faculties', () =>
        HttpResponse.json({
          items: [
            {
              id: 'f1',
              code: 'MUH',
              name_tr: 'Mühendislik Fakültesi',
              name_en: 'Engineering',
              unit_type: 'FACULTY',
              campus: null,
              is_active: true,
            },
          ],
        }),
      ),
      http.get('/api/v1/faculties/f1/departments', () =>
        HttpResponse.json({
          items: [
            {
              id: 'd1',
              code: 'BIL',
              name_tr: 'Bilgisayar Mühendisliği',
              name_en: 'Computer Engineering',
              faculty: { id: 'f1', code: 'MUH', name_tr: 'Mühendislik' },
              is_active: true,
            },
          ],
        }),
      ),
      http.post(`/api/v1/users/${userId}/roles`, async ({ request }) => {
        assigned = await request.json()
        roles.push({
          id: 'a1',
          role: 'ADVISOR',
          role_name: 'Danışman',
          scope_type: 'DEPARTMENT',
          scope_id: 'd1',
          scope_name: 'Bilgisayar Mühendisliği',
          valid_from: '2026-10-09T09:00:00Z',
          valid_until: null,
          state: 'ACTIVE',
          assigned_by: null,
          reason: 'Güz dönemi',
          created_at: '2026-10-09T09:00:00Z',
        })
        return HttpResponse.json(roles[0], { status: 201 })
      }),
    )
    const { user } = renderApp(`/yonetim/kullanicilar/${userId}`)
    await user.click(await screen.findByRole('button', { name: 'Rol ata' }))
    const dialog = await screen.findByRole('dialog')
    const submit = within(dialog).getByRole('button', { name: 'Ata' })

    await user.click(within(dialog).getByRole('combobox', { name: 'Rol' }))
    await user.click(await screen.findByRole('option', { name: 'Danışman' }))
    await user.type(within(dialog).getByLabelText('Gerekçe'), 'Güz dönemi')
    expect(submit).toBeDisabled()

    await user.click(within(dialog).getByRole('combobox', { name: 'Birim' }))
    await user.click(await screen.findByRole('option', { name: 'Mühendislik Fakültesi' }))
    await user.click(within(dialog).getByRole('combobox', { name: 'Bölüm' }))
    await user.click(await screen.findByRole('option', { name: 'Bilgisayar Mühendisliği' }))
    await user.click(submit)

    expect(await screen.findByRole('cell', { name: 'Bilgisayar Mühendisliği' })).toBeInTheDocument()
    expect(assigned).toEqual({ role: 'ADVISOR', reason: 'Güz dönemi', scope_id: 'd1' })
  })
})
