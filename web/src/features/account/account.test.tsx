import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { me, problem, tokens } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { server } from '@/test/server'

const signedIn = () => http.post('/api/v1/auth/refresh', () => HttpResponse.json(tokens()))

describe('iki adımlı doğrulama kurulumu', () => {
  it('QR kodu gösterir, kodu doğrular, kurtarma kodlarını bir kez gösterir ve oturumu yeniler', async () => {
    let enabled = false
    let refreshes = 0
    const codes = Array.from({ length: 10 }, (_, i) => `kod${i}a-bcdef`)
    server.use(
      http.post('/api/v1/auth/refresh', () => {
        refreshes++
        return HttpResponse.json(tokens())
      }),
      http.get('/api/v1/me/mfa', () =>
        HttpResponse.json({
          enabled,
          enabled_at: enabled ? '2026-10-09T08:00:00Z' : null,
          recovery_codes_remaining: enabled ? 10 : 0,
        }),
      ),
      http.post('/api/v1/me/mfa/setup', () =>
        HttpResponse.json({
          secret: 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP',
          otpauth_uri: 'otpauth://totp/Agora:P90001?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=Agora',
        }),
      ),
      http.post('/api/v1/me/mfa/enable', async ({ request }) => {
        const b = (await request.json()) as { code: string; current_password: string }
        if (b.current_password !== 'dogru-parola') {
          return HttpResponse.json(problem(400, 'INVALID_CURRENT_PASSWORD'), { status: 400 })
        }
        enabled = true
        return HttpResponse.json({ recovery_codes: codes })
      }),
    )
    const { user } = renderApp('/guvenlik?tab=mfa')

    await user.click(await screen.findByRole('button', { name: 'Kurulumu başlat' }))
    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByLabelText('Kurulum anahtarı')).toHaveTextContent('JBSW Y3DP EHPK 3PXP')
    expect(within(dialog).getByRole('img', { name: /QR kodu okutun/ })).toBeInTheDocument()

    const enable = within(dialog).getByRole('button', { name: 'Etkinleştir' })
    expect(enable).toBeDisabled()
    await user.type(within(dialog).getByLabelText('Doğrulama kodu'), '123456')
    await user.type(within(dialog).getByLabelText('Mevcut parola'), 'yanlis')
    await user.click(enable)
    expect(await within(dialog).findByText('Mevcut parola hatalı.')).toBeInTheDocument()

    await user.type(within(dialog).getByLabelText('Doğrulama kodu'), '123456')
    await user.clear(within(dialog).getByLabelText('Mevcut parola'))
    await user.type(within(dialog).getByLabelText('Mevcut parola'), 'dogru-parola')
    await user.click(within(dialog).getByRole('button', { name: 'Etkinleştir' }))

    const codesDialog = await screen.findByRole('dialog', { name: 'Kurtarma kodlarınız' })
    for (const c of codes) {
      expect(within(codesDialog).getByText(c)).toBeInTheDocument()
    }
    // Kullanıcı kaydettiğini onaylamadan pencere kapanmaz.
    const close = within(codesDialog).getByRole('button', { name: 'Kapat' })
    expect(close).toBeDisabled()
    await user.click(within(codesDialog).getByLabelText('Kodları güvenli bir yere kaydettim'))
    await user.click(close)

    expect(await screen.findByText(/tarihinden beri açık/)).toBeInTheDocument()
    // Açılışta bir, etkinleştirmeden sonra AMR'si güncellenmiş token için bir yenileme.
    await waitFor(() => expect(refreshes).toBe(2))
  })
})

describe('oturumlar', () => {
  it('diğer oturumları onayla kapatır', async () => {
    let ended = false
    server.use(
      signedIn(),
      http.get('/api/v1/me/sessions', () =>
        HttpResponse.json({
          items: [
            {
              id: 's1',
              client: 'WEB',
              browser: 'Firefox 140',
              os: 'macOS',
              device: 'DESKTOP',
              ip: '203.0.113.7',
              created_at: '2026-10-09T08:00:00Z',
              last_seen_at: '2026-10-09T08:05:00Z',
              expires_at: '2026-10-09T10:05:00Z',
              current: true,
            },
            ...(ended
              ? []
              : [
                  {
                    id: 's2',
                    client: 'MOBILE',
                    browser: null,
                    os: 'Android',
                    device: 'MOBILE',
                    ip: null,
                    created_at: '2026-10-08T08:00:00Z',
                    last_seen_at: '2026-10-08T09:00:00Z',
                    expires_at: '2026-10-09T09:00:00Z',
                    current: false,
                  },
                ]),
          ],
        }),
      ),
      http.delete('/api/v1/me/sessions', () => {
        ended = true
        return HttpResponse.json({ revoked: 1 })
      }),
    )
    const { user } = renderApp('/guvenlik?tab=sessions')
    expect(await screen.findByText('Firefox 140 · macOS')).toBeInTheDocument()
    expect(screen.getByText('Bilinmeyen tarayıcı · Android')).toBeInTheDocument()
    expect(screen.getByText('Bu cihaz')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Diğer bütün oturumları kapat' }))
    const confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: 'Diğer bütün oturumları kapat' }))

    expect(await screen.findByText('1 oturum kapatıldı.')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText('Bilinmeyen tarayıcı · Android')).not.toBeInTheDocument())
  })
})

describe('profil', () => {
  it('İngilizcede rol adını katalogdan gösterir', async () => {
    const { default: i18n } = await import('@/i18n')
    await i18n.changeLanguage('en')
    server.use(
      signedIn(),
      http.get('/api/v1/me', () => HttpResponse.json(me())),
      http.get('/api/v1/roles', () =>
        HttpResponse.json({
          items: [
            {
              code: 'SYSTEM_ADMIN',
              name_tr: 'Sistem Yöneticisi',
              name_en: 'System Administrator',
              scope_type: 'UNIVERSITY',
              description: null,
              permissions: [],
            },
          ],
        }),
      ),
    )
    renderApp('/profil')
    expect(await screen.findByRole('cell', { name: 'System Administrator' })).toBeInTheDocument()
  })
})
