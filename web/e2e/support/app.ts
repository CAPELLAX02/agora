import { expect, type Page } from '@playwright/test'

/** seedPassword, demo hesaplarının parolasıdır (backend/cmd/seed). */
export const seedPassword = process.env.AGORA_SEED_PASSWORD ?? 'agora-dev-parola'

export async function submitLogin(page: Page, username: string, password: string) {
  await page.goto('/giris')
  await page.getByLabel('Öğrenci / personel numarası').fill(username)
  await page.getByLabel('Parola', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Giriş yap' }).click()
}

/** login, MFA'sız bir hesapla giriş yapar ve ana sayfanın açılmasını bekler. */
export async function login(page: Page, username: string, password = seedPassword) {
  await submitLogin(page, username, password)
  await expect(page.getByRole('heading', { name: /Hoş geldiniz/ })).toBeVisible()
}

export async function logout(page: Page) {
  await page.getByRole('button', { name: 'Hesap menüsü' }).click()
  await page.getByRole('menuitem', { name: 'Çıkış yap' }).click()
  await expect(page.getByText('Çıkış yaptınız.')).toBeVisible()
}

/** nav, yan menüdeki bağlantıdır. */
export const nav = (page: Page, name: string) =>
  page.getByRole('navigation', { name: 'Ana menü' }).getByRole('link', { name })
