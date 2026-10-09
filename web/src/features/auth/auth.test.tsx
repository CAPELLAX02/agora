import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { grants, me, problem, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

async function fillLogin(
  user: ReturnType<typeof renderApp>['user'],
  username = 'P90001',
  password = 'parola-123456',
) {
  await user.type(await screen.findByLabelText('Öğrenci / personel numarası'), username)
  await user.type(screen.getByLabelText('Parola'), password)
  await user.click(screen.getByRole('button', { name: 'Giriş yap' }))
}

describe('giriş', () => {
  it('oturumu olmayan kullanıcıyı girişe yönlendirir, girişten sonra istediği sayfaya döndürür', async () => {
    let body: unknown
    server.use(
      http.post('/api/v1/auth/login', async ({ request }) => {
        body = await request.json()
        expect(request.headers.get('X-Agora-Client')).toBe('web')
        return HttpResponse.json(tokens())
      }),
    )
    const { user, router } = renderApp('/profil')

    await fillLogin(user, '  P90001 ')
    expect(await screen.findByRole('heading', { name: 'Profilim' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/profil')
    expect(body).toEqual({ username: 'P90001', password: 'parola-123456' })
  })

  it('boş alanları sunucuya göndermeden uyarır', async () => {
    const { user } = renderApp('/giris')
    await user.click(await screen.findByRole('button', { name: 'Giriş yap' }))
    expect(await screen.findByText('Numaranızı girin.')).toBeInTheDocument()
    expect(screen.getByText('Parolanızı girin.')).toBeInTheDocument()
  })

  it('hatalı parolada mesaj gösterir ve parolayı temizler', async () => {
    server.use(
      http.post('/api/v1/auth/login', () =>
        HttpResponse.json(problem(401, 'INVALID_CREDENTIALS'), { status: 401 }),
      ),
    )
    const { user } = renderApp('/giris')
    await fillLogin(user)
    expect(await screen.findByText('Numara ya da parola hatalı.')).toBeInTheDocument()
    expect(screen.getByLabelText('Parola')).toHaveValue('')
  })

  it('kilitli hesapta kalan süreyi Retry-After başlığından gösterir', async () => {
    server.use(
      http.post('/api/v1/auth/login', () =>
        HttpResponse.json(problem(429, 'ACCOUNT_LOCKED'), { status: 429, headers: { 'Retry-After': '95' } }),
      ),
    )
    const { user } = renderApp('/giris')
    await fillLogin(user)
    expect(await screen.findByText(/2 dakika sonra tekrar deneyin/)).toBeInTheDocument()
  })

  it('iki adımlı doğrulamada kodu ister, yanlış kodda uyarır, doğru kodla oturumu açar', async () => {
    const codes: unknown[] = []
    server.use(
      http.post('/api/v1/auth/login', () =>
        HttpResponse.json({ mfa_required: true, mfa_token: 'mfa-1', expires_in: 300 }),
      ),
      http.post('/api/v1/auth/mfa/verify', async ({ request }) => {
        const b = (await request.json()) as { code?: string; mfa_token: string }
        codes.push(b)
        return b.code === '123456'
          ? HttpResponse.json(tokens())
          : HttpResponse.json(problem(401, 'INVALID_MFA_CODE'), { status: 401 })
      }),
    )
    const { user } = renderApp('/')
    await fillLogin(user)

    const code = await screen.findByLabelText('Doğrulama kodu')
    // Rakam dışı karakterler atılır, altıncı hanede kendiliğinden gönderilir.
    await user.type(code, '65-43-21')
    expect(await screen.findByText('Kod hatalı. Tekrar deneyin.')).toBeInTheDocument()
    await user.type(screen.getByLabelText('Doğrulama kodu'), '123456')

    expect(await screen.findByRole('heading', { name: /Hoş geldiniz, Ayşe/ })).toBeInTheDocument()
    expect(codes).toEqual([
      { mfa_token: 'mfa-1', code: '654321' },
      { mfa_token: 'mfa-1', code: '123456' },
    ])
  })

  it('kurtarma koduyla giriş yapılabilir', async () => {
    server.use(
      http.post('/api/v1/auth/login', () =>
        HttpResponse.json({ mfa_required: true, mfa_token: 'mfa-2', expires_in: 300 }),
      ),
      http.post('/api/v1/auth/mfa/verify', async ({ request }) => {
        const b = (await request.json()) as { recovery_code?: string }
        return b.recovery_code === 'k7m2p-x9qaz'
          ? HttpResponse.json(tokens())
          : HttpResponse.json(problem(401, 'INVALID_MFA_CODE'), { status: 401 })
      }),
    )
    const { user } = renderApp('/')
    await fillLogin(user)
    await user.click(await screen.findByRole('button', { name: /kurtarma kodu kullan/ }))
    await user.type(screen.getByLabelText('Kurtarma kodu'), 'k7m2p-x9qaz')
    await user.click(screen.getByRole('button', { name: 'Doğrula' }))
    expect(await screen.findByRole('heading', { name: /Hoş geldiniz/ })).toBeInTheDocument()
  })

  it('ikinci adımın süresi dolunca parolaya geri döner', async () => {
    server.use(
      http.post('/api/v1/auth/login', () =>
        HttpResponse.json({ mfa_required: true, mfa_token: 'x', expires_in: 300 }),
      ),
      http.post('/api/v1/auth/mfa/verify', () =>
        HttpResponse.json(problem(401, 'INVALID_MFA_TOKEN'), { status: 401 }),
      ),
    )
    const { user } = renderApp('/')
    await fillLogin(user)
    await user.type(await screen.findByLabelText('Doğrulama kodu'), '111111')
    expect(await screen.findByText('Doğrulama süresi doldu. Lütfen tekrar giriş yapın.')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Giriş yap' })).toBeInTheDocument()
  })
})

describe('oturum', () => {
  it('açılışta refresh çereziyle oturumu geri yükler', async () => {
    server.use(http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())))
    renderApp('/')
    expect(await screen.findByRole('heading', { name: /Hoş geldiniz, Ayşe/ })).toBeInTheDocument()
  })

  it('süresi dolan access token sessizce yenilenir', async () => {
    let refreshes = 0
    server.use(
      http.post('/api/v1/auth/refresh', () => {
        refreshes++
        return HttpResponse.json(tokens({ access_token: `access-${refreshes}` }))
      }),
      http.get('/api/v1/me', ({ request }) =>
        // İlk token (açılışta alınan) süresi dolmuş sayılır.
        request.headers.get('Authorization') === 'Bearer access-1'
          ? HttpResponse.json(problem(401, 'INVALID_TOKEN'), { status: 401 })
          : HttpResponse.json(me()),
      ),
    )
    renderApp('/')
    expect(await screen.findByRole('heading', { name: /Hoş geldiniz/ })).toBeInTheDocument()
    expect(refreshes).toBe(2)
  })

  it('yenileme reddedilince girişe döner ve sebebini söyler', async () => {
    let refreshes = 0
    server.use(
      http.post('/api/v1/auth/refresh', () => {
        refreshes++
        return refreshes === 1
          ? HttpResponse.json(tokens())
          : HttpResponse.json(problem(401, 'INVALID_REFRESH_TOKEN'), { status: 401 })
      }),
      http.get('/api/v1/me', () => HttpResponse.json(problem(401, 'SESSION_REVOKED'), { status: 401 })),
    )
    renderApp('/')
    expect(
      await screen.findByText('Oturumunuzun süresi doldu. Lütfen tekrar giriş yapın.'),
    ).toBeInTheDocument()
  })

  it('menü yetkilere göre süzülür ve çıkış giriş ekranına götürür', async () => {
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
      http.get('/api/v1/me/permissions', () => HttpResponse.json(grants('profile:read_own'))),
    )
    const { user } = renderApp('/')
    expect(await screen.findByRole('link', { name: 'Profilim' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Kullanıcılar' })).not.toBeInTheDocument()

    await user.click(await screen.findByRole('button', { name: 'Hesap menüsü' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Çıkış yap' }))
    expect(await screen.findByText('Çıkış yaptınız.')).toBeInTheDocument()
  })

  it('MFA gerektiren yetkisi olan kullanıcıya MFA kurulumunu önerir', async () => {
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens())),
      http.get('/api/v1/me', () => HttpResponse.json(me({ mfa_required: true }))),
    )
    renderApp('/')
    expect(await screen.findByText('İki adımlı doğrulama gerekli')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'İki adımlı doğrulamayı aç' })).toHaveAttribute(
      'href',
      '/guvenlik?tab=mfa',
    )
  })
})

describe('parola', () => {
  it('ilk girişte parola değiştirmeye zorlar ve politika ihlallerini gösterir', async () => {
    server.use(
      http.post('/api/v1/auth/login', () => HttpResponse.json(tokens({ must_change_password: true }))),
      http.post('/api/v1/me/password', async ({ request }) => {
        const b = (await request.json()) as { new_password: string }
        if (b.new_password === 'parola12345') {
          return HttpResponse.json(
            problem(400, 'VALIDATION_FAILED', {
              errors: [{ field: 'new_password', message: 'çok yaygın', code: 'TOO_COMMON' }],
            }),
            { status: 400 },
          )
        }
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { user } = renderApp('/')
    await fillLogin(user)

    expect(await screen.findByRole('heading', { name: 'Parolanızı değiştirin' })).toBeInTheDocument()
    await user.type(screen.getByLabelText('Mevcut parola'), 'gecici-parola-1')
    await user.type(screen.getByLabelText('Yeni parola'), 'kisa')
    await user.type(screen.getByLabelText('Yeni parola (tekrar)'), 'kisa')
    await user.click(screen.getByRole('button', { name: 'Parolayı değiştir' }))
    expect(await screen.findByText('Parola en az 10 karakter olmalı.')).toBeInTheDocument()

    await user.clear(screen.getByLabelText('Yeni parola'))
    await user.type(screen.getByLabelText('Yeni parola'), 'parola12345')
    await user.clear(screen.getByLabelText('Yeni parola (tekrar)'))
    await user.type(screen.getByLabelText('Yeni parola (tekrar)'), 'parola12345')
    await user.click(screen.getByRole('button', { name: 'Parolayı değiştir' }))
    expect(await screen.findByText('Bu parola çok yaygın ve kolay tahmin edilir.')).toBeInTheDocument()

    await user.clear(screen.getByLabelText('Yeni parola'))
    await user.type(screen.getByLabelText('Yeni parola'), 'mavi gökyüzü uzun cümle')
    await user.clear(screen.getByLabelText('Yeni parola (tekrar)'))
    await user.type(screen.getByLabelText('Yeni parola (tekrar)'), 'mavi gökyüzü uzun cümle')
    await user.click(screen.getByRole('button', { name: 'Parolayı değiştir' }))
    expect(await screen.findByRole('heading', { name: /Hoş geldiniz/ })).toBeInTheDocument()
  })

  it('aktivasyon bağlantısıyla parola belirler ve token adres çubuğundan silinir', async () => {
    window.history.replaceState(null, '', '/sifre-sifirla#token=tok-1')
    server.use(
      http.post('/api/v1/auth/password/reset/verify', () =>
        HttpResponse.json({ purpose: 'ACTIVATION', expires_at: '2026-10-11T09:00:00Z' }),
      ),
      http.post('/api/v1/auth/password/reset', async ({ request }) => {
        expect(await request.json()).toEqual({ token: 'tok-1', new_password: 'mavi gökyüzü uzun cümle' })
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { user } = renderApp('/sifre-sifirla')
    expect(await screen.findByRole('heading', { name: 'Hesabınızı etkinleştirin' })).toBeInTheDocument()
    expect(window.location.hash).toBe('')

    await user.type(screen.getByLabelText('Yeni parola'), 'mavi gökyüzü uzun cümle')
    await user.type(screen.getByLabelText('Yeni parola (tekrar)'), 'mavi gökyüzü uzun cümle')
    await user.click(screen.getByRole('button', { name: 'Parolayı kaydet' }))
    expect(
      await screen.findByText('Hesabınız etkinleştirildi. Belirlediğiniz parolayla giriş yapabilirsiniz.'),
    ).toBeInTheDocument()
  })

  it('geçersiz bağlantıda yeni bağlantı istemeyi önerir', async () => {
    window.history.replaceState(null, '', '/sifre-sifirla#token=eski')
    server.use(
      http.post('/api/v1/auth/password/reset/verify', () =>
        HttpResponse.json(problem(400, 'INVALID_RESET_TOKEN'), { status: 400 }),
      ),
    )
    renderApp('/sifre-sifirla')
    expect(await screen.findByRole('heading', { name: 'Bağlantı geçersiz' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Yeni bağlantı iste' })).toHaveAttribute(
      'href',
      '/sifremi-unuttum',
    )
  })

  it('parolamı unuttum her zaman aynı mesajı gösterir', async () => {
    server.use(http.post('/api/v1/auth/password/forgot', () => new HttpResponse(null, { status: 202 })))
    const { user } = renderApp('/sifremi-unuttum')
    await user.type(await screen.findByLabelText('Öğrenci / personel numarası ya da e-posta'), '22290001')
    await user.click(screen.getByRole('button', { name: 'Bağlantı gönder' }))
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'E-postanızı kontrol edin' })).toBeInTheDocument(),
    )
  })
})
