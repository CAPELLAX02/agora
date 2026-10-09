import { expect, test, type Page } from '@playwright/test'

import { login, logout, nav, seedPassword, submitLogin } from './support/app'
import { linkFromLatestEmail } from './support/mailpit'
import { freshTotp } from './support/totp'

// Senaryolar sırayla çalışır: yönetici önce MFA kurar, sonraki adımlar MFA'lı oturum ister.
test.describe.configure({ mode: 'serial' })

const admin = { username: 'P90001', secret: '', lastStep: { value: 0 } }

async function adminLogin(page: Page) {
  await submitLogin(page, admin.username, seedPassword)
  await page.getByLabel('Doğrulama kodu').fill(await freshTotp(admin.secret, admin.lastStep))
  await expect(page.getByRole('heading', { name: /Hoş geldiniz/ })).toBeVisible()
}

test('yönetici MFA kurmadan hassas işlemleri yapamaz, kurunca oturumu yükselir', async ({ page }) => {
  await login(page, admin.username)
  await expect(page.getByText('İki adımlı doğrulama gerekli')).toBeVisible()
  await expect(nav(page, 'Kullanıcılar')).toBeVisible() // user:read MFA istemez
  await expect(nav(page, 'Denetim kayıtları')).toHaveCount(0) // audit:read ister

  await page.getByRole('link', { name: 'İki adımlı doğrulamayı aç' }).click()
  await page.getByRole('button', { name: 'Kurulumu başlat' }).click()
  const dialog = page.getByRole('dialog')
  const key = dialog.getByLabel('Kurulum anahtarı')
  await expect(key).toHaveText(/[A-Z2-7 ]{20,}/)
  admin.secret = (await key.innerText()).replace(/\s/g, '')

  await dialog.getByLabel('Doğrulama kodu').fill(await freshTotp(admin.secret, admin.lastStep))
  await dialog.getByLabel('Mevcut parola').fill(seedPassword)
  await dialog.getByRole('button', { name: 'Etkinleştir' }).click()

  const codes = page.getByRole('dialog', { name: 'Kurtarma kodlarınız' })
  await expect(codes.getByRole('listitem')).toHaveCount(10)
  await codes.getByLabel('Kodları güvenli bir yere kaydettim').check()
  await codes.getByRole('button', { name: 'Kapat' }).click()

  // Etkinleştiren oturum yeniden doğrulandı sayılır: hassas yetkiler hemen açılır.
  await expect(nav(page, 'Denetim kayıtları')).toBeVisible()
  await expect(page.getByText('İki adımlı doğrulama gerekli')).toHaveCount(0)
})

test('MFA açık hesabın girişi iki adımlıdır', async ({ page }) => {
  await submitLogin(page, admin.username, seedPassword)
  await expect(page.getByRole('heading', { name: 'İki adımlı doğrulama' })).toBeVisible()
  await page.getByLabel('Doğrulama kodu').fill('000000')
  await expect(page.getByText('Kod hatalı. Tekrar deneyin.')).toBeVisible()
  await page.getByLabel('Doğrulama kodu').fill(await freshTotp(admin.secret, admin.lastStep))
  await expect(page.getByRole('heading', { name: /Hoş geldiniz, Sistem/ })).toBeVisible()
  await expect(nav(page, 'Denetim kayıtları')).toBeVisible()
})

test('rol ataması ve sonlandırması kullanıcının açık oturumunda hemen etkili olur', async ({
  page,
  browser,
}) => {
  // Öğrencinin kendi tarayıcısı: giriş bir kez yapılır, rol değişiklikleri bu oturumda görülür.
  const studentContext = await browser.newContext()
  const student = await studentContext.newPage()
  await login(student, '22290001')
  await expect(nav(student, 'Kullanıcılar')).toHaveCount(0)

  await adminLogin(page)
  await nav(page, 'Kullanıcılar').click()
  await page.getByRole('searchbox', { name: 'Ara' }).fill('22290001')
  await page.getByRole('link', { name: 'Deniz Aksoy' }).click()

  await page.getByRole('button', { name: 'Rol ata' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('combobox', { name: 'Rol' }).click()
  await page.getByRole('option', { name: 'Denetçi' }).click()
  await dialog.getByLabel('Gerekçe').fill('Uçtan uca test: geçici denetçi')
  await dialog.getByRole('button', { name: 'Ata' }).click()
  const row = page.getByRole('row', { name: /Denetçi/ })
  await expect(row.getByText('Geçerli')).toBeVisible()

  // Aynı oturum, yeniden giriş yok: yetkiler her istekte sunucuda çözülür.
  await student.reload()
  await expect(nav(student, 'Kullanıcılar')).toBeVisible()
  // Denetçinin denetim izi yetkisi MFA ister: öğrencinin oturumunda MFA yok.
  await expect(nav(student, 'Denetim kayıtları')).toHaveCount(0)
  await expect(student.getByText('İki adımlı doğrulama gerekli')).toBeVisible()

  await row.getByRole('button', { name: 'Sonlandır' }).click()
  await page.getByRole('dialog').getByLabel('Gerekçe').fill('Test bitti')
  await page.getByRole('dialog').getByRole('button', { name: 'Sonlandır' }).click()
  await expect(row.getByText('Sona erdi')).toBeVisible()

  await student.goto('/yonetim/kullanicilar')
  await expect(student.getByText('Bu işlem için yetkiniz yok.')).toBeVisible()
  await studentContext.close()
})

test('yöneticinin açtığı hesap aktivasyon e-postasıyla etkinleşir', async ({ page, browser }) => {
  const number = `P7${Date.now().toString().slice(-6)}`
  const email = `${number.toLowerCase()}@agora.test`
  const password = 'aktivasyon için uzun parola'
  const since = new Date(Date.now() - 1000)

  await adminLogin(page)
  await page.goto('/yonetim/kullanicilar')
  await page.getByRole('button', { name: 'Yeni kullanıcı' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('combobox', { name: 'Hesap türü' }).click()
  await page.getByRole('option', { name: 'Personel' }).click()
  await dialog.getByLabel('Personel numarası').fill(number)
  await dialog.getByLabel('Ad', { exact: true }).fill('Selin')
  await dialog.getByLabel('Soyad').fill('Er')
  await dialog.getByLabel('E-posta').fill(email)
  await dialog.getByRole('button', { name: 'Hesabı oluştur' }).click()
  await expect(page.getByRole('heading', { name: 'Selin Er' })).toBeVisible()
  await expect(page.getByText('Aktivasyon bekliyor').first()).toBeVisible()

  const newcomer = await (await browser.newContext()).newPage()
  await newcomer.goto(await linkFromLatestEmail(email, since))
  await expect(newcomer.getByRole('heading', { name: 'Hesabınızı etkinleştirin' })).toBeVisible()
  await newcomer.getByLabel('Yeni parola', { exact: true }).fill(password)
  await newcomer.getByLabel('Yeni parola (tekrar)', { exact: true }).fill(password)
  await newcomer.getByRole('button', { name: 'Parolayı kaydet' }).click()
  await expect(newcomer.getByText('Hesabınız etkinleştirildi.', { exact: false })).toBeVisible()
  await login(newcomer, number, password)

  await page.reload()
  await expect(page.getByText('Aktif').first()).toBeVisible()
  await logout(page)
})
