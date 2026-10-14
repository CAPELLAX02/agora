import { expect, type Page } from '@playwright/test'

import { seedPassword, submitLogin } from './app'
import { freshTotp } from './totp'

/** MfaAccount, testte iki adımlı doğrulama kurulan hesabın sırrı ve son kullanılan kod adımıdır. */
export type MfaAccount = { username: string; secret: string; lastStep: { value: number } }

export const mfaAccount = (username: string): MfaAccount => ({ username, secret: '', lastStep: { value: 0 } })

/**
 * enableMfa, oturumu açık hesapta MFA bandından iki adımlı doğrulamayı kurar. Etkinleştiren
 * oturum yeniden doğrulanmış sayılır: MFA isteyen yetkiler hemen açılır.
 */
export async function enableMfa(page: Page, account: MfaAccount) {
  await page.getByRole('link', { name: 'İki adımlı doğrulamayı aç' }).click()
  await page.getByRole('button', { name: 'Kurulumu başlat' }).click()
  const dialog = page.getByRole('dialog')
  const key = dialog.getByLabel('Kurulum anahtarı')
  await expect(key).toHaveText(/[A-Z2-7 ]{20,}/)
  account.secret = (await key.innerText()).replace(/\s/g, '')

  await dialog.getByLabel('Doğrulama kodu').fill(await freshTotp(account.secret, account.lastStep))
  await dialog.getByLabel('Mevcut parola').fill(seedPassword)
  await dialog.getByRole('button', { name: 'Etkinleştir' }).click()

  const codes = page.getByRole('dialog', { name: 'Kurtarma kodlarınız' })
  await expect(codes.getByRole('listitem')).toHaveCount(10)
  await codes.getByLabel('Kodları güvenli bir yere kaydettim').check()
  await codes.getByRole('button', { name: 'Kapat' }).click()
}

/** mfaLogin, MFA açık hesabın iki adımlı girişidir. */
export async function mfaLogin(page: Page, account: MfaAccount) {
  await submitLogin(page, account.username, seedPassword)
  await page.getByLabel('Doğrulama kodu').fill(await freshTotp(account.secret, account.lastStep))
  await expect(page.getByRole('heading', { name: /Hoş geldiniz/ })).toBeVisible()
}
