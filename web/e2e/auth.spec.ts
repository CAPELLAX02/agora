import { expect, test } from '@playwright/test'

import { linkFromLatestEmail } from './support/mailpit'
import { login, logout, nav, seedPassword, submitLogin } from './support/app'

test('hatalı parola girişi reddedilir', async ({ page }) => {
  await submitLogin(page, '22290001', 'yanlış parola 123')
  await expect(page.getByText('Numara ya da parola hatalı.')).toBeVisible()
})

test('öğrenci giriş yapar, oturumu sayfa yenilenince korunur, çıkış yapar', async ({ page }) => {
  await login(page, '22290001')
  await expect(page.getByRole('heading', { name: 'Hoş geldiniz, Deniz' })).toBeVisible()
  // Öğrencinin yönetim yetkisi yok: menüde görünmez.
  await expect(nav(page, 'Profilim')).toBeVisible()
  await expect(nav(page, 'Kullanıcılar')).toHaveCount(0)

  // Access token bellekte: yenilenen sayfa oturumu refresh çereziyle geri yükler.
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Hoş geldiniz, Deniz' })).toBeVisible()

  await logout(page)
  await page.goto('/profil')
  await expect(page).toHaveURL(/\/giris$/)
})

test('geçici parolalı hesap ilk girişte parolasını değiştirmeden ilerleyemez', async ({ page }) => {
  const newPassword = 'yeni bir uzun parola 2026'
  await submitLogin(page, '22290003', seedPassword)
  await expect(page.getByRole('heading', { name: 'Parolanızı değiştirin' })).toBeVisible()

  await page.goto('/profil')
  await expect(page.getByRole('heading', { name: 'Parolanızı değiştirin' })).toBeVisible()

  await page.getByLabel('Mevcut parola', { exact: true }).fill(seedPassword)
  await page.getByLabel('Yeni parola', { exact: true }).fill(newPassword)
  await page.getByLabel('Yeni parola (tekrar)', { exact: true }).fill(newPassword)
  await page.getByRole('button', { name: 'Parolayı değiştir' }).click()
  await expect(page.getByRole('heading', { name: 'Hoş geldiniz, Can' })).toBeVisible()

  await logout(page)
  await login(page, '22290003', newPassword)
})

test('unutulan parola e-postadaki bağlantıyla sıfırlanır', async ({ page }) => {
  const newPassword = 'sıfırlanmış uzun bir parola'
  const since = new Date(Date.now() - 1000)
  await page.goto('/sifremi-unuttum')
  await page.getByLabel('Öğrenci / personel numarası ya da e-posta').fill('22290002')
  await page.getByRole('button', { name: 'Bağlantı gönder' }).click()
  await expect(page.getByRole('heading', { name: 'E-postanızı kontrol edin' })).toBeVisible()

  const link = await linkFromLatestEmail('ece.kaya@ogrenci.agora.test', since)
  await page.goto(link)
  await expect(page.getByRole('heading', { name: 'Yeni parola belirleyin' })).toBeVisible()
  // Token adres çubuğundan silinir.
  expect(new URL(page.url()).hash).toBe('')

  await page.getByLabel('Yeni parola', { exact: true }).fill(newPassword)
  await page.getByLabel('Yeni parola (tekrar)', { exact: true }).fill(newPassword)
  await page.getByRole('button', { name: 'Parolayı kaydet' }).click()
  await expect(page.getByText('Parolanız kaydedildi.')).toBeVisible()

  // Bağlantı tek kullanımlıktır.
  await page.goto(link)
  await expect(page.getByRole('heading', { name: 'Bağlantı geçersiz' })).toBeVisible()

  await login(page, '22290002', newPassword)
})
